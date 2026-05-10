package handler

import (
	corecodex "github.com/nekohy/MeowCLI/core/codex"
	coregemini "github.com/nekohy/MeowCLI/core/gemini"
)

type quotaCacheOverlay interface {
	CachedCodexQuota(id string) (corecodex.CachedQuotaSnapshot, bool)
	CachedGeminiQuota(id string) (coregemini.CachedQuotaSnapshot, bool)
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
	}
}

func codexMetricFromCache(metric corecodex.CachedQuotaMetric) codexSchedulingMetric {
	return codexSchedulingMetric{
		Available: metric.Available,
		Quota5h:   metric.Quota5h,
		Quota7d:   metric.Quota7d,
		Reset5h:   metric.Reset5h,
		Reset7d:   metric.Reset7d,
		Score:     metric.Score,
		Weight:    metric.Weight,
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
	}
}

func geminiMetricFromCache(metric coregemini.CachedQuotaMetric) geminiSchedulingMetric {
	return geminiSchedulingMetric{
		Available: metric.Available,
		Quota:     metric.Quota,
		Reset:     metric.Reset,
		Score:     metric.Score,
		Weight:    metric.Weight,
	}
}
