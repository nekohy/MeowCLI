package bridge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nekohy/MeowCLI/api"
	storedb "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
)

const defaultRelayRoundTripTimeout = 2 * time.Minute
const maxBridgeRequestBodyBytes = 32 << 20

func (h *Handler) handle(c *gin.Context, apiType utils.APIType) {
	ctx := c.Request.Context()

	bodyReader := http.MaxBytesReader(c.Writer, c.Request.Body, maxBridgeRequestBodyBytes)
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeRelayError(c, errRequestBodyTooLarge)
			return
		}
		writeRelayError(c, errReadRequestBody)
		return
	}

	alias := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if alias == "" {
		writeRelayError(c, errModelRequired)
		return
	}

	info, err := h.resolveModel(ctx, alias)
	if err != nil {
		switch {
		case errors.Is(err, storedb.ErrNotFound):
			writeRelayError(c, errModelNotConfigured)
		default:
			writeRelayError(c, errModelResolutionFailed)
		}
		return
	}

	backend, ok := h.backends[info.handler]
	if !ok {
		writeRelayError(c, errBackendUnavailable)
		return
	}
	if !slices.Contains(backend.APIType(), apiType) {
		writeRelayError(c, errUnsupportedAPIType)
		return
	}

	sched, ok := h.schedulers[info.handler]
	if !ok {
		writeRelayError(c, errSchedulerUnavailable)
		return
	}

	// alias != origin 时，请求体替换为上游模型名，响应体替换回别名
	needReplace := alias != info.origin
	upstreamBody := body
	if needReplace {
		upstreamBody = backend.ReplaceModel(body, info.origin)
	}
	streamRequest := gjson.GetBytes(body, "stream").Bool()

	// 带重试的上游请求：每次失败换一个新凭证
	var lastRelayErr relayError
	var haveLastRelayErr bool
	for attempt := 0; attempt < h.maxAttempts(); attempt++ {
		credID, err := sched.Pick(ctx, c.Request.Header)
		if err != nil {
			if haveLastRelayErr {
				break
			}
			writeRelayError(c, errNoAvailableCredential)
			return
		}

		authHeaders, err := sched.AuthHeaders(ctx, credID)
		if err != nil {
			sched.RecordFailure(ctx, credID, 0, err.Error(), 0)
			lastRelayErr = errUpstreamAuthFailed
			haveLastRelayErr = true
			log.Warn().Err(err).Str("credential", credID).Msg("get auth headers failed, retrying")
			continue
		}

		headers := c.Request.Header.Clone()
		headers.Del(utils.HeaderPlanTypePreference)
		for k, vs := range authHeaders {
			headers[k] = vs
		}

		attemptCtx, cancel := relayAttemptContext(ctx, streamRequest)
		resp, err := backend.Chat(attemptCtx, credID, upstreamBody, headers, apiType)
		if err != nil {
			cancel()
			sched.RecordFailure(ctx, credID, 0, err.Error(), 0)
			lastRelayErr = errUpstreamRequestFailed
			haveLastRelayErr = true
			log.Warn().Err(err).Str("credential", credID).Int("attempt", attempt+1).Msg("upstream request failed, retrying")
			continue
		}

		if isSuccessfulUpstreamStatus(resp.StatusCode) {
			if err := h.writeResponse(c, resp, backend, alias, needReplace); err != nil {
				cancel()
				sched.RecordFailure(ctx, credID, 0, err.Error(), 0)
				log.Warn().Err(err).Str("credential", credID).Int("status", resp.StatusCode).Msg("relay response write failed")
				if !c.Writer.Written() {
					writeRelayError(c, errRelayResponseFailed)
				}
				return
			}
			cancel()
			sched.RecordSuccess(ctx, credID, int32(resp.StatusCode))
			return
		}

		// 上游返回错误
		errBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		cancel()
		errText := string(errBytes)
		lastRelayErr = relayErrorForUpstreamStatus(resp.StatusCode)
		haveLastRelayErr = true

		if sched.HandleUnauthorized(ctx, credID, int32(resp.StatusCode), errText) {
			log.Warn().
				Int("status", resp.StatusCode).
				Str("credential", credID).
				Int("attempt", attempt+1).
				Msg("upstream returned unauthorized, retrying with next credential")
			continue
		}

		if !shouldRetryUpstreamStatus(resp.StatusCode) {
			writeRelayError(c, lastRelayErr)
			return
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			var retryAfter time.Duration
			if raw := resp.Header.Get("Retry-After"); raw != "" {
				if seconds, convErr := strconv.Atoi(raw); convErr == nil && seconds > 0 {
					retryAfter = time.Duration(seconds) * time.Second
				}
			}
			sched.RecordFailure(ctx, credID, int32(resp.StatusCode), errText, retryAfter)
		} else {
			sched.RecordFailure(ctx, credID, int32(resp.StatusCode), errText, 0)
		}

		log.Warn().Int("status", resp.StatusCode).Str("credential", credID).Int("attempt", attempt+1).Msg("upstream error, retrying")
	}

	// 所有重试耗尽
	if !haveLastRelayErr {
		lastRelayErr = errUpstreamRequestFailed
	}
	writeRelayError(c, lastRelayErr)
}

func relayAttemptContext(ctx context.Context, stream bool) (context.Context, context.CancelFunc) {
	if stream {
		return ctx, func() {}
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultRelayRoundTripTimeout)
}

func shouldRetryUpstreamStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout,
		http.StatusMisdirectedRequest,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (h *Handler) writeResponse(c *gin.Context, resp *http.Response, backend api.Backend, alias string, needReplace bool) error {
	responseAlias := ""
	if needReplace {
		responseAlias = alias
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/event-stream") {
		return h.streamSSE(c, resp, backend, responseAlias)
	}

	if responseAlias == "" {
		defer func() {
			_ = resp.Body.Close()
		}()

		if contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Status(resp.StatusCode)
		_, err := io.CopyBuffer(c.Writer, resp.Body, make([]byte, 32*1024))
		return err
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}

	if responseAlias != "" {
		bodyBytes = backend.ReplaceModel(bodyBytes, responseAlias)
	}

	c.Data(resp.StatusCode, contentType, bodyBytes)
	return nil
}
