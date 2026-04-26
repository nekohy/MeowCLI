package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/nekohy/MeowCLI/api/codex"
	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	"github.com/nekohy/MeowCLI/api/gemini"
	runtimelogs "github.com/nekohy/MeowCLI/internal/logs"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"
	"net/http"
	"strings"
	"sync"
	"time"

	coreCodex "github.com/nekohy/MeowCLI/core/codex"
	coreGemini "github.com/nekohy/MeowCLI/core/gemini"
	"github.com/nekohy/MeowCLI/db/postgres"
	"github.com/nekohy/MeowCLI/db/sqlite"
	"github.com/nekohy/MeowCLI/internal/auth"
	"github.com/nekohy/MeowCLI/internal/bridge"
	"github.com/nekohy/MeowCLI/internal/handler"
	"github.com/nekohy/MeowCLI/internal/router"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	serverReadTimeout       = 30 * time.Second
	serverWriteTimeout      = 10 * time.Minute
	serverIdleTimeout       = 2 * time.Minute
	serverMaxHeaderBytes    = 1 << 20
)

func Run(ctx context.Context, cfg Config) error {
	buildInfo := CurrentBuildInfo()
	log.Info().
		Str("version", buildInfo.Version).
		Str("build_time", buildInfo.BuildTime).
		Str("listen_addr", cfg.ListenAddr).
		Msg("starting MeowCLI")
	log.Info().Str("db_type", cfg.DBType).Str("dsn", RedactedDatabaseURL(cfg.DatabaseURL)).Msg("database config")

	store, err := openStore(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open %s database: %w", cfg.DBType, err)
	}
	defer store.Close()

	// Auth key cache
	authCache := auth.NewKeyCache(store)
	if err := authCache.Load(ctx); err != nil {
		return fmt.Errorf("load auth keys: %w", err)
	}

	settingsSvc, err := settings.NewService(ctx, store)
	if err != nil {
		return fmt.Errorf("load runtime settings: %w", err)
	}
	logStore := runtimelogs.NewStore(settingsSvc)

	codexClient := codex.NewClient()
	codexClient.SetSettingsProvider(settingsSvc)
	codexManager, err := coreCodex.NewManager(coreCodex.ManagerConfig{
		Store:    store,
		CodexAPI: codexClient,
		Settings: settingsSvc,
	})
	if err != nil {
		return fmt.Errorf("init codex manager: %w", err)
	}

	codexScheduler := coreCodex.NewScheduler(store, codexManager)
	codexScheduler.SetSettingsProvider(settingsSvc)
	codexScheduler.SetLogStore(logStore)
	codexScheduler.StartQuotaSyncer(ctx)

	codexClient.OnQuota = func(ctx context.Context, credentialID string, q *codexutils.Quota) {
		codexScheduler.UpdateQuota(ctx, credentialID, q)
	}

	geminiClient := gemini.NewClient()
	geminiClient.SetSettingsProvider(settingsSvc)
	geminiManager, err := coreGemini.NewManager(coreGemini.ManagerConfig{
		Store:    store,
		Client:   geminiClient,
		Settings: settingsSvc,
	})
	if err != nil {
		return fmt.Errorf("init gemini manager: %w", err)
	}
	geminiScheduler := coreGemini.NewScheduler(store, geminiManager)
	geminiScheduler.SetSettingsProvider(settingsSvc)
	geminiScheduler.SetLogStore(logStore)
	geminiScheduler.SetQuotaFetcher(geminiClient)
	geminiScheduler.StartQuotaSyncer(ctx)

	modelCache := &modelCacheResolver{store: store}
	h := bridge.NewHandler(
		modelCache,
		map[utils.HandlerType]bridge.CredentialScheduler{
			utils.HandlerCodex:  codexScheduler,
			utils.HandlerGemini: geminiScheduler,
		},
		codexClient,
		geminiClient,
	)
	h.SetSettingsProvider(settingsSvc)

	adminHandler := handler.NewAdminHandler(store, codexClient, geminiClient)
	adminHandler.SetAuthCache(authCache)
	adminHandler.SetModelCache(modelCache)
	adminHandler.SetCredentialRefresher(&credentialRefreshDispatcher{
		codex:  codexScheduler,
		gemini: geminiScheduler,
	})
	adminHandler.SetLogStore(logStore)
	adminHandler.SetSettingsService(settingsSvc)
	adminHandler.SetBuildInfoProvider(func() handler.BuildInfo {
		info := CurrentBuildInfo()
		return handler.BuildInfo{
			Version:   info.Version,
			BuildTime: info.BuildTime,
		}
	})

	r := gin.New()
	r.Use(gin.Recovery())
	router.Setup(r, router.Deps{
		Bridge:    h,
		Admin:     adminHandler,
		AuthCache: authCache,
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
		MaxHeaderBytes:    serverMaxHeaderBytes,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-serverErr:
		return err
	}
}

func openStore(ctx context.Context, cfg Config) (db.Store, error) {
	switch normalizeDBType(cfg.DBType) {
	case "sqlite":
		if strings.TrimSpace(cfg.DatabaseURL) == "" {
			return nil, errors.New("database url is required for sqlite")
		}
		return sqlite.Open(ctx, cfg.DatabaseURL)
	case "postgres":
		if strings.TrimSpace(cfg.DatabaseURL) == "" {
			return nil, errors.New("database url is required for postgres")
		}
		return postgres.Open(ctx, cfg.DatabaseURL)
	default:
		return nil, fmt.Errorf("unsupported db type %q", cfg.DBType)
	}
}

type modelCacheResolver struct {
	store db.Store
	mu    sync.Mutex
	cache map[string]modelCacheEntry
	group singleflight.Group
}

const (
	modelCacheTTL         = 10 * time.Minute
	modelNegativeCacheTTL = 30 * time.Second
	modelCacheMaxEntries  = 1024
)

type modelCacheEntry struct {
	info       bridge.ResolvedModel
	err        error
	expiresAt  time.Time
	lastAccess time.Time
}

func (r *modelCacheResolver) ResolveModel(ctx context.Context, alias string) (*bridge.ResolvedModel, error) {
	alias = strings.TrimSpace(alias)
	if r == nil || r.store == nil {
		return nil, errors.New("model store is unavailable")
	}
	if info, err, ok := r.lookupModelCache(alias, time.Now()); ok {
		if err != nil {
			return nil, err
		}
		return cloneResolvedModel(info), nil
	}

	value, err, _ := r.group.Do(alias, func() (any, error) {
		if info, cachedErr, ok := r.lookupModelCache(alias, time.Now()); ok {
			if cachedErr != nil {
				return nil, cachedErr
			}
			return info, nil
		}

		row, err := r.store.ReverseInfoFromModel(ctx, alias)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				r.storeModelCache(alias, modelCacheEntry{
					err:       db.ErrNotFound,
					expiresAt: time.Now().Add(modelNegativeCacheTTL),
				})
			}
			return nil, err
		}
		ht, ok := utils.ParseHandlerType(row.Handler)
		if !ok {
			return nil, fmt.Errorf("unknown handler type: %q", row.Handler)
		}
		info := bridge.ResolvedModel{
			Origin:           row.Origin,
			Handler:          ht,
			AllowedPlanTypes: parseModelPlanTypes(ht, row.PlanTypes),
		}
		r.storeModelCache(alias, modelCacheEntry{
			info:      info,
			expiresAt: time.Now().Add(modelCacheTTL),
		})
		return info, nil
	})
	if err != nil {
		return nil, err
	}
	info := value.(bridge.ResolvedModel)
	return cloneResolvedModel(info), nil
}

func (r *modelCacheResolver) ListModels(ctx context.Context) ([]bridge.ModelListItem, error) {
	rows, err := r.store.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]bridge.ModelListItem, 0, len(rows))
	for _, row := range rows {
		ht, ok := utils.ParseHandlerType(row.Handler)
		if !ok {
			continue
		}
		items = append(items, bridge.ModelListItem{
			Alias:   row.Alias,
			Origin:  row.Origin,
			Handler: ht,
		})
	}
	return items, nil
}

func (r *modelCacheResolver) InvalidateModel(alias string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.cache, strings.TrimSpace(alias))
	r.mu.Unlock()
}

func (r *modelCacheResolver) lookupModelCache(alias string, now time.Time) (bridge.ResolvedModel, error, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		return bridge.ResolvedModel{}, nil, false
	}
	entry, ok := r.cache[alias]
	if !ok {
		return bridge.ResolvedModel{}, nil, false
	}
	if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
		delete(r.cache, alias)
		return bridge.ResolvedModel{}, nil, false
	}
	entry.lastAccess = now
	r.cache[alias] = entry
	return entry.info, entry.err, true
}

func (r *modelCacheResolver) storeModelCache(alias string, entry modelCacheEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cache == nil {
		r.cache = make(map[string]modelCacheEntry)
	}
	now := time.Now()
	entry.lastAccess = now
	r.cache[alias] = entry
	r.pruneModelCacheLocked(now)
}

func (r *modelCacheResolver) pruneModelCacheLocked(now time.Time) {
	for alias, entry := range r.cache {
		if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
			delete(r.cache, alias)
		}
	}
	for len(r.cache) > modelCacheMaxEntries {
		var oldestAlias string
		var oldest time.Time
		for alias, entry := range r.cache {
			if oldestAlias == "" || entry.lastAccess.Before(oldest) {
				oldestAlias = alias
				oldest = entry.lastAccess
			}
		}
		delete(r.cache, oldestAlias)
	}
}

func cloneResolvedModel(info bridge.ResolvedModel) *bridge.ResolvedModel {
	info.AllowedPlanTypes = append([]string(nil), info.AllowedPlanTypes...)
	return &info
}

func parseModelPlanTypes(handler utils.HandlerType, raw string) []string {
	switch handler {
	case utils.HandlerGemini:
		return coreGemini.ParsePlanTypeList(raw)
	case utils.HandlerCodex:
		return coreCodex.ParsePlanTypeList(raw)
	default:
		return nil
	}
}

type credentialRefreshDispatcher struct {
	codex  *coreCodex.Scheduler
	gemini *coreGemini.Scheduler
}

func (d *credentialRefreshDispatcher) RefreshAvailable(ctx context.Context, handler utils.HandlerType) error {
	switch handler {
	case utils.HandlerGemini:
		_, err := d.gemini.RefreshAvailable(ctx)
		return err
	case utils.HandlerCodex:
		_, err := d.codex.RefreshAvailable(ctx)
		return err
	default:
		return fmt.Errorf("unsupported credential handler %q", handler)
	}
}

func (d *credentialRefreshDispatcher) SyncQuotas(ctx context.Context, handler utils.HandlerType, ids []string) {
	switch handler {
	case utils.HandlerGemini:
		d.gemini.SyncCredentials(ctx, ids)
	case utils.HandlerCodex:
		d.codex.SyncCredentials(ctx, ids)
	}
}

func (d *credentialRefreshDispatcher) InvalidateCredentials(handler utils.HandlerType, ids []string) {
	if d == nil || len(ids) == 0 {
		return
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		switch handler {
		case utils.HandlerGemini:
			if d.gemini != nil {
				d.gemini.InvalidateCredential(id)
			}
		case utils.HandlerCodex:
			if d.codex != nil {
				d.codex.InvalidateCredential(id)
			}
		}
	}
}
