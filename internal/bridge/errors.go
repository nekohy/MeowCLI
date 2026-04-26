package bridge

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type relayError struct {
	StatusCode int
	Code       string
	Message    string
}

var (
	errReadRequestBody        = relayError{StatusCode: http.StatusBadRequest, Code: "invalid_request_body", Message: "failed to read request body"}
	errRequestBodyTooLarge    = relayError{StatusCode: http.StatusRequestEntityTooLarge, Code: "request_body_too_large", Message: "request body is too large"}
	errModelRequired          = relayError{StatusCode: http.StatusBadRequest, Code: "model_required", Message: "model is required"}
	errModelNotConfigured     = relayError{StatusCode: http.StatusBadRequest, Code: "model_not_configured", Message: "model is not configured"}
	errModelResolutionFailed  = relayError{StatusCode: http.StatusInternalServerError, Code: "model_resolution_failed", Message: "failed to resolve model"}
	errBackendUnavailable     = relayError{StatusCode: http.StatusServiceUnavailable, Code: "backend_unavailable", Message: "backend is unavailable"}
	errUnsupportedAPIType     = relayError{StatusCode: http.StatusBadRequest, Code: "unsupported_api_type", Message: "handler does not support requested api type"}
	errSchedulerUnavailable   = relayError{StatusCode: http.StatusServiceUnavailable, Code: "scheduler_unavailable", Message: "credential scheduler is unavailable"}
	errNoAvailableCredential  = relayError{StatusCode: http.StatusServiceUnavailable, Code: "credential_unavailable", Message: "no available credential"}
	errUpstreamAuthFailed     = relayError{StatusCode: http.StatusBadGateway, Code: "upstream_auth_failed", Message: "failed to prepare upstream authentication"}
	errUpstreamRequestFailed  = relayError{StatusCode: http.StatusBadGateway, Code: "upstream_request_failed", Message: "upstream request failed"}
	errUpstreamRequestInvalid = relayError{StatusCode: http.StatusBadRequest, Code: "upstream_request_invalid", Message: "upstream rejected the request"}
	errUpstreamCredentialFail = relayError{StatusCode: http.StatusBadGateway, Code: "upstream_credential_rejected", Message: "upstream rejected internal credentials"}
	errUpstreamRateLimited    = relayError{StatusCode: http.StatusTooManyRequests, Code: "upstream_rate_limited", Message: "upstream is rate limited"}
	errUpstreamUnavailable    = relayError{StatusCode: http.StatusBadGateway, Code: "upstream_unavailable", Message: "upstream is unavailable"}
	errRelayResponseFailed    = relayError{StatusCode: http.StatusBadGateway, Code: "relay_response_failed", Message: "failed to relay upstream response"}
)

func writeRelayError(c *gin.Context, relayErr relayError) {
	c.JSON(relayErr.StatusCode, gin.H{
		"error": relayErr.Message,
		"code":  relayErr.Code,
	})
}

func relayErrorForUpstreamStatus(status int) relayError {
	switch status {
	case http.StatusUnauthorized, http.StatusPaymentRequired:
		return errUpstreamCredentialFail
	case http.StatusTooManyRequests:
		return errUpstreamRateLimited
	case http.StatusBadRequest,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusNotAcceptable,
		http.StatusConflict,
		http.StatusGone,
		http.StatusLengthRequired,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable,
		http.StatusExpectationFailed,
		http.StatusUnprocessableEntity,
		http.StatusLocked,
		http.StatusFailedDependency,
		http.StatusUpgradeRequired,
		http.StatusPreconditionRequired,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusUnavailableForLegalReasons:
		return errUpstreamRequestInvalid
	case http.StatusRequestTimeout,
		http.StatusMisdirectedRequest,
		http.StatusTooEarly,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return errUpstreamUnavailable
	default:
		return errUpstreamRequestFailed
	}
}

func isSuccessfulUpstreamStatus(status int) bool {
	switch status {
	case http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNonAuthoritativeInfo,
		http.StatusNoContent,
		http.StatusResetContent,
		http.StatusPartialContent,
		http.StatusMultiStatus,
		http.StatusAlreadyReported,
		http.StatusIMUsed:
		return true
	default:
		return false
	}
}

func retryableUpstreamStatus(status int) bool {
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
