package handler

import (
	coreantigravity "github.com/nekohy/MeowCLI/core/antigravity"
	corecodex "github.com/nekohy/MeowCLI/core/codex"
	coregemini "github.com/nekohy/MeowCLI/core/gemini"
)

type quotaCacheOverlay interface {
	CachedCodexQuota(id string) (corecodex.CachedQuotaSnapshot, bool)
	CachedGeminiQuota(id string) (coregemini.CachedQuotaSnapshot, bool)
	CachedAntigravityQuota(id string) (coreantigravity.CachedQuotaSnapshot, bool)
}

func overlayCodexQuotaCache(items []codexListItem, overlay quotaCacheOverlay) {
	if overlay == nil {
		return
	}
	for i := range items {
		cached, ok := overlay.CachedCodexQuota(items[i].ID)
		if !ok {
			continue
		}
		items[i].Default = codexMetricFromCache(cached.Default)
		items[i].Spark = codexMetricFromCache(cached.Spark)
		items[i].Status = credentialStatusList(baseCredentialStatus(items[i].Status),
			throttleStatusDeadline{Tier: corecodex.ModelTierDefault, Deadline: throttleDeadlineValue(items[i].Default.ThrottledUntil)},
			throttleStatusDeadline{Tier: corecodex.ModelTierSpark, Deadline: throttleDeadlineValue(items[i].Spark.ThrottledUntil)},
		)
	}
}

func codexMetricFromCache(metric corecodex.CachedQuotaMetric) codexSchedulingMetric {
	return codexSchedulingMetric{
		Available:      metric.Available,
		Quota5h:        metric.Quota5h,
		Quota7d:        metric.Quota7d,
		Quota1mo:       metric.Quota1mo,
		Reset5h:        metric.Reset5h,
		Reset7d:        metric.Reset7d,
		Reset1mo:       metric.Reset1mo,
		ThrottledUntil: activeThrottleDeadline(metric.ThrottledUntil),
		Score:          metric.Score,
		Weight:         metric.Weight,
	}
}

func overlayGeminiQuotaCache(items []geminiListItem, overlay quotaCacheOverlay) {
	if overlay == nil {
		return
	}
	for i := range items {
		cached, ok := overlay.CachedGeminiQuota(items[i].ID)
		if !ok {
			continue
		}
		items[i].Pro = geminiMetricFromCache(cached.Pro)
		items[i].Flash = geminiMetricFromCache(cached.Flash)
		items[i].Flashlite = geminiMetricFromCache(cached.FlashLite)
		items[i].Status = credentialStatusList(baseCredentialStatus(items[i].Status),
			throttleStatusDeadline{Tier: coregemini.ModelTierPro, Deadline: throttleDeadlineValue(items[i].Pro.ThrottledUntil)},
			throttleStatusDeadline{Tier: coregemini.ModelTierFlash, Deadline: throttleDeadlineValue(items[i].Flash.ThrottledUntil)},
			throttleStatusDeadline{Tier: coregemini.ModelTierFlashLite, Deadline: throttleDeadlineValue(items[i].Flashlite.ThrottledUntil)},
		)
	}
}

func geminiMetricFromCache(metric coregemini.CachedQuotaMetric) quotaSchedulingMetric {
	return quotaSchedulingMetric{
		Available:      metric.Available,
		Quota:          metric.Quota,
		Reset:          metric.Reset,
		ThrottledUntil: activeThrottleDeadline(metric.ThrottledUntil),
		Score:          metric.Score,
		Weight:         metric.Weight,
	}
}

func overlayAntigravityQuotaCache(items []antigravityListItem, overlay quotaCacheOverlay) {
	if overlay == nil {
		return
	}
	for i := range items {
		cached, ok := overlay.CachedAntigravityQuota(items[i].ID)
		if !ok {
			continue
		}
		items[i].Claude = antigravityMetricFromCache(cached.Claude)
		items[i].Pro = antigravityMetricFromCache(cached.Pro)
		items[i].Flash = antigravityMetricFromCache(cached.Flash)
		items[i].Flashlite = antigravityMetricFromCache(cached.FlashLite)
		items[i].Tab = antigravityMetricFromCache(cached.Tab)
		items[i].Image = antigravityMetricFromCache(cached.Image)
		items[i].Status = credentialStatusList(baseCredentialStatus(items[i].Status),
			throttleStatusDeadline{Tier: coreantigravity.ModelTierClaude, Deadline: throttleDeadlineValue(items[i].Claude.ThrottledUntil)},
			throttleStatusDeadline{Tier: coreantigravity.ModelTierPro, Deadline: throttleDeadlineValue(items[i].Pro.ThrottledUntil)},
			throttleStatusDeadline{Tier: coreantigravity.ModelTierFlash, Deadline: throttleDeadlineValue(items[i].Flash.ThrottledUntil)},
			throttleStatusDeadline{Tier: coreantigravity.ModelTierFlashLite, Deadline: throttleDeadlineValue(items[i].Flashlite.ThrottledUntil)},
			throttleStatusDeadline{Tier: coreantigravity.ModelTierTab, Deadline: throttleDeadlineValue(items[i].Tab.ThrottledUntil)},
			throttleStatusDeadline{Tier: coreantigravity.ModelTierImage, Deadline: throttleDeadlineValue(items[i].Image.ThrottledUntil)},
		)
	}
}

func antigravityMetricFromCache(metric coreantigravity.CachedQuotaMetric) quotaSchedulingMetric {
	return quotaSchedulingMetric{
		Available:      metric.Available,
		Quota:          metric.Quota,
		Reset:          metric.Reset,
		ThrottledUntil: activeThrottleDeadline(metric.ThrottledUntil),
		Score:          metric.Score,
		Weight:         metric.Weight,
	}
}
