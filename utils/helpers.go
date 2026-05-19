package utils

import (
	"io"
	"math"
	"net/http"
	"net/url"
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

func ReadLimitedBody(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}

func CopyHeadersExcept(dst, src http.Header, excluded ...string) {
	if dst == nil || src == nil {
		return
	}
	skip := make(map[string]struct{}, len(excluded))
	for _, key := range excluded {
		skip[http.CanonicalHeaderKey(key)] = struct{}{}
	}
	for key, values := range src {
		if _, ok := skip[http.CanonicalHeaderKey(key)]; ok {
			continue
		}
		if len(values) == 0 {
			continue
		}
		dst[key] = append([]string(nil), values...)
	}
}

func TransformSSEQuery(action, rawQuery string) string {
	query := strings.TrimSpace(rawQuery)
	if action == "streamGenerateContent" && query == "" {
		return "alt=sse"
	}
	return query
}

func NormalizeAllowedKeys(raw string, allowed []string, fallback string) []string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" {
			allowedSet[key] = struct{}{}
		}
	}
	keys := ParseDelimitedList(raw, func(value string) string {
		key := strings.ToLower(strings.TrimSpace(value))
		if _, ok := allowedSet[key]; !ok {
			return ""
		}
		return key
	})
	if len(keys) == 0 {
		fallback = strings.ToLower(strings.TrimSpace(fallback))
		if _, ok := allowedSet[fallback]; ok {
			return []string{fallback}
		}
	}
	return keys
}

func NewProxyHTTPClient(timeout time.Duration, proxy func(*http.Request) (*url.URL, error)) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = timeout
	transport.IdleConnTimeout = timeout
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 20
	transport.Proxy = proxy
	return &http.Client{Transport: transport}
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

// TruncateQuotaRatio clamps a quota ratio to [0, 1] and keeps two decimal
// places by truncation, not rounding.
func TruncateQuotaRatio(value float64) float64 {
	if math.IsNaN(value) || value <= 0 {
		return 0
	}
	if value >= 1 {
		return 1
	}
	return math.Trunc(value*100) / 100
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

func TemporaryThrottleReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "temporary throttle"
	}
	return "temporary throttle: " + reason
}
