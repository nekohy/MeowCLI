package utils

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ParseRetryAfterHeader parses the Retry-After header into a duration.
// Returns 0 if the header is missing or invalid.
func ParseRetryAfterHeader(headers http.Header) time.Duration {
	if headers == nil {
		return 0
	}
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// CalcBackoff returns base × 2^(consecutive-1), capped at max.
func CalcBackoff(consecutive int, base, max time.Duration) time.Duration {
	if consecutive <= 0 {
		consecutive = 1
	}
	if base <= 0 {
		base = time.Minute
	}
	if max < base {
		max = 30 * time.Minute
	}
	backoff := time.Duration(math.Pow(2, float64(consecutive-1))) * base
	if backoff > max {
		return max
	}
	return backoff
}

// ParseDelimitedList splits a comma/semicolon/space-separated string,
// normalizes each element with the provided function, and deduplicates.
func ParseDelimitedList(raw string, normalize func(string) string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	})
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		normalized := normalize(part)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

// JoinNormalizedList normalizes a list of strings with the provided function,
// deduplicates, and joins with commas.
func JoinNormalizedList(items []string, normalize func(string) string) string {
	normalized := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		n := normalize(item)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		normalized = append(normalized, n)
	}
	return strings.Join(normalized, ",")
}
