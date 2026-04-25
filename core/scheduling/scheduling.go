package scheduling

import (
	"context"
	"net/http"
	"time"
)

const (
	MinErrorRateSamples                 = 1
	ConsecutiveFailureThrottleThreshold = 3
)

type FailureThrottleDecision struct {
	Throttle           bool
	Backoff            time.Duration
	ExplicitRetryAfter bool
	Reason             string
}

type RefreshMode int

const (
	UseCached RefreshMode = iota
	ForceRefresh
)

type CredentialManager interface {
	AccessToken(ctx context.Context, credentialID string, mode RefreshMode) (string, error)
	AuthHeaders(ctx context.Context, credentialID string, mode RefreshMode) (http.Header, error)
	RefreshCredential(ctx context.Context, credentialID string) error
	InvalidateCredential(credentialID string)
}

// CalcWeight returns the error-rate weight multiplier (1.0 - errorRate).
func CalcWeight(errorRate float64) float64 {
	return 1.0 - errorRate
}

// AdjustedScore applies the weight to a raw score.
// If the raw score is negative (unavailable), it is preserved as-is.
func AdjustedScore(score, weight float64) float64 {
	if score < 0 {
		return score
	}
	return score * weight
}

func DecideFailureThrottle(statusCode int32, retryAfter time.Duration, consecutive int, base time.Duration, max time.Duration) FailureThrottleDecision {
	if statusCode == http.StatusTooManyRequests && retryAfter > 0 {
		return FailureThrottleDecision{
			Throttle:           true,
			Backoff:            retryAfter,
			ExplicitRetryAfter: true,
			Reason:             "Retry-After",
		}
	}
	if consecutive < ConsecutiveFailureThrottleThreshold {
		return FailureThrottleDecision{}
	}
	return FailureThrottleDecision{
		Throttle: true,
		Backoff:  calcBackoff(consecutive, base, max),
		Reason:   "consecutive failure threshold",
	}
}

func calcBackoff(consecutive int, base time.Duration, max time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	if consecutive <= 1 {
		return base
	}
	d := base
	for range consecutive - 1 {
		if max > 0 && d >= max/2 {
			return max
		}
		d *= 2
	}
	if max > 0 && d > max {
		return max
	}
	return d
}

// UrgencyFactor returns how urgent a credential window reset is (second-level precision).
// 0.0 = just reset (far from next reset), 1.0 = about to reset or already expired.
func UrgencyFactor(resetAtUnix, nowUnix, windowSeconds int64) float64 {
	if resetAtUnix == 0 || windowSeconds == 0 {
		return 0.0
	}
	remaining := resetAtUnix - nowUnix
	if remaining <= 0 {
		return 1.0
	}
	ratio := float64(remaining) / float64(windowSeconds)
	if ratio >= 1.0 {
		return 0.0
	}
	return 1.0 - ratio
}

func WindowStart(resetAt time.Time, windowSeconds int64) time.Time {
	if resetAt.IsZero() || windowSeconds <= 0 {
		return time.Time{}
	}
	return resetAt.Add(-time.Duration(windowSeconds) * time.Second)
}

func LatestWindowStart(starts ...time.Time) time.Time {
	var latest time.Time
	for _, start := range starts {
		if start.IsZero() {
			continue
		}
		if latest.IsZero() || start.After(latest) {
			latest = start
		}
	}
	return latest
}

// PlanTypeCodeSet converts a list of plan type codes to a set.
// A nil result means all plan types are allowed.
func PlanTypeCodeSet(codes []int) map[int]struct{} {
	if len(codes) == 0 {
		return nil
	}

	set := make(map[int]struct{}, len(codes))
	for _, code := range codes {
		set[code] = struct{}{}
	}
	return set
}

// PlanTypeAllowed checks whether a plan type code is in the allowed set.
// A nil or empty set means all plan types are allowed.
func PlanTypeAllowed(code int, allowed map[int]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[code]
	return ok
}

// MergePlanTypeCodes deduplicates and concatenates multiple plan type code lists.
func MergePlanTypeCodes(groups ...[]int) []int {
	var merged []int
	seen := make(map[int]struct{})

	for _, group := range groups {
		for _, code := range group {
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			merged = append(merged, code)
		}
	}

	return merged
}
