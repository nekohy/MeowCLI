package scheduling

import (
	"context"
	"math"
	"net/http"
	"time"

	weightedrand "github.com/mroth/weightedrand/v3"
)

const (
	// MinErrorRateSamples 计算错误率时需要的最少样本数
	MinErrorRateSamples = 1
	// ConsecutiveFailureThrottleThreshold 连续失败达到该次数后进入退避节流
	ConsecutiveFailureThrottleThreshold = 3
	// DefaultWeightedTopK 高并发调度时进入加权随机池的最高分候选数量
	DefaultWeightedTopK = 10
	// MaxQuotaPressure 限制临近刷新时的压力分数上限，避免单个账号垄断流量
	MaxQuotaPressure = 10.0
	// quotaPressureEpsilon 防止剩余窗口时间为 0 时除零
	quotaPressureEpsilon = 1e-6
	// multiWindowShortWeight 多窗口评分中短窗口压力的权重
	multiWindowShortWeight = 0.8
	// multiWindowLongWeight 多窗口评分中长窗口压力的权重
	multiWindowLongWeight = 0.2
	// multiWindowLongConstraintRatio 长窗口压力对短窗口压力的约束比例
	// 越高5h压力越高，越低7d约束越强
	multiWindowLongConstraintRatio = 1
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

// CalcWeight 返回错误率权重乘数，错误率越高权重越低
func CalcWeight(errorRate float64) float64 {
	return 1.0 - errorRate
}

// AdjustedScore 对原始分数应用权重
// 如果原始分数为负数（不可用），则保持原值
func AdjustedScore(score, weight float64) float64 {
	if score < 0 {
		return score
	}
	return score * weight
}

// QuotaPressureScore 计算单窗口归一化配额压力
// 剩余额度越多、距离刷新越近，压力分数越高
func QuotaPressureScore(quota float64, resetAt time.Time, now time.Time, windowSeconds int64) float64 {
	if quota <= 0 {
		return -1
	}
	remainingFraction := 1.0
	if !resetAt.IsZero() && windowSeconds > 0 {
		remaining := resetAt.Sub(now).Seconds()
		switch {
		case remaining <= 0:
			remainingFraction = quotaPressureEpsilon
		case remaining < float64(windowSeconds):
			remainingFraction = max(remaining/float64(windowSeconds), quotaPressureEpsilon)
		}
	}

	pressure := quota / remainingFraction
	return min(pressure, MaxQuotaPressure)
}

// MultiWindowQuotaPressureScore 计算短窗口受长窗口约束的多窗口压力分数
func MultiWindowQuotaPressureScore(shortQuota, longQuota float64, shortReset, longReset time.Time, shortWindowSeconds, longWindowSeconds int64) float64 {
	now := time.Now()
	longPressure := QuotaPressureScore(longQuota, longReset, now, longWindowSeconds)
	if longPressure < 0 {
		return -1
	}
	if shortReset.IsZero() || shortWindowSeconds <= 0 {
		return multiWindowLongWeight * longPressure
	}

	shortPressure := QuotaPressureScore(shortQuota, shortReset, now, shortWindowSeconds)
	if shortPressure < 0 {
		return -1
	}
	longCap := multiWindowLongConstraintRatio * longPressure
	shortEffective := min(shortPressure, longCap)
	return multiWindowShortWeight*shortEffective + multiWindowLongWeight*longPressure
}

// PickWeightedTopK 从分数最高的前 K 个可用候选中按分数权重随机选择一个
func PickWeightedTopK[T any](items []T, topK int, score func(T) float64) (T, bool) {
	var zero T
	if len(items) == 0 || topK <= 0 {
		return zero, false
	}

	candidates := make([]weightedCandidate[T], 0, min(topK, len(items)))
	for _, item := range items {
		s := score(item)
		if s <= 0 {
			continue
		}
		candidates = insertTopKCandidate(candidates, topK, weightedCandidate[T]{
			item:  item,
			score: s,
			// weightedrand uses integer weights; keep any positive score pickable.
			weight: max(uint64(math.Ceil(s*1_000_000)), 1),
		})
	}
	if len(candidates) == 0 {
		return zero, false
	}

	choices := make([]weightedrand.Choice[T, uint64], 0, len(candidates))
	for _, candidate := range candidates {
		choices = append(choices, weightedrand.NewChoice(candidate.item, candidate.weight))
	}
	chooser, err := weightedrand.NewChooser(choices...)
	if err != nil {
		return zero, false
	}

	return chooser.Pick(), true
}

type weightedCandidate[T any] struct {
	item   T
	score  float64
	weight uint64
}

func insertTopKCandidate[T any](candidates []weightedCandidate[T], topK int, candidate weightedCandidate[T]) []weightedCandidate[T] {
	if len(candidates) < topK {
		candidates = append(candidates, candidate)
	} else if candidate.score <= candidates[len(candidates)-1].score {
		return candidates
	} else {
		candidates[len(candidates)-1] = candidate
	}

	index := len(candidates) - 1
	for index > 0 && candidates[index].score > candidates[index-1].score {
		candidates[index], candidates[index-1] = candidates[index-1], candidates[index]
		index--
	}
	return candidates
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
	backoffAttempt := consecutive - ConsecutiveFailureThrottleThreshold + 1
	return FailureThrottleDecision{
		Throttle: true,
		Backoff:  calcBackoff(backoffAttempt, base, max),
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

// PlanTypeCodeSet 将计划类型编码列表转换为集合
// 返回 nil 表示允许所有计划类型
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

// PlanTypeAllowed 检查计划类型编码是否在允许集合内
// nil 或空集合表示允许所有计划类型
func PlanTypeAllowed(code int, allowed map[int]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[code]
	return ok
}

// MergePlanTypeCodes 对多个计划类型编码列表去重并按顺序合并
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
