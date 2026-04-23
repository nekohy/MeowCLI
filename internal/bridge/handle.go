package bridge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/nekohy/MeowCLI/api"
	storedb "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

const defaultRelayRoundTripTimeout = 2 * time.Minute
const maxBridgeRequestBodyBytes = 32 << 20

type relayRequest struct {
	Model     string `json:"model"`
	SessionID string `json:"session_id"`
	Stream    bool   `json:"stream"`
}

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

	var req relayRequest
	if err := sonic.Unmarshal(body, &req); err != nil {
		writeRelayError(c, errReadRequestBody)
		return
	}

	alias := strings.TrimSpace(req.Model)
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
	sessionKey := preferredSessionKey(info.Handler, req.SessionID)

	backend, ok := h.backends[info.Handler]
	if !ok {
		writeRelayError(c, errBackendUnavailable)
		return
	}
	if !slices.Contains(backend.APIType(), apiType) {
		writeRelayError(c, errUnsupportedAPIType)
		return
	}

	sched, ok := h.schedulers[info.Handler]
	if !ok {
		writeRelayError(c, errSchedulerUnavailable)
		return
	}

	needReplace := alias != info.Origin
	upstreamBody := body
	if needReplace {
		upstreamBody = backend.ReplaceModel(body, info.Origin)
	}

	chatBackend, ok := backend.(api.ChatBackend)
	if !ok {
		writeRelayError(c, errBackendUnavailable)
		return
	}

	h.relayWithRetry(c, relayConfig{
		ctx:            ctx,
		sched:          sched,
		requestHeaders: c.Request.Header,
		allowedPlans:   info.AllowedPlanTypes,
		streamRequest:  req.Stream,
		modelAlias:     alias,
		backend:        backend,
		needReplace:    needReplace,
		responseAlias:  alias,
		resolvePreferred: func(graceCredID string) string {
			if graceCredID != "" {
				return graceCredID
			}
			preferred, _ := h.readSessionRoute(sessionKey)
			return preferred
		},
		doRequest: func(attemptCtx context.Context, credID string, headers http.Header) (*http.Response, error) {
			return chatBackend.Chat(attemptCtx, credID, upstreamBody, headers, apiType)
		},
		onSuccess: func(credID string) {
			h.writeSessionRoute(sessionKey, credID)
		},
	})
}

func preferredSessionKey(providerType utils.HandlerType, sessionID string) string {
	provider := strings.TrimSpace(string(providerType))
	sessionID = strings.TrimSpace(sessionID)
	if provider != "" && sessionID != "" {
		return provider + "-" + sessionID
	}
	return ""
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

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (h *Handler) writeResponse(c *gin.Context, resp *http.Response, backend api.Backend, alias string, needReplace bool) error {
	responseAlias := ""
	if needReplace {
		responseAlias = alias
	}
	normalizeGemini := backend.HandlerType() == utils.HandlerGemini

	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/event-stream") {
		return h.streamSSE(c, resp, backend, responseAlias)
	}

	if responseAlias == "" && !normalizeGemini {
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

	bodyBytes = backend.ReplaceModel(bodyBytes, responseAlias)
	c.Data(resp.StatusCode, contentType, bodyBytes)
	return nil
}
