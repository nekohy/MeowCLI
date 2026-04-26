package bridge

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/nekohy/MeowCLI/api"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
	"github.com/maypok86/otter/v2"
)

type ResolvedModel struct {
	Origin           string
	Handler          utils.HandlerType
	AllowedPlanTypes []string
}

// ModelStore 提供模型别名到上游模型名的映射
type ModelStore interface {
	ResolveModel(ctx context.Context, alias string) (*ResolvedModel, error)
}

type ModelListItem struct {
	Alias   string
	Origin  string
	Handler utils.HandlerType
}

type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelListItem, error)
}

// CredentialScheduler 提供凭证调度与状态记录（每个 HandlerType 独立一个）
type CredentialScheduler interface {
	Pick(ctx context.Context, headers http.Header, preferredCredentialID string, allowedPlanTypes []string) (credentialID string, err error)
	// AuthHeaders 返回该凭证的认证头（如 Authorization, Account-Id 等），由各类型自行实现
	AuthHeaders(ctx context.Context, credentialID string) (http.Header, error)
	RecordSuccess(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics)
	RecordFailure(ctx context.Context, credentialID string, statusCode int32, modelTier string, retryAfter time.Duration, metrics db.LogRequestMetrics)
	HandleUnauthorized(ctx context.Context, credentialID string, statusCode int32, modelTier string, metrics db.LogRequestMetrics) bool
}

type RetryDelayParser interface {
	RetryDelay(statusCode int32, text string, headers http.Header) time.Duration
}

type GraceRetryDecider interface {
	GraceRetry(statusCode int32, text string, retryAfter time.Duration) (time.Duration, bool)
}

type QuotaRefresher interface {
	RefreshQuota(ctx context.Context, credentialID string, modelTier string)
}

// ModelTierPicker is an optional extension of CredentialScheduler that supports
// model-tier-aware credential selection. When a scheduler implements this interface,
// the relay layer will call PickWithTier instead of Pick, passing the model tier
// (e.g., "pro", "flash", "flashlite") so the scheduler can score credentials
// based on the relevant quota fields.
type ModelTierPicker interface {
	PickWithTier(ctx context.Context, headers http.Header, preferredCredentialID string, allowedPlanTypes []string, modelTier string) (credentialID string, err error)
}

// Handler 处理 /v1/responses 请求
type Handler struct {
	backends   map[utils.HandlerType]api.Backend
	models     ModelStore
	schedulers map[utils.HandlerType]CredentialScheduler
	settings   settings.Provider
	sessions   *otter.Cache[string, string]
}

func NewHandler(models ModelStore, schedulers map[utils.HandlerType]CredentialScheduler, backends ...api.Backend) *Handler {
	m := make(map[utils.HandlerType]api.Backend, len(backends))
	for _, b := range backends {
		m[b.HandlerType()] = b
	}
	return &Handler{
		backends:   m,
		models:     models,
		schedulers: schedulers,
		sessions:   newSessionAffinityCache(),
	}
}

func (h *Handler) SetSettingsProvider(provider settings.Provider) {
	if h == nil {
		return
	}
	h.settings = provider
}

func (h *Handler) Route(apiType utils.APIType) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.handle(c, apiType)
	}
}

func (h *Handler) resolveModel(ctx context.Context, alias string) (*ResolvedModel, error) {
	return h.models.ResolveModel(ctx, alias)
}

func (h *Handler) maxRetries() int {
	return h.settingsSnapshot().RelayMaxRetries
}

func (h *Handler) maxAttempts() int {
	retries := h.maxRetries()
	if retries < 0 {
		retries = 0
	}
	return retries + 1
}

func (h *Handler) settingsSnapshot() settings.Snapshot {
	if h == nil || h.settings == nil {
		return settings.DefaultSnapshot()
	}
	return h.settings.Snapshot()
}

func (h *Handler) streamSSE(c *gin.Context, resp *http.Response, backend api.Backend, alias string) error {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Status(resp.StatusCode)

	flusher, _ := c.Writer.(http.Flusher)

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	normalizePayload := alias != "" || backend.HandlerType() == utils.HandlerGemini
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
