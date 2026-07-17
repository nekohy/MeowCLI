package handler

import (
	"sort"
	"strings"
	"time"

	coreantigravity "github.com/nekohy/MeowCLI/core/antigravity"
	corecodex "github.com/nekohy/MeowCLI/core/codex"
	coregemini "github.com/nekohy/MeowCLI/core/gemini"
	coreopencodego "github.com/nekohy/MeowCLI/core/opencodego"
	"github.com/nekohy/MeowCLI/core/scheduling"
)

type credentialSortOptions struct {
	Model  string
	Metric string
	Order  string
}

type credentialSortOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type credentialSortCapabilities struct {
	Models  []credentialSortOption `json:"models"`
	Metrics []credentialSortOption `json:"metrics"`
}

const (
	credentialSortMetricScore     = "score"
	credentialSortMetricErrorRate = "error_rate"
	credentialSortMetricQuota     = "quota"
	credentialSortMetricQuota5h   = "quota_5h"
	credentialSortMetricQuota7d   = "quota_7d"
	credentialSortMetricQuota1mo  = "quota_1mo"
)

var modelQuotaCredentialSortMetrics = []credentialSortOption{
	{Value: credentialSortMetricScore, Label: "Score"},
	{Value: credentialSortMetricErrorRate, Label: "错误率"},
	{Value: credentialSortMetricQuota, Label: "额度"},
}

var codexCredentialSortCapabilities = credentialSortCapabilities{
	Models: []credentialSortOption{
		{Value: corecodex.ModelTierDefault, Label: "Default"},
		{Value: corecodex.ModelTierSpark, Label: "Spark"},
	},
	Metrics: []credentialSortOption{
		{Value: credentialSortMetricScore, Label: "Score"},
		{Value: credentialSortMetricErrorRate, Label: "错误率"},
		{Value: credentialSortMetricQuota5h, Label: "5 小时额度"},
		{Value: credentialSortMetricQuota7d, Label: "7 天额度"},
		{Value: credentialSortMetricQuota1mo, Label: "月额度"},
	},
}

var openCodeGoCredentialSortCapabilities = credentialSortCapabilities{
	Models: []credentialSortOption{
		{Value: coreopencodego.ModelTierDefault, Label: "Default"},
	},
	Metrics: []credentialSortOption{
		{Value: credentialSortMetricScore, Label: "Score"},
		{Value: credentialSortMetricErrorRate, Label: "错误率"},
		{Value: credentialSortMetricQuota5h, Label: "5 小时额度"},
		{Value: credentialSortMetricQuota7d, Label: "7 天额度"},
		{Value: credentialSortMetricQuota1mo, Label: "月额度"},
	},
}

var geminiCredentialSortCapabilities = credentialSortCapabilities{
	Models: []credentialSortOption{
		{Value: coregemini.ModelTierPro, Label: "Pro"},
		{Value: coregemini.ModelTierFlash, Label: "Flash"},
		{Value: coregemini.ModelTierFlashLite, Label: "Lite"},
	},
	Metrics: modelQuotaCredentialSortMetrics,
}

var antigravityCredentialSortCapabilities = credentialSortCapabilities{
	Models: []credentialSortOption{
		{Value: coreantigravity.ModelTierClaude, Label: "Claude"},
		{Value: coreantigravity.ModelTierPro, Label: "Pro"},
		{Value: coreantigravity.ModelTierFlash, Label: "Flash"},
		{Value: coreantigravity.ModelTierFlashLite, Label: "Lite"},
		{Value: coreantigravity.ModelTierTab, Label: "Tab"},
		{Value: coreantigravity.ModelTierImage, Label: "Image"},
	},
	Metrics: modelQuotaCredentialSortMetrics,
}

func credentialSortOptionsFromRequest(query func(string) string, capabilities credentialSortCapabilities) credentialSortOptions {
	model := strings.ToLower(strings.TrimSpace(query("sort_model")))
	metric := strings.ToLower(strings.TrimSpace(query("sort_metric")))
	if !capabilities.supports(model, metric) {
		model = ""
		metric = ""
	}
	order := strings.ToLower(strings.TrimSpace(query("sort_order")))
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	return credentialSortOptions{
		Model:  model,
		Metric: metric,
		Order:  order,
	}
}

func (c credentialSortCapabilities) supports(model, metric string) bool {
	if model == "" || metric == "" {
		return false
	}
	return credentialSortOptionExists(c.Models, model) && credentialSortOptionExists(c.Metrics, metric)
}

func credentialSortOptionExists(options []credentialSortOption, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func (o credentialSortOptions) enabled() bool {
	return o.Model != "" && o.Metric != ""
}

func sortCodexListItems(items []codexListItem, options credentialSortOptions) {
	value := func(item codexListItem) (float64, bool) {
		metric, ok := codexSortMetricForModel(item, options.Model)
		if !ok {
			return 0, false
		}
		return codexSchedulingMetricSortValue(metric, options.Metric)
	}
	sortCredentialItems(items, func(item codexListItem) string { return item.ID }, value, options.Order)
}

func sortOpenCodeGoListItems(items []openCodeGoListItem, options credentialSortOptions) {
	value := func(item openCodeGoListItem) (float64, bool) {
		if options.Model != coreopencodego.ModelTierDefault {
			return 0, false
		}
		return codexSchedulingMetricSortValue(item.Quota, options.Metric)
	}
	sortCredentialItems(items, func(item openCodeGoListItem) string { return item.ID }, value, options.Order)
}

func sortGeminiListItems(items []geminiListItem, options credentialSortOptions) {
	value := func(item geminiListItem) (float64, bool) {
		metric, ok := geminiSortMetricForModel(item, options.Model)
		if !ok {
			return 0, false
		}
		return quotaSchedulingMetricSortValue(metric, options.Metric)
	}
	sortCredentialItems(items, func(item geminiListItem) string { return item.ID }, value, options.Order)
}

func sortAntigravityListItems(items []antigravityListItem, options credentialSortOptions) {
	value := func(item antigravityListItem) (float64, bool) {
		metric, ok := antigravitySortMetricForModel(item, options.Model)
		if !ok {
			return 0, false
		}
		return quotaSchedulingMetricSortValue(metric, options.Metric)
	}
	sortCredentialItems(items, func(item antigravityListItem) string { return item.ID }, value, options.Order)
}

func codexSortMetricForModel(item codexListItem, model string) (codexSchedulingMetric, bool) {
	switch model {
	case corecodex.ModelTierDefault:
		return item.Default, true
	case corecodex.ModelTierSpark:
		return item.Spark, true
	default:
		return codexSchedulingMetric{}, false
	}
}

func geminiSortMetricForModel(item geminiListItem, model string) (quotaSchedulingMetric, bool) {
	switch model {
	case coregemini.ModelTierPro:
		return item.Pro, true
	case coregemini.ModelTierFlash:
		return item.Flash, true
	case coregemini.ModelTierFlashLite:
		return item.Flashlite, true
	default:
		return quotaSchedulingMetric{}, false
	}
}

func antigravitySortMetricForModel(item antigravityListItem, model string) (quotaSchedulingMetric, bool) {
	switch model {
	case coreantigravity.ModelTierClaude:
		return item.Claude, true
	case coreantigravity.ModelTierPro:
		return item.Pro, true
	case coreantigravity.ModelTierFlash:
		return item.Flash, true
	case coreantigravity.ModelTierFlashLite:
		return item.Flashlite, true
	case coreantigravity.ModelTierTab:
		return item.Tab, true
	case coreantigravity.ModelTierImage:
		return item.Image, true
	default:
		return quotaSchedulingMetric{}, false
	}
}

func codexSchedulingMetricSortValue(metric codexSchedulingMetric, sortMetric string) (float64, bool) {
	switch sortMetric {
	case credentialSortMetricScore:
		return adjustedMetricScore(metric.Score, metric.Weight), true
	case credentialSortMetricErrorRate:
		return errorRateFromMetricWeight(metric.Weight), true
	case credentialSortMetricQuota5h:
		return windowedQuotaSortValue(metric.Quota5h, metric.Reset5h)
	case credentialSortMetricQuota7d:
		return windowedQuotaSortValue(metric.Quota7d, metric.Reset7d)
	case credentialSortMetricQuota1mo:
		return windowedQuotaSortValue(metric.Quota1mo, metric.Reset1mo)
	default:
		return 0, false
	}
}

func quotaSchedulingMetricSortValue(metric quotaSchedulingMetric, sortMetric string) (float64, bool) {
	switch sortMetric {
	case credentialSortMetricScore:
		return adjustedMetricScore(metric.Score, metric.Weight), true
	case credentialSortMetricErrorRate:
		return errorRateFromMetricWeight(metric.Weight), true
	case credentialSortMetricQuota:
		return windowedQuotaSortValue(metric.Quota, metric.Reset)
	default:
		return 0, false
	}
}

func sortCredentialItems[T any](items []T, id func(T) string, value func(T) (float64, bool), order string) {
	sort.SliceStable(items, func(i, j int) bool {
		left, leftOK := value(items[i])
		right, rightOK := value(items[j])
		if leftOK != rightOK {
			return leftOK
		}
		if !leftOK {
			return id(items[i]) < id(items[j])
		}
		if left == right {
			return id(items[i]) < id(items[j])
		}
		if order == "asc" {
			return left < right
		}
		return left > right
	})
}

func windowedQuotaSortValue(quota float64, reset time.Time) (float64, bool) {
	return quota, !reset.IsZero()
}

func adjustedMetricScore(score, weight float64) float64 {
	return scheduling.AdjustedScore(score, weight)
}

func errorRateFromMetricWeight(weight float64) float64 {
	rate := 1 - weight
	if rate < 0 {
		return 0
	}
	if rate > 1 {
		return 1
	}
	return rate
}

func paginateCodexListItems(items []codexListItem, page, pageSize int) []codexListItem {
	start, end := paginationBounds(len(items), page, pageSize)
	return items[start:end]
}

func paginateOpenCodeGoListItems(items []openCodeGoListItem, page, pageSize int) []openCodeGoListItem {
	start, end := paginationBounds(len(items), page, pageSize)
	return items[start:end]
}

func paginateGeminiListItems(items []geminiListItem, page, pageSize int) []geminiListItem {
	start, end := paginationBounds(len(items), page, pageSize)
	return items[start:end]
}

func paginateAntigravityListItems(items []antigravityListItem, page, pageSize int) []antigravityListItem {
	start, end := paginationBounds(len(items), page, pageSize)
	return items[start:end]
}

func paginationBounds(length, page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = length
	}
	start := (page - 1) * pageSize
	if start > length {
		start = length
	}
	end := start + pageSize
	if end > length {
		end = length
	}
	return start, end
}

func credentialFetchLimit(total int64) int32 {
	const maxInt32 = int64(1<<31 - 1)
	if total < 0 {
		return 0
	}
	if total > maxInt32 {
		return int32(maxInt32)
	}
	return int32(total)
}
