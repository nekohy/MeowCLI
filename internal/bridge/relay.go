package bridge

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/bytedance/sonic/ast"
	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/core/scheduling"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type upstreamRelay struct {
	ctx                   context.Context
	scheduler             CredentialScheduler
	requestHeaders        http.Header
	allowedPlans          []string
	streamRequest         bool
	modelAlias            string
	modelTier             string
	apiType               utils.APIType
	backend               api.Backend
	replaceResponseModel  bool
	responseModel         string
	requestJSON           *ast.Node
	backendOptions        api.BackendOpts
	prepareBackendOptions func(context.Context, string, api.BackendOpts) (api.BackendOpts, error)
	sessionKey            string
	modelScheduling       scheduling.ModelScheduling
	payloadAPIType        utils.APIType
	contentAffinity       contentAffinityRequest
}

type retryTracker struct {
	lastErr                  relayError
	hasLastErr               bool
	graceCredentialID        string
	graceRetriedCredentialID string
}

// relayUpstream 是所有协议共用的上游重试循环，最终负责写回响应或错误
func (h *Handler) relayUpstream(c *gin.Context, cfg upstreamRelay) {
	if cfg.requestJSON == nil {
		writeRelayError(c, errReadRequestBody)
		return
	}
	if cfg.modelScheduling.ContentAffinity {
		h.contentAffinity.configureCapacity(h.settingsSnapshot().ContentAffinityMaxEntries)
		cfg.contentAffinity = h.buildContentAffinityRequest(cfg.modelAlias, cfg.payloadAPIType, cfg.requestJSON)
	}
	requestBody, err := cfg.requestJSON.MarshalJSON()
	if err != nil {
		writeRelayError(c, errReadRequestBody)
		return
	}
	// 防止长时间的SSE让AST对象持续占用内存
	requestBody = bytes.Clone(requestBody)
	cfg.requestJSON = nil

	state := retryTracker{}
	for attempt := 1; attempt <= h.maxAttempts(); attempt++ {
		credID, err := h.selectRelayCredential(cfg, state)
		if err != nil {
			if state.hasLastErr {
				break
			}
			writeRelayError(c, errNoAvailableCredential)
			return
		}

		authHeaders, err := cfg.scheduler.AuthHeaders(cfg.ctx, credID)
		if err != nil {
			if h.handleAuthFailure(cfg, credID, err, &state) {
				return
			}
			h.fillFirst.deleteIf(cfg.modelAlias, credID)
			continue
		}

		headers := cfg.upstreamHeaders(authHeaders)
		upstreamStarted := time.Now()
		resp, err := cfg.send(credID, headers, requestBody)
		if err != nil {
			if h.handleSendFailure(cfg, credID, err, upstreamStarted, attempt, &state) {
				return
			}
			h.fillFirst.deleteIf(cfg.modelAlias, credID)
			continue
		}

		if isSuccessfulUpstreamStatus(resp.StatusCode) {
			h.handleSuccessfulResponse(c, cfg, credID, resp, upstreamStarted)
			return
		}

		stop := h.handleUpstreamError(c, cfg, credID, resp, upstreamStarted, attempt, &state)
		if stop {
			return
		}
		if state.graceCredentialID == "" {
			h.fillFirst.deleteIf(cfg.modelAlias, credID)
		}
	}

	if !state.hasLastErr {
		state.lastErr = errUpstreamRequestFailed
	}
	writeRelayError(c, state.lastErr)
}

func (h *Handler) selectRelayCredential(cfg upstreamRelay, state retryTracker) (string, error) {
	selection := scheduling.CredentialSelection{
		AllowedPlanTypes: cfg.allowedPlans,
		ModelTier:        cfg.modelTier,
	}
	selectCredential := func(preferred string) (string, error) {
		selection.PreferredCredentialID = preferred
		return cfg.scheduler.SelectCredential(cfg.ctx, selection)
	}

	preferred := h.preferredCredential(cfg.sessionKey, state.graceCredentialID)
	if preferred != "" {
		credID, err := selectCredential(preferred)
		if err == nil {
			return credID, nil
		}
	}
	// 凭据续用低于显式会话亲和，且进入重试后直接交回原有调度逻辑。
	if cfg.modelScheduling.FillFirst && !state.hasLastErr {
		fillFirstPreferred, _ := h.fillFirst.GetIfPresent(cfg.modelAlias)
		if fillFirstPreferred != "" && fillFirstPreferred != preferred {
			credID, err := selectCredential(fillFirstPreferred)
			if err == nil {
				return credID, nil
			}
			h.fillFirst.deleteIf(cfg.modelAlias, fillFirstPreferred)
		}
	}

	// hasLastErr 已经能表示“请求进入重试” 内容粘性只参与首次选择，
	// 优先级低于显式 prompt_cache_key；请求发出后的重试完全沿用原逻辑
	if cfg.modelScheduling.ContentAffinity && !state.hasLastErr {
		contentPreferred := h.contentAffinity.match(cfg.contentAffinity.modelName, cfg.contentAffinity.fingerprint)
		if contentPreferred != "" && contentPreferred != preferred {
			credID, err := selectCredential(contentPreferred)
			if err == nil {
				return credID, nil
			}
		}
	}

	return selectCredential("")
}

func (h *Handler) handleAuthFailure(cfg upstreamRelay, credID string, err error, state *retryTracker) bool {
	if cfg.ctx.Err() != nil {
		return true
	}
	cfg.scheduler.RecordFailure(cfg.ctx, credID, 0, cfg.modelTier, 0, cfg.logMetrics(0, 0, ""))
	state.remember(errUpstreamAuthFailed)
	log.Warn().Err(err).Str("credential", credID).Msg("get auth headers failed, retrying")
	return false
}

func (h *Handler) handleSendFailure(cfg upstreamRelay, credID string, err error, started time.Time, attempt int, state *retryTracker) bool {
	if cfg.ctx.Err() != nil {
		return true
	}
	cfg.scheduler.RecordFailure(cfg.ctx, credID, 0, cfg.modelTier, 0, cfg.logMetrics(0, time.Since(started), ""))
	state.remember(errUpstreamRequestFailed)
	log.Warn().Err(err).Str("credential", credID).Int("attempt", attempt).Msg("upstream request failed, retrying")
	return false
}

func (h *Handler) handleSuccessfulResponse(c *gin.Context, cfg upstreamRelay, credID string, resp *http.Response, started time.Time) {
	if cfg.modelScheduling.FillFirst {
		h.fillFirst.SetIfAbsent(cfg.modelAlias, credID)
	}
	timing, err := h.writeUpstreamResponse(c, resp, cfg.backend, cfg.responseModel, cfg.replaceResponseModel, cfg.streamRequest, started)
	metrics := cfg.logMetrics(timing.firstByte, timing.duration, "")
	recordCtx := detachedRecordContext(cfg.ctx)
	if err != nil {
		if cfg.ctx.Err() != nil {
			// 客户端意外断开时仍然记录日志
			cfg.scheduler.RecordSuccess(recordCtx, credID, int32(resp.StatusCode), cfg.modelTier, metrics)
			return
		}
		h.fillFirst.deleteIf(cfg.modelAlias, credID)
		cfg.scheduler.RecordFailure(recordCtx, credID, 0, cfg.modelTier, 0, metrics)
		log.Warn().Err(err).Str("credential", credID).Int("status", resp.StatusCode).Msg("relay response write failed")
		if !c.Writer.Written() {
			writeRelayError(c, errRelayResponseFailed)
		}
		return
	}
	h.bindSessionCredential(cfg.sessionKey, credID)
	h.contentAffinity.bind(cfg.contentAffinity.modelName, cfg.contentAffinity.fingerprint, credID)
	cfg.scheduler.RecordSuccess(recordCtx, credID, int32(resp.StatusCode), cfg.modelTier, metrics)
}

func detachedRecordContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

func (h *Handler) handleUpstreamError(c *gin.Context, cfg upstreamRelay, credID string, resp *http.Response, started time.Time, attempt int, state *retryTracker) bool {
	errText, timing := readUpstreamError(resp.Body, started)
	metrics := cfg.logMetrics(timing.firstByte, timing.duration, errText)
	state.remember(relayErrorForUpstreamStatus(resp.StatusCode))

	if cfg.scheduler.HandleUnauthorized(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, metrics) {
		state.clearGrace()
		log.Warn().
			Int("status", resp.StatusCode).
			Str("credential", credID).
			Int("attempt", attempt).
			Msg("upstream returned unauthorized, retrying with next credential")
		return false
	}

	if !retryableUpstreamStatus(resp.StatusCode) {
		cfg.scheduler.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, 0, metrics)
		writeRelayError(c, state.lastErr)
		return true
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return h.handleRetryHint(cfg, credID, resp, errText, metrics, attempt, state, true)
	}

	if decision := cfg.scheduler.RetryDecision(int32(resp.StatusCode), errText, resp.Header); decision.Delay > 0 {
		return h.handleRetryHintDecision(cfg, credID, resp.StatusCode, decision, metrics, attempt, state, false)
	}

	state.clearGrace()
	cfg.scheduler.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, 0, metrics)
	logRetryingUpstreamError(resp.StatusCode, credID, cfg.modelAlias, attempt)
	return false
}

func readUpstreamError(body io.ReadCloser, started time.Time) (string, responseTiming) {
	timedBody := newTimedReadCloser(body, started)
	errBytes, _ := io.ReadAll(io.LimitReader(timedBody, 4096))
	_, _ = io.Copy(io.Discard, timedBody)
	_ = timedBody.Close()
	return string(errBytes), timedBody.timing()
}

func (h *Handler) handleRetryHint(cfg upstreamRelay, credID string, resp *http.Response, errText string, metrics db.LogRequestMetrics, attempt int, state *retryTracker, refreshQuota bool) bool {
	decision := cfg.scheduler.RetryDecision(int32(resp.StatusCode), errText, resp.Header)
	return h.handleRetryHintDecision(cfg, credID, resp.StatusCode, decision, metrics, attempt, state, refreshQuota)
}

func (h *Handler) handleRetryHintDecision(cfg upstreamRelay, credID string, statusCode int, decision scheduling.RetryDecision, metrics db.LogRequestMetrics, attempt int, state *retryTracker, refreshQuota bool) bool {
	if !decision.SameCredential {
		state.clearGrace()
		if refreshQuota {
			cfg.scheduler.QueueQuotaRefresh(cfg.ctx, credID, cfg.modelTier)
		}
		cfg.scheduler.RecordFailure(cfg.ctx, credID, int32(statusCode), cfg.modelTier, decision.Delay, metrics)
		logRetryingUpstreamError(statusCode, credID, cfg.modelAlias, attempt)
		return false
	}

	if state.graceRetriedCredentialID == credID {
		state.clearGrace()
		if refreshQuota {
			cfg.scheduler.QueueQuotaRefresh(cfg.ctx, credID, cfg.modelTier)
		}
		cfg.scheduler.RecordFailure(cfg.ctx, credID, int32(statusCode), cfg.modelTier, decision.Delay, metrics)
		log.Warn().
			Int("status", statusCode).
			Str("credential", credID).
			Str("model", cfg.modelAlias).
			Int("attempt", attempt).
			Msg("upstream retry hint repeated after grace retry, retrying with next credential")
		return false
	}

	state.graceCredentialID = credID
	state.graceRetriedCredentialID = credID
	cfg.scheduler.RecordFailure(cfg.ctx, credID, int32(statusCode), cfg.modelTier, 0, metrics)
	if !waitForRetry(cfg.ctx, decision.Delay) {
		return true
	}
	log.Warn().
		Int("status", statusCode).
		Str("credential", credID).
		Str("model", cfg.modelAlias).
		Dur("delay", decision.Delay).
		Int("attempt", attempt).
		Msg("upstream retry hint, grace retrying same credential")
	return false
}

func (h *Handler) preferredCredential(sessionKey string, graceCredentialID string) string {
	if graceCredentialID != "" {
		return graceCredentialID
	}
	preferred, _ := h.sessionCredential(sessionKey)
	return preferred
}

func (cfg upstreamRelay) upstreamHeaders(authHeaders http.Header) http.Header {
	headers := cfg.requestHeaders.Clone()
	headers.Del("Accept")
	scrubLocalAuthHeaders(headers)
	for k, vs := range authHeaders {
		headers[k] = vs
	}
	return headers
}

func (cfg upstreamRelay) send(credentialID string, headers http.Header, body []byte) (*http.Response, error) {
	opts := cfg.backendOptions
	if cfg.prepareBackendOptions != nil {
		prepared, err := cfg.prepareBackendOptions(cfg.ctx, credentialID, opts)
		if err != nil {
			return nil, err
		}
		opts = prepared
	}
	return cfg.backend.Chat(&api.Request{
		Ctx:     cfg.ctx,
		CredID:  credentialID,
		Body:    body,
		Headers: headers,
		APIType: cfg.apiType,
		Stream:  cfg.streamRequest,
		Opts:    opts,
	})
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
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

func (s *retryTracker) remember(err relayError) {
	s.lastErr = err
	s.hasLastErr = true
}

func (s *retryTracker) clearGrace() {
	s.graceCredentialID = ""
	s.graceRetriedCredentialID = ""
}

func logRetryingUpstreamError(status int, credentialID string, model string, attempt int) {
	log.Warn().
		Int("status", status).
		Str("credential", credentialID).
		Str("model", model).
		Int("attempt", attempt).
		Msg("upstream error, retrying")
}

func (cfg upstreamRelay) logMetrics(firstByte time.Duration, duration time.Duration, errorBody string) db.LogRequestMetrics {
	return db.LogRequestMetrics{
		Model:     cfg.modelAlias,
		APIType:   string(cfg.apiType),
		Stream:    cfg.streamRequest,
		FirstByte: logSeconds(firstByte),
		Duration:  logSeconds(duration),
		Error:     errorBody,
	}
}
