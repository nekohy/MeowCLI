package bridge

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) writeUpstreamResponse(c *gin.Context, resp *http.Response, backend api.Backend, alias string, replaceModel bool, streamRequest bool, started time.Time) (responseTiming, error) {
	timedBody := newTimedReadCloser(resp.Body, started)
	resp.Body = timedBody

	responseAlias := ""
	if replaceModel {
		responseAlias = alias
	}
	normalizeGemini := backend.HandlerType() == utils.HandlerGemini || backend.HandlerType() == utils.HandlerAntigravity

	if streamRequest {
		err := h.streamSSE(c, resp, backend, responseAlias)
		return timedBody.timing(), err
	}

	if responseAlias == "" && !normalizeGemini {
		defer func() {
			_ = resp.Body.Close()
		}()

		c.Header("Content-Type", "application/json")
		c.Status(resp.StatusCode)
		_, err := io.CopyBuffer(c.Writer, resp.Body, make([]byte, 32*1024))
		return timedBody.timing(), err
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return timedBody.timing(), err
	}

	bodyBytes = backend.ReplaceModel(bodyBytes, responseAlias)
	c.Data(resp.StatusCode, "application/json", bodyBytes)
	return timedBody.timing(), nil
}

func (h *Handler) streamSSE(c *gin.Context, resp *http.Response, backend api.Backend, alias string) error {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Status(resp.StatusCode)

	flusher, _ := c.Writer.(http.Flusher)

	defer func(Body io.ReadCloser) {
		_, _ = io.Copy(io.Discard, Body)
		_ = Body.Close()
	}(resp.Body)

	// opencode-go 上游在 usage 之后仍会推送额外 SSE 事件，部分客户端无法兼容，
	// 因此转发完包含 usage 的分块后直接以 [DONE] 终止流。
	stopAfterUsage := backend.HandlerType() == utils.HandlerOpenCodeGo
	normalizePayload := alias != "" || stopAfterUsage || backend.HandlerType() == utils.HandlerGemini || backend.HandlerType() == utils.HandlerAntigravity
	if !normalizePayload {
		buf := make([]byte, 32*1024)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
					return writeErr
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		}
	}

	// 需要替换：逐行读取，替换 data 行中的 model 字段
	reader := bufio.NewReaderSize(resp.Body, 32*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if writeErr := writeSSELine(c.Writer, backend, alias, line); writeErr != nil {
				return writeErr
			}
			if stopAfterUsage && sseLineHasUsage(line) {
				if flusher != nil {
					flusher.Flush()
				}
				return finishSSEStream(c.Writer, flusher)
			}
		}
		if flusher != nil {
			flusher.Flush()
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// finishSSEStream 在提前截断上游流时向客户端补齐事件结束空行和标准 [DONE] 标记。
func finishSSEStream(w io.Writer, flusher http.Flusher) error {
	if _, err := w.Write([]byte("\ndata: [DONE]\n\n")); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// sseLineHasUsage 判断 SSE data 行是否携带了真实的 usage 对象（忽略 "usage":null）。
func sseLineHasUsage(line []byte) bool {
	payload, ok := sseDataPayload(bytes.TrimRight(line, "\r\n"))
	if !ok || len(payload) == 0 || payload[0] != '{' {
		return false
	}
	return bytes.Contains(payload, []byte(`"usage":{`)) || bytes.Contains(payload, []byte(`"usage": {`))
}

func writeSSELine(w io.Writer, backend api.Backend, alias string, line []byte) error {
	line = bytes.TrimSuffix(line, []byte("\n"))
	line = bytes.TrimSuffix(line, []byte("\r"))

	if payload, ok := sseDataPayload(line); ok {
		if len(payload) > 0 && payload[0] == '{' {
			replaced := backend.ReplaceModel(payload, alias)
			if _, err := w.Write([]byte("data: ")); err != nil {
				return err
			}
			if _, err := w.Write(replaced); err != nil {
				return err
			}
			_, err := w.Write([]byte("\n"))
			return err
		}
	}

	if _, err := w.Write(line); err != nil {
		return err
	}
	_, err := w.Write([]byte("\n"))
	return err
}

func sseDataPayload(line []byte) ([]byte, bool) {
	switch {
	case bytes.HasPrefix(line, []byte("data: ")):
		return line[6:], true
	case bytes.HasPrefix(line, []byte("data:")):
		return line[5:], true
	default:
		return nil, false
	}
}
