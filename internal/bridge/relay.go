package bridge

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/nekohy/MeowCLI/api"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// relayConfig encapsulates the parameters that differ between relay handlers.
type relayConfig struct {
	ctx            context.Context
	sched          CredentialScheduler
	requestHeaders http.Header
	allowedPlans   []string
	streamRequest  bool
	modelAlias     string
	modelTier      string
	apiType        utils.APIType
	backend        api.Backend
	needReplace    bool
	responseAlias  string

	// resolvePreferred returns the preferred credential ID for this attempt.
	// graceCredID is the credential to retry from the previous grace retry.
	resolvePreferred func(graceCredID string) string
	// doRequest sends the upstream request with the given context, credential and headers.
	doRequest func(ctx context.Context, credID string, headers http.Header) (*http.Response, error)
	// onSuccess is called after a successful relay (e.g., to write session affinity).
	onSuccess func(credID string)
}

// relayWithRetry executes the common retry loop shared by all relay handlers.
// It writes the response or an error to c and returns.
func (h *Handler) relayWithRetry(c *gin.Context, cfg relayConfig) {
	var lastRelayErr relayError
	var haveLastRelayErr bool
	graceCredentialID := ""
	graceRetriedCredentialID := ""

	for attempt := 0; attempt < h.maxAttempts(); attempt++ {
		preferredCredentialID := cfg.resolvePreferred(graceCredentialID)
		var credID string
		var err error
		if tierPicker, ok := cfg.sched.(ModelTierPicker); ok && cfg.modelTier != "" {
			credID, err = tierPicker.PickWithTier(cfg.ctx, cfg.requestHeaders, preferredCredentialID, cfg.allowedPlans, cfg.modelTier)
		} else {
			credID, err = cfg.sched.Pick(cfg.ctx, cfg.requestHeaders, preferredCredentialID, cfg.allowedPlans)
		}
		if err != nil {
			if haveLastRelayErr {
				break
			}
			writeRelayError(c, errNoAvailableCredential)
			return
		}

		authHeaders, err := cfg.sched.AuthHeaders(cfg.ctx, credID)
		if err != nil {
			if cfg.ctx.Err() != nil {
				return
			}
			cfg.sched.RecordFailure(cfg.ctx, credID, 0, cfg.modelTier, 0, cfg.logMetrics(0, 0, ""))
			lastRelayErr = errUpstreamAuthFailed
			haveLastRelayErr = true
			log.Warn().Err(err).Str("credential", credID).Msg("get auth headers failed, retrying")
			continue
		}

		headers := cfg.requestHeaders.Clone()
		headers.Del("Accept")
		headers.Del(utils.HeaderPlanTypePreference)
		scrubLocalAuthHeaders(headers)
		for k, vs := range authHeaders {
			headers[k] = vs
		}

		upstreamStarted := time.Now()
		attemptCtx, cancel := relayAttemptContext(cfg.ctx, cfg.streamRequest)
		resp, err := cfg.doRequest(attemptCtx, credID, headers)
		if err != nil {
			cancel()
			if cfg.ctx.Err() != nil {
				return
			}
			cfg.sched.RecordFailure(cfg.ctx, credID, 0, cfg.modelTier, 0, cfg.logMetrics(0, time.Since(upstreamStarted), ""))
			lastRelayErr = errUpstreamRequestFailed
			haveLastRelayErr = true
			log.Warn().Err(err).Str("credential", credID).Int("attempt", attempt+1).Msg("upstream request failed, retrying")
			continue
		}
		if isSuccessfulUpstreamStatus(resp.StatusCode) {
			timing, err := h.writeResponse(c, resp, cfg.backend, cfg.responseAlias, cfg.needReplace, cfg.streamRequest, upstreamStarted)
			metrics := cfg.logMetrics(timing.firstByte, timing.duration, "")
			if err != nil {
				cancel()
				if cfg.ctx.Err() != nil {
					return
				}
				cfg.sched.RecordFailure(cfg.ctx, credID, 0, cfg.modelTier, 0, metrics)
				log.Warn().Err(err).Str("credential", credID).Int("status", resp.StatusCode).Msg("relay response write failed")
				if !c.Writer.Written() {
					writeRelayError(c, errRelayResponseFailed)
				}
				return
			}
			cancel()
			graceCredentialID = ""
			if cfg.onSuccess != nil {
				cfg.onSuccess(credID)
			}
			cfg.sched.RecordSuccess(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, metrics)
			return
		}

		// 上游返回错误
		timedBody := newTimedReadCloser(resp.Body, upstreamStarted)
		errBytes, _ := io.ReadAll(io.LimitReader(timedBody, 4096))
		_, _ = io.Copy(io.Discard, timedBody)
		_ = timedBody.Close()
		timing := timedBody.timing()
		cancel()
		errText := string(errBytes)
		metrics := cfg.logMetrics(timing.firstByte, timing.duration, errText)
		lastRelayErr = relayErrorForUpstreamStatus(resp.StatusCode)
		haveLastRelayErr = true

		if cfg.sched.HandleUnauthorized(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, metrics) {
			graceCredentialID = ""
			log.Warn().
				Int("status", resp.StatusCode).
				Str("credential", credID).
				Int("attempt", attempt+1).
				Msg("upstream returned unauthorized, retrying with next credential")
			continue
		}

		if !shouldRetryUpstreamStatus(resp.StatusCode) {
			cfg.sched.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, 0, metrics)
			writeRelayError(c, lastRelayErr)
			return
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := utils.ParseRetryAfterHeader(resp.Header)
			if retryAfter <= 0 {
				if parser, ok := cfg.sched.(RetryDelayParser); ok {
					retryAfter = parser.RetryDelay(int32(resp.StatusCode), errText, resp.Header)
				}
			}
			if decider, ok := cfg.sched.(GraceRetryDecider); ok {
				if delay, shouldRetrySameCredential := decider.GraceRetry(int32(resp.StatusCode), errText, retryAfter); shouldRetrySameCredential {
					if graceRetriedCredentialID == credID {
						graceCredentialID = ""
						graceRetriedCredentialID = ""
						refreshQuotaAfterRateLimit(cfg.ctx, cfg.sched, credID, cfg.modelTier)
						cfg.sched.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, retryAfter, metrics)
						log.Warn().
							Int("status", resp.StatusCode).
							Str("credential", credID).
							Str("model", cfg.modelAlias).
							Int("attempt", attempt+1).
							Msg("upstream rate limited after grace retry, retrying with next credential")
						continue
					}
					graceCredentialID = credID
					graceRetriedCredentialID = credID
					cfg.sched.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, 0, metrics)
					if !sleepWithContext(cfg.ctx, delay) {
						return
					}
					log.Warn().
						Int("status", resp.StatusCode).
						Str("credential", credID).
						Str("model", cfg.modelAlias).
						Dur("delay", delay).
						Int("attempt", attempt+1).
						Msg("upstream rate limited, grace retrying same credential")
					continue
				}
			}
			graceCredentialID = ""
			graceRetriedCredentialID = ""
			refreshQuotaAfterRateLimit(cfg.ctx, cfg.sched, credID, cfg.modelTier)
			cfg.sched.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, retryAfter, metrics)
		} else {
			graceCredentialID = ""
			graceRetriedCredentialID = ""
			cfg.sched.RecordFailure(cfg.ctx, credID, int32(resp.StatusCode), cfg.modelTier, 0, metrics)
		}

		log.Warn().
			Int("status", resp.StatusCode).
			Str("credential", credID).
			Str("model", cfg.modelAlias).
			Int("attempt", attempt+1).
			Msg("upstream error, retrying")
	}

	if !haveLastRelayErr {
		lastRelayErr = errUpstreamRequestFailed
	}
	writeRelayError(c, lastRelayErr)
}

func (cfg relayConfig) logMetrics(firstByte time.Duration, duration time.Duration, errorBody string) db.LogRequestMetrics {
	return db.LogRequestMetrics{
		Model:     cfg.modelAlias,
		APIType:   string(cfg.apiType),
		Stream:    cfg.streamRequest,
		FirstByte: logSeconds(firstByte),
		Duration:  logSeconds(duration),
		Error:     errorBody,
	}
}

func refreshQuotaAfterRateLimit(ctx context.Context, sched CredentialScheduler, credentialID string, modelTier string) {
	refresher, ok := sched.(QuotaRefresher)
	if !ok {
		return
	}
	refresher.RefreshQuota(ctx, credentialID, modelTier)
}
