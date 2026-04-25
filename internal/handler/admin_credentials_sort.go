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

var codexCredentialSortKeys = map[string]struct{}{
	"default_score":      {},
	"spark_score":        {},
	"default_error_rate": {},
	"spark_error_rate":   {},
	"default_quota_5h":   {},
	"default_quota_7d":   {},
	"spark_quota_5h":     {},
	"spark_quota_7d":     {},
	"throttled_until":    {},
}

var geminiCredentialSortKeys = map[string]struct{}{
	"score":                {},
	"pro_score":            {},
	"flash_score":          {},
	"flashlite_score":      {},
	"lite_score":           {},
	"error_rate":           {},
	"pro_error_rate":       {},
	"flash_error_rate":     {},
	"flashlite_error_rate": {},
	"lite_error_rate":      {},
	"quota":                {},
	"pro_quota":            {},
	"flash_quota":          {},
	"flashlite_quota":      {},
	"lite_quota":           {},
	"throttled_until":      {},
}

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
			return adjustedMetricScore(item.Default), true
		case "spark_score":
			return adjustedMetricScore(item.Spark), true
		case "default_error_rate":
			return errorRateFromMetricWeight(item.Default.Weight), true
		case "spark_error_rate":
			return errorRateFromMetricWeight(item.Spark.Weight), true
		case "default_quota_5h":
			return item.Default.Quota5h, true
		case "default_quota_7d":
			return item.Default.Quota7d, true
		case "spark_quota_5h":
			return item.Spark.Quota5h, true
		case "spark_quota_7d":
			return item.Spark.Quota7d, true
		case "throttled_until":
			return timeSortValue(item.ThrottledUntil), true
		default:
			return 0, false
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, ok := value(items[i])
		right, _ := value(items[j])
		if !ok {
			return items[i].ID < items[j].ID
		}
		if left == right {
			return items[i].ID < items[j].ID
		}
		if options.Order == "asc" {
			return left < right
		}
		return left > right
	})
}

func sortGeminiListItems(items []geminiListItem, options credentialSortOptions) {
	value := func(item geminiListItem) (float64, bool) {
		switch options.By {
		case "score", "pro_score":
			return adjustedMetricScore(item.Pro), true
		case "flash_score":
			return adjustedMetricScore(item.Flash), true
		case "flashlite_score", "lite_score":
			return adjustedMetricScore(item.Flashlite), true
		case "error_rate", "pro_error_rate":
			return errorRateFromMetricWeight(item.Pro.Weight), true
		case "flash_error_rate":
			return errorRateFromMetricWeight(item.Flash.Weight), true
		case "flashlite_error_rate", "lite_error_rate":
			return errorRateFromMetricWeight(item.Flashlite.Weight), true
		case "quota", "pro_quota":
			return item.Pro.Quota, true
		case "flash_quota":
			return item.Flash.Quota, true
		case "flashlite_quota", "lite_quota":
			return item.Flashlite.Quota, true
		case "throttled_until":
			return timeSortValue(item.ThrottledUntil), true
		default:
			return 0, false
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, ok := value(items[i])
		right, _ := value(items[j])
		if !ok {
			return items[i].ID < items[j].ID
		}
		if left == right {
			return items[i].ID < items[j].ID
		}
		if options.Order == "asc" {
			return left < right
		}
		return left > right
	})
}

type schedulingMetric interface {
	scoreValue() float64
	weightValue() float64
}

func adjustedMetricScore(metric schedulingMetric) float64 {
	return scheduling.AdjustedScore(metric.scoreValue(), metric.weightValue())
}

func (m codexSchedulingMetric) scoreValue() float64  { return m.Score }
func (m codexSchedulingMetric) weightValue() float64 { return m.Weight }
func (m geminiSchedulingMetric) scoreValue() float64 { return m.Score }
func (m geminiSchedulingMetric) weightValue() float64 {
	return m.Weight
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

func timeSortValue(value time.Time) float64 {
	if value.IsZero() {
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
