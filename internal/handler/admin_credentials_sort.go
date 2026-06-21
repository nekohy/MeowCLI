package handler

import (
	"sort"
	"strings"
	"time"

	"github.com/nekohy/MeowCLI/core/scheduling"
)

type credentialSortOptions struct {
	By    string
	Order string
}

var codexCredentialSortKeys = stringSet(
	"default_score",
	"spark_score",
	"default_error_rate",
	"spark_error_rate",
	"default_quota_5h",
	"default_quota_7d",
	"default_quota_1mo",
	"spark_quota_5h",
	"spark_quota_7d",
	"spark_quota_1mo",
	"default_throttled_until",
	"spark_throttled_until",
)

var geminiCredentialSortKeys = stringSet(
	"pro_score",
	"flash_score",
	"flashlite_score",
	"pro_error_rate",
	"flash_error_rate",
	"flashlite_error_rate",
	"pro_quota",
	"flash_quota",
	"flashlite_quota",
	"pro_throttled_until",
	"flash_throttled_until",
	"flashlite_throttled_until",
)

var antigravityCredentialSortKeys = stringSet(
	"claude_score",
	"pro_score",
	"flash_score",
	"flashlite_score",
	"tab_score",
	"image_score",
	"claude_error_rate",
	"pro_error_rate",
	"flash_error_rate",
	"flashlite_error_rate",
	"tab_error_rate",
	"image_error_rate",
	"claude_quota",
	"pro_quota",
	"flash_quota",
	"flashlite_quota",
	"tab_quota",
	"image_quota",
	"claude_throttled_until",
	"pro_throttled_until",
	"flash_throttled_until",
	"flashlite_throttled_until",
	"tab_throttled_until",
	"image_throttled_until",
)

func credentialSortOptionsFromRequest(query func(string) string, supported map[string]struct{}) credentialSortOptions {
	by := strings.ToLower(strings.TrimSpace(query("sort_by")))
	if by != "" {
		if _, ok := supported[by]; !ok {
			by = ""
		}
	}
	order := strings.ToLower(strings.TrimSpace(query("sort_order")))
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	return credentialSortOptions{
		By:    by,
		Order: order,
	}
}

func (o credentialSortOptions) enabled() bool {
	return o.By != ""
}

func sortCodexListItems(items []codexListItem, options credentialSortOptions) {
	value := func(item codexListItem) (float64, bool) {
		switch options.By {
		case "default_score":
			return adjustedMetricScore(item.Default.Score, item.Default.Weight), true
		case "spark_score":
			return adjustedMetricScore(item.Spark.Score, item.Spark.Weight), true
		case "default_error_rate":
			return errorRateFromMetricWeight(item.Default.Weight), true
		case "spark_error_rate":
			return errorRateFromMetricWeight(item.Spark.Weight), true
		case "default_quota_5h":
			return item.Default.Quota5h, true
		case "default_quota_7d":
			return item.Default.Quota7d, true
		case "default_quota_1mo":
			return item.Default.Quota1mo, true
		case "spark_quota_5h":
			return item.Spark.Quota5h, true
		case "spark_quota_7d":
			return item.Spark.Quota7d, true
		case "spark_quota_1mo":
			return item.Spark.Quota1mo, true
		case "default_throttled_until":
			return timeSortValue(item.Default.ThrottledUntil), true
		case "spark_throttled_until":
			return timeSortValue(item.Spark.ThrottledUntil), true
		default:
			return 0, false
		}
	}
	sortCredentialItems(items, func(item codexListItem) string { return item.ID }, value, options.Order)
}

func sortGeminiListItems(items []geminiListItem, options credentialSortOptions) {
	value := func(item geminiListItem) (float64, bool) {
		switch options.By {
		case "pro_score":
			return adjustedMetricScore(item.Pro.Score, item.Pro.Weight), true
		case "flash_score":
			return adjustedMetricScore(item.Flash.Score, item.Flash.Weight), true
		case "flashlite_score":
			return adjustedMetricScore(item.Flashlite.Score, item.Flashlite.Weight), true
		case "pro_error_rate":
			return errorRateFromMetricWeight(item.Pro.Weight), true
		case "flash_error_rate":
			return errorRateFromMetricWeight(item.Flash.Weight), true
		case "flashlite_error_rate":
			return errorRateFromMetricWeight(item.Flashlite.Weight), true
		case "pro_quota":
			return item.Pro.Quota, true
		case "flash_quota":
			return item.Flash.Quota, true
		case "flashlite_quota":
			return item.Flashlite.Quota, true
		case "pro_throttled_until":
			return timeSortValue(item.Pro.ThrottledUntil), true
		case "flash_throttled_until":
			return timeSortValue(item.Flash.ThrottledUntil), true
		case "flashlite_throttled_until":
			return timeSortValue(item.Flashlite.ThrottledUntil), true
		default:
			return 0, false
		}
	}
	sortCredentialItems(items, func(item geminiListItem) string { return item.ID }, value, options.Order)
}

func sortAntigravityListItems(items []antigravityListItem, options credentialSortOptions) {
	value := func(item antigravityListItem) (float64, bool) {
		switch options.By {
		case "claude_score":
			return adjustedMetricScore(item.Claude.Score, item.Claude.Weight), true
		case "pro_score":
			return adjustedMetricScore(item.Pro.Score, item.Pro.Weight), true
		case "flash_score":
			return adjustedMetricScore(item.Flash.Score, item.Flash.Weight), true
		case "flashlite_score":
			return adjustedMetricScore(item.Flashlite.Score, item.Flashlite.Weight), true
		case "tab_score":
			return adjustedMetricScore(item.Tab.Score, item.Tab.Weight), true
		case "image_score":
			return adjustedMetricScore(item.Image.Score, item.Image.Weight), true
		case "claude_error_rate":
			return errorRateFromMetricWeight(item.Claude.Weight), true
		case "pro_error_rate":
			return errorRateFromMetricWeight(item.Pro.Weight), true
		case "flash_error_rate":
			return errorRateFromMetricWeight(item.Flash.Weight), true
		case "flashlite_error_rate":
			return errorRateFromMetricWeight(item.Flashlite.Weight), true
		case "tab_error_rate":
			return errorRateFromMetricWeight(item.Tab.Weight), true
		case "image_error_rate":
			return errorRateFromMetricWeight(item.Image.Weight), true
		case "claude_quota":
			return item.Claude.Quota, true
		case "pro_quota":
			return item.Pro.Quota, true
		case "flash_quota":
			return item.Flash.Quota, true
		case "flashlite_quota":
			return item.Flashlite.Quota, true
		case "tab_quota":
			return item.Tab.Quota, true
		case "image_quota":
			return item.Image.Quota, true
		case "claude_throttled_until":
			return timeSortValue(item.Claude.ThrottledUntil), true
		case "pro_throttled_until":
			return timeSortValue(item.Pro.ThrottledUntil), true
		case "flash_throttled_until":
			return timeSortValue(item.Flash.ThrottledUntil), true
		case "flashlite_throttled_until":
			return timeSortValue(item.Flashlite.ThrottledUntil), true
		case "tab_throttled_until":
			return timeSortValue(item.Tab.ThrottledUntil), true
		case "image_throttled_until":
			return timeSortValue(item.Image.ThrottledUntil), true
		default:
			return 0, false
		}
	}
	sortCredentialItems(items, func(item antigravityListItem) string { return item.ID }, value, options.Order)
}

func sortCredentialItems[T any](items []T, id func(T) string, value func(T) (float64, bool), order string) {
	sort.SliceStable(items, func(i, j int) bool {
		left, ok := value(items[i])
		right, _ := value(items[j])
		if !ok {
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

func timeSortValue(value *time.Time) float64 {
	if value == nil || value.IsZero() {
		return 0
	}
	return float64(value.UnixNano())
}

func paginateCodexListItems(items []codexListItem, page, pageSize int) []codexListItem {
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
