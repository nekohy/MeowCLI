package completion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/tidwall/gjson"
)

func rawJSONMessage(v gjson.Result) json.RawMessage {
	if !v.Exists() {
		return nil
	}
	return json.RawMessage(v.Raw)
}

func marshalRaw(v interface{}) json.RawMessage {
	out, err := sonic.Marshal(v)
	if err != nil {
		return nil
	}
	return out
}

func appendRaw[T interface{}](items []json.RawMessage, v T) []json.RawMessage {
	raw := marshalRaw(v)
	if len(raw) == 0 {
		return items
	}
	return append(items, raw)
}

func ptr[T interface{}](v T) *T {
	return &v
}

// sseDataPayload strips the "data:" or "data: " SSE prefix and returns the
// payload bytes. If the line doesn't start with "data:", it is returned as-is.
func sseDataPayload(line []byte) []byte {
	if bytes.HasPrefix(line, []byte("data: ")) {
		return line[6:]
	}
	if bytes.HasPrefix(line, []byte("data:")) {
		return line[5:]
	}
	return line
}

func mimeTypeFromCodexOutputFormat(outputFormat string) (string, error) {
	if outputFormat == "" {
		return "", fmt.Errorf("missing image output_format")
	}
	if strings.Contains(outputFormat, "/") {
		return outputFormat, nil
	}
	switch strings.ToLower(outputFormat) {
	case "png":
		return "image/png", nil
	case "jpg", "jpeg":
		return "image/jpeg", nil
	case "webp":
		return "image/webp", nil
	case "gif":
		return "image/gif", nil
	default:
		return "", fmt.Errorf("unsupported image output_format %q", outputFormat)
	}
}
