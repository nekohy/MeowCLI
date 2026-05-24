package handler

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type throttleStatusDeadline struct {
	Tier     string
	Deadline time.Time
}

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func credentialStatusesFromRequest(c *gin.Context, throttleTiers map[string]struct{}) []string {
	values := c.QueryArray("status")
	if len(values) == 0 {
		values = []string{c.Query("status")}
	}

	statuses := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			status := strings.ToLower(strings.TrimSpace(part))
			if !validCredentialStatusFilter(status, throttleTiers) {
				continue
			}
			if _, ok := seen[status]; ok {
				continue
			}
			seen[status] = struct{}{}
			statuses = append(statuses, status)
		}
	}
	return statuses
}

func validCredentialStatusFilter(status string, throttleTiers map[string]struct{}) bool {
	switch status {
	case "enabled", "disabled":
		return true
	}
	tier, ok := strings.CutPrefix(status, "throttled:")
	if !ok {
		return false
	}
	if tier == "all" {
		return true
	}
	_, ok = throttleTiers[tier]
	return ok
}

func credentialStatusList(baseStatus string, deadlines ...throttleStatusDeadline) []string {
	now := time.Now()
	statuses := []string{baseStatus}
	for _, deadline := range deadlines {
		if !deadline.Deadline.After(now) {
			continue
		}
		statuses = append(statuses, throttledStatus(deadline.Tier))
	}
	return statuses
}

func baseCredentialStatus(statuses []string) string {
	for _, status := range statuses {
		if !strings.HasPrefix(status, "throttled:") {
			return status
		}
	}
	return ""
}

func activeThrottleDeadline(value time.Time) *time.Time {
	if !value.After(time.Now()) {
		return nil
	}
	return &value
}

func throttleDeadlineValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func throttledStatus(tier string) string {
	return "throttled:" + tier
}
