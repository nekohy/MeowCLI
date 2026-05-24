package handler

import "time"

type codexSchedulingMetric struct {
	Available      bool       `json:"available"`
	Quota5h        float64    `json:"quota_5h"`
	Quota7d        float64    `json:"quota_7d"`
	Reset5h        time.Time  `json:"reset_5h"`
	Reset7d        time.Time  `json:"reset_7d"`
	ThrottledUntil *time.Time `json:"throttled_until,omitempty"`
	Score          float64    `json:"score"`
	Weight         float64    `json:"weight"`
}

type quotaSchedulingMetric struct {
	Available      bool       `json:"available"`
	Quota          float64    `json:"quota"`
	Reset          time.Time  `json:"reset"`
	ThrottledUntil *time.Time `json:"throttled_until,omitempty"`
	Score          float64    `json:"score"`
	Weight         float64    `json:"weight"`
}
