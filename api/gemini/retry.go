package gemini

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nekohy/MeowCLI/utils"
	"github.com/tidwall/gjson"
)

func ParseRetryDelay(errorBody []byte) (*time.Duration, error) {
	details := gjson.GetBytes(errorBody, "error.details")
	if details.Exists() && details.IsArray() {
		for _, detail := range details.Array() {
			if detail.Get("@type").String() == "type.googleapis.com/google.rpc.RetryInfo" {
				retryDelay := detail.Get("retryDelay").String()
				if retryDelay == "" {
					continue
				}
				duration, err := time.ParseDuration(retryDelay)
				if err != nil {
					return nil, fmt.Errorf("failed to parse retryDelay: %w", err)
				}
				return &duration, nil
			}
		}

		for _, detail := range details.Array() {
			if detail.Get("@type").String() == "type.googleapis.com/google.rpc.ErrorInfo" {
				quotaResetDelay := detail.Get("metadata.quotaResetDelay").String()
				if quotaResetDelay == "" {
					continue
				}
				duration, err := time.ParseDuration(quotaResetDelay)
				if err == nil {
					return &duration, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no retry delay found")
}

func ParseRetryDelayText(text string) time.Duration {
	if delay, err := ParseRetryDelay([]byte(text)); err == nil && delay != nil {
		return *delay
	}
	return 0
}

func ParseRetryAfterHeader(headers http.Header) time.Duration {
	return utils.ParseRetryAfterHeader(headers)
}
