package completion

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/tidwall/gjson"
)

// TranslateNonStream 消费上游 SSE，提取最终 completed 事件并返回 JSON 响应
func TranslateNonStream(resp *http.Response) (*http.Response, error) {
	if resp == nil || resp.Body == nil {
		return resp, nil
	}
	body, err := readCodexCompletionNonStreamResponse(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()

	cloned := resp.Header.Clone()
	cloned.Set("Content-Type", "application/json")
	cloned.Del("Content-Length")
	resp.Header = cloned
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp, nil
}

func readCodexCompletionNonStreamResponse(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 20_971_520)

	var outputItems []json.RawMessage
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if isSSEFrameLine(line) {
			continue
		}
		line = sseDataPayload(line)
		switch gjson.GetBytes(line, "type").String() {
		case "response.output_item.done":
			if item := gjson.GetBytes(line, "item"); item.Exists() && item.Type == gjson.JSON {
				outputItems = append(outputItems, json.RawMessage(item.Raw))
			}
		case "response.completed":
			event, err := patchCodexCompletedOutput(line, outputItems)
			if err != nil {
				return nil, err
			}
			return convertCodexCompletionResponseToOpenAINonStream(event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.ErrUnexpectedEOF
}

// patchCodexCompletedOutput 用 sonic AST 回填 completed.response.output，避免 sjson 多次动态 patch
func patchCodexCompletedOutput(event []byte, outputItems []json.RawMessage) ([]byte, error) {
	if len(outputItems) == 0 {
		return event, nil
	}
	rawOutput, err := sonic.Marshal(outputItems)
	if err != nil {
		return nil, err
	}
	var root ast.Node
	if err := root.UnmarshalJSON(event); err != nil {
		return nil, err
	}
	var output ast.Node
	if err := output.UnmarshalJSON(rawOutput); err != nil {
		return nil, err
	}
	if _, err := root.Set("response.output", output); err != nil {
		return nil, err
	}
	out, err := root.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func NewStreamReadCloser(ctx context.Context, body io.ReadCloser, model string) io.ReadCloser {
	reader, writer := io.Pipe()
	rc := &completionStreamReadCloser{PipeReader: reader, upstream: body}
	go func() {
		defer body.Close()
		var state *completionResponseState
		completed := false
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 20_971_520)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				_ = writer.CloseWithError(ctx.Err())
				return
			default:
			}
			line := bytes.TrimSpace(scanner.Bytes())
			if isSSEFrameLine(line) {
				continue
			}
			line = sseDataPayload(line)
			chunks, done, err := convertStreamResponseToCompletions(line, &state)
			if err != nil {
				_ = writer.CloseWithError(err)
				return
			}
			if done {
				completed = true
			}
			for _, chunk := range chunks {
				if len(chunk) == 0 {
					continue
				}
				if _, err := writer.Write([]byte("data: ")); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
				if _, err := writer.Write(chunk); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
				if _, err := writer.Write([]byte("\n\n")); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		if !completed {
			_ = writer.CloseWithError(io.ErrUnexpectedEOF)
			return
		}
		_, _ = writer.Write([]byte("data: [DONE]\n\n"))
		_ = writer.Close()
	}()
	return rc
}

func isSSEFrameLine(line []byte) bool {
	trimmed := strings.TrimSpace(string(line))
	return trimmed == "" || strings.HasPrefix(trimmed, "event:")
}

type completionStreamReadCloser struct {
	*io.PipeReader
	upstream io.Closer
}

func (r *completionStreamReadCloser) Close() error {
	if r.upstream != nil {
		_ = r.upstream.Close()
	}
	return r.PipeReader.Close()
}
