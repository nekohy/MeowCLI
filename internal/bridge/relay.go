package bridge

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/nekohy/MeowCLI/api"
	apiGemini "github.com/nekohy/MeowCLI/api/gemini"
	"github.com/nekohy/MeowCLI/core/scheduling"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type upstreamRelay struct {
	ctx                  context.Context
	scheduler            CredentialScheduler
	requestHeaders       http.Header
	allowedPlans         []string
	streamRequest        bool
	modelAlias           string
	modelTier            string
	apiType              utils.APIType
	backend              api.Backend
	replaceResponseModel bool
	responseModel        string
	requestBody          []byte
	backendOptions       api.BackendOpts
	sessionKey           string
}

type retryTracker struct {
	lastErr                  relayError
	hasLastErr               bool
	graceCredentialID        string
	graceRetriedCredentialID string
}

// relayUpstream executes the common retry loop shared by all relay handlers.
// It writes the response or an error to c and returns.
func (h *Handler) relayUpstream(c *gin.Context, cfg upstreamRelay) {
	state := retryTracker{}
	for attempt := 1; attempt <= h.maxAttempts(); attempt++ {
		credID, err := cfg.scheduler.SelectCredential(cfg.ctx, scheduling.CredentialSelection{
			Headers:               cfg.requestHeaders,
			PreferredCredentialID: h.preferredCredential(cfg.sessionKey, state.graceCredentialID),
			AllowedPlanTypes:      cfg.allowedPlans,
			ModelTier:             cfg.modelTier,
		})
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
			continue
		}

		headers := cfg.upstreamHeaders(authHeaders)
		upstreamStarted := time.Now()
		resp, err := cfg.send(credID, headers)
		if err != nil {
			if h.handleSendFailure(cfg, credID, err, upstreamStarted, attempt, &state) {
				return
			}
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
	}

	if !state.hasLastErr {
		state.lastErr = errUpstreamRequestFailed
	}
	writeRelayError(c, state.lastErr)
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
	timing, err := h.writeUpstreamResponse(c, resp, cfg.backend, cfg.responseModel, cfg.replaceResponseModel, cfg.streamRequest, started)
	metrics := cfg.logMetrics(timing.firstByte, timing.duration, "")
	if err != nil {
		if cfg.ctx.Err() != nil {
			return
		}
		cfg.scheduler.RecordFailure(cfg.ctx, credID, 0, cfg.modelTier, 0, metrics)
		log.Warn().Err(err).Str("credential", credID).Int("status", resp.StatusCode).Msg("relay response write failed")
		if !c.Writer.Written() {
			writeRelayError(c, errRelayResponseFailed)
		}
		return
	}
	h.bindSessionCredential(cfg.sessionKey, credID)
	cfg.scheduler.RecordSuccess(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, metrics)
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
		return h.handleRateLimit(cfg, credID, resp, errText, metrics, attempt, state)
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

func (h *Handler) handleRateLimit(cfg upstreamRelay, credID string, resp *http.Response, errText string, metrics db.LogRequestMetrics, attempt int, state *retryTracker) bool {
	decision := cfg.scheduler.RetryDecision(int32(resp.StatusCode), errText, resp.Header)
	if !decision.SameCredential {
		state.clearGrace()
		cfg.scheduler.QueueQuotaRefresh(cfg.ctx, credID, cfg.modelTier)
		cfg.scheduler.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, decision.Delay, metrics)
		logRetryingUpstreamError(resp.StatusCode, credID, cfg.modelAlias, attempt)
		return false
	}

	if state.graceRetriedCredentialID == credID {
		state.clearGrace()
		cfg.scheduler.QueueQuotaRefresh(cfg.ctx, credID, cfg.modelTier)
		cfg.scheduler.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, decision.Delay, metrics)
		log.Warn().
			Int("status", resp.StatusCode).
			Str("credential", credID).
			Str("model", cfg.modelAlias).
			Int("attempt", attempt).
			Msg("upstream rate limited after grace retry, retrying with next credential")
		return false
	}

	state.graceCredentialID = credID
	state.graceRetriedCredentialID = credID
	cfg.scheduler.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, 0, metrics)
	if !waitForRetry(cfg.ctx, decision.Delay) {
		return true
	}
	log.Warn().
		Int("status", resp.StatusCode).
		Str("credential", credID).
		Str("model", cfg.modelAlias).
		Dur("delay", decision.Delay).
		Int("attempt", attempt).
		Msg("upstream rate limited, grace retrying same credential")
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
	headers.Del(utils.HeaderPlanTypePreference)
	scrubLocalAuthHeaders(headers)
	for k, vs := range authHeaders {
		headers[k] = vs
	}
	return headers
}

func (cfg upstreamRelay) send(credentialID string, headers http.Header) (*http.Response, error) {
	opts := cfg.backendOptions
	if geminiOpts, ok := opts.(*apiGemini.Options); ok {
		cloned := *geminiOpts
		cloned.ProjectID = headers.Get("X-Meow-Gemini-Project")
		opts = &cloned
	}
	return cfg.backend.Chat(&api.Request{
		Ctx:     cfg.ctx,
		CredID:  credentialID,
		Body:    cfg.requestBody,
		Headers: headers,
		APIType: cfg.apiType,
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
