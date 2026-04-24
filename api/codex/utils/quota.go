package utils

import (
	"net/http"
	"strconv"
	"time"
)

// Quota 包含从上游获取的配额使用情况和重置时间
type Quota struct {
	Quota5h      float64   // 5h 窗口剩余比率 (0.0–1.0)，无此窗口时为 1.0
	Quota7d      float64   // 7d 窗口剩余比率 (0.0–1.0)，无此窗口时为 1.0
	QuotaSpark5h float64   // Spark 5h 窗口剩余比率 (0.0–1.0)，无此窗口时为 1.0
	QuotaSpark7d float64   // Spark 7d 窗口剩余比率 (0.0–1.0)，无此窗口时为 1.0
	Reset5h      time.Time // 5h 窗口重置绝对时间（零值表示无此窗口）
	Reset7d      time.Time // 7d 窗口重置绝对时间（零值表示无此窗口）
	ResetSpark5h time.Time // Spark 5h 窗口重置绝对时间（零值表示无此窗口）
	ResetSpark7d time.Time // Spark 7d 窗口重置绝对时间（零值表示无此窗口）

	// HasDefaultQuota / HasSparkQuota distinguish partial updates from response
	// headers. When both are false, callers treat the value as a full snapshot
	// for backward compatibility with older construction sites.
	HasDefaultQuota bool
	HasSparkQuota   bool
}

// CodexRateLimit 从上游 x-codex-* 响应头解析的配额元信息
type CodexRateLimit struct {
	PrimaryUsedPercent          int
	PrimaryResetAt              int64
	PrimaryLimitWindowSeconds   int64
	SecondaryUsedPercent        int
	SecondaryResetAt            int64
	SecondaryLimitWindowSeconds int64
}

// ParseRateLimit 遍历响应头一次，将 x-codex-* 字段填入 CodexRateLimit
func ParseRateLimit(h http.Header) *CodexRateLimit {
	rl := &CodexRateLimit{}
	for key, vals := range h {
		if len(vals) == 0 || vals[0] == "" {
			continue
		}
		v := vals[0]
		switch http.CanonicalHeaderKey(key) {
		case "X-Codex-Primary-Used-Percent":
			rl.PrimaryUsedPercent, _ = strconv.Atoi(v)
		case "X-Codex-Secondary-Used-Percent":
			rl.SecondaryUsedPercent, _ = strconv.Atoi(v)
		case "X-Codex-Primary-Reset-At":
			rl.PrimaryResetAt, _ = strconv.ParseInt(v, 10, 64)
		case "X-Codex-Secondary-Reset-At":
			rl.SecondaryResetAt, _ = strconv.ParseInt(v, 10, 64)
		case "X-Codex-Primary-Window-Minutes":
			mins, _ := strconv.ParseInt(v, 10, 64)
			rl.PrimaryLimitWindowSeconds = mins * 60
		case "X-Codex-Secondary-Window-Minutes":
			mins, _ := strconv.ParseInt(v, 10, 64)
			rl.SecondaryLimitWindowSeconds = mins * 60
		}
	}
	return rl
}

func (rl *CodexRateLimit) HasQuotaWindows() bool {
	return rl.PrimaryLimitWindowSeconds != 0 || rl.SecondaryLimitWindowSeconds != 0
}

// ToQuota 根据 LimitWindowSeconds 匹配默认模型窗口类型
func (rl *CodexRateLimit) ToQuota() Quota {
	return rl.ToQuotaForTier("default")
}

// ToQuotaForTier converts response header quota into a partial update for the
// model tier that produced the response.
func (rl *CodexRateLimit) ToQuotaForTier(modelTier string) Quota {
	q := Quota{Quota5h: 1.0, Quota7d: 1.0, QuotaSpark5h: 1.0, QuotaSpark7d: 1.0}
	updateSpark := modelTier == "spark"
	if rl.HasQuotaWindows() {
		if updateSpark {
			q.HasSparkQuota = true
		} else {
			q.HasDefaultQuota = true
		}
	}
	type window struct {
		usedPercent        int
		resetAt            int64
		limitWindowSeconds int64
	}
	for _, w := range []window{
		{rl.PrimaryUsedPercent, rl.PrimaryResetAt, rl.PrimaryLimitWindowSeconds},
		{rl.SecondaryUsedPercent, rl.SecondaryResetAt, rl.SecondaryLimitWindowSeconds},
	} {
		if w.limitWindowSeconds == 0 {
			continue
		}
		remaining := float64(100-w.usedPercent) / 100
		switch w.limitWindowSeconds {
		case int64((5 * time.Hour).Seconds()): // 18000
			if updateSpark {
				q.QuotaSpark5h = remaining
				if w.resetAt > 0 {
					q.ResetSpark5h = time.Unix(w.resetAt, 0)
				}
			} else {
				q.Quota5h = remaining
				if w.resetAt > 0 {
					q.Reset5h = time.Unix(w.resetAt, 0)
				}
			}
		case int64((7 * 24 * time.Hour).Seconds()): // 604800
			if updateSpark {
				q.QuotaSpark7d = remaining
				if w.resetAt > 0 {
					q.ResetSpark7d = time.Unix(w.resetAt, 0)
				}
			} else {
				q.Quota7d = remaining
				if w.resetAt > 0 {
					q.Reset7d = time.Unix(w.resetAt, 0)
				}
			}
		}
	}
	return q
}
