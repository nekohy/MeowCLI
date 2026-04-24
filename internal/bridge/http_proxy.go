package bridge

import (
	"net/http"
	"strings"
)

var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func copyUpstreamHeaders(dst, src http.Header, dropContentLength bool) {
	if dst == nil || src == nil {
		return
	}

	skip := make(map[string]struct{}, len(hopByHopHeaders))
	for key := range hopByHopHeaders {
		skip[key] = struct{}{}
	}

	for _, raw := range src.Values("Connection") {
		for _, token := range strings.Split(raw, ",") {
			if token = strings.TrimSpace(token); token != "" {
				skip[http.CanonicalHeaderKey(token)] = struct{}{}
			}
		}
	}

	for key, values := range src {
		canonical := http.CanonicalHeaderKey(key)
		if _, ok := skip[canonical]; ok {
			continue
		}
		if dropContentLength && canonical == "Content-Length" {
			continue
		}
		for _, value := range values {
			dst.Add(canonical, value)
		}
	}
}

func scrubLocalAuthHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	headers.Del("X-Goog-Api-Key")
	headers.Del("X-Api-Key")
}
