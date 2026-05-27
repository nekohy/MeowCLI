package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nekohy/MeowCLI/internal/settings"
	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

const defaultImportConcurrency = 4

var (
	errImportJobNotFound = errors.New("import job not found")
	errImportJobRunning  = errors.New("import job is still running")
)

type importJobStatus string

const (
	importJobStatusRunning   importJobStatus = "running"
	importJobStatusCompleted importJobStatus = "completed"
)

type importJobSnapshot struct {
	ID        string              `json:"id"`
	Handler   string              `json:"handler"`
	Status    importJobStatus     `json:"status"`
	Total     int                 `json:"total"`
	Processed int                 `json:"processed"`
	Succeeded int                 `json:"succeeded"`
	Errors    []map[string]string `json:"error"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type importJobStartedSnapshot struct {
	ID      string          `json:"id"`
	Handler string          `json:"handler"`
	Status  importJobStatus `json:"status"`
	Total   int             `json:"total"`
}

type importJob struct {
	mu        sync.Mutex
	snapshot  importJobSnapshot
	workerCap int
}

type importProcessor func(context.Context, string) (string, error)
type importCreatedHook func(string)

type importJobManager struct {
	mu          sync.RWMutex
	jobs        map[string]*importJob
	concurrency int
	settings    settings.Provider
}

func newImportJobManager(concurrency int) *importJobManager {
	if concurrency < 1 {
		concurrency = defaultImportConcurrency
	}
	return &importJobManager{
		jobs:        make(map[string]*importJob),
		concurrency: concurrency,
	}
}

func (m *importJobManager) SetSettingsProvider(provider settings.Provider) {
	m.mu.Lock()
	m.settings = provider
	m.mu.Unlock()
}

func (m *importJobManager) Start(ctx context.Context, handler utils.HandlerType, tokens []string, process importProcessor, onCreated importCreatedHook) importJobSnapshot {
	if m == nil {
		m = newImportJobManager(defaultImportConcurrency)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cleaned := normalizeImportTokens(tokens)
	now := time.Now()
	job := &importJob{
		workerCap: m.concurrencyLimit(),
		snapshot: importJobSnapshot{
			ID:        newImportJobID(),
			Handler:   string(handler),
			Status:    importJobStatusRunning,
			Total:     len(cleaned),
			Errors:    []map[string]string{},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	m.mu.Lock()
	m.jobs[job.snapshot.ID] = job
	m.mu.Unlock()

	if len(cleaned) == 0 {
		job.finish()
		return job.Snapshot()
	}

	go job.run(ctx, cleaned, process, onCreated)
	return job.Snapshot()
}

func (m *importJobManager) concurrencyLimit() int {
	m.mu.RLock()
	provider := m.settings
	fallback := m.concurrency
	m.mu.RUnlock()
	if fallback < 1 {
		fallback = defaultImportConcurrency
	}
	if provider == nil {
		return fallback
	}
	configured := provider.Snapshot().ImportConcurrency
	if configured < 1 {
		return fallback
	}
	return configured
}

func (m *importJobManager) List() []importJobSnapshot {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	jobs := make([]*importJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job)
	}
	m.mu.RUnlock()

	snapshots := make([]importJobSnapshot, 0, len(jobs))
	for _, job := range jobs {
		snapshots = append(snapshots, job.Snapshot())
	}
	return snapshots
}

func (m *importJobManager) Acknowledge(id string) error {
	if m == nil {
		return errImportJobNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errImportJobNotFound
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	job, ok := m.jobs[id]
	if !ok {
		return errImportJobNotFound
	}

	if job.Snapshot().Status != importJobStatusCompleted {
		return errImportJobRunning
	}

	delete(m.jobs, id)
	return nil
}

func (j *importJob) run(ctx context.Context, tokens []string, process importProcessor, onCreated importCreatedHook) {
	if process == nil {
		j.failAll(tokens)
		j.finish()
		return
	}

	workerCount := j.workerCap
	if workerCount > len(tokens) {
		workerCount = len(tokens)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	tokenCh := make(chan string)
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()
			for token := range tokenCh {
				id, err := process(ctx, token)
				if err != nil {
					j.recordError(token, err)
					continue
				}
				j.recordCreated()
				if onCreated != nil {
					onCreated(id)
				}
			}
		}()
	}

	for _, token := range tokens {
		tokenCh <- token
	}
	close(tokenCh)
	wg.Wait()
	j.finish()
}

func (j *importJob) Snapshot() importJobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.snapshot
}

func (j *importJob) recordCreated() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.snapshot.Processed++
	j.snapshot.Succeeded++
	j.snapshot.UpdatedAt = time.Now()
}

func (j *importJob) recordError(input string, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.snapshot.Processed++
	if err != nil {
		j.snapshot.Errors = append(j.snapshot.Errors, map[string]string{input: err.Error()})
	}
	j.snapshot.UpdatedAt = time.Now()
}

func (j *importJob) failAll(tokens []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for range tokens {
		j.snapshot.Processed++
	}
	j.snapshot.UpdatedAt = time.Now()
}

func (j *importJob) finish() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.snapshot.Status = importJobStatusCompleted
	j.snapshot.UpdatedAt = time.Now()
}

func importJobStartResponse(snapshot importJobSnapshot) importJobStartedSnapshot {
	return importJobStartedSnapshot{
		ID:      snapshot.ID,
		Handler: snapshot.Handler,
		Status:  snapshot.Status,
		Total:   snapshot.Total,
	}
}

func normalizeImportTokens(tokens []string) []string {
	cleaned := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			cleaned = append(cleaned, token)
		}
	}
	return cleaned
}

func newImportJobID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (a *AdminHandler) ListJobs(c *gin.Context) {
	if a == nil || a.importJobs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "import job manager is unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": a.importJobs.List()})
}

func (a *AdminHandler) AcknowledgeJob(c *gin.Context) {
	if a == nil || a.importJobs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "import job manager is unavailable"})
		return
	}

	if err := a.importJobs.Acknowledge(c.Param("id")); err != nil {
		switch {
		case errors.Is(err, errImportJobNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		case errors.Is(err, errImportJobRunning):
			c.JSON(http.StatusConflict, gin.H{"error": "job is still running"})
		default:
			writeInternalError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
