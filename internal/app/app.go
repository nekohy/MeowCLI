package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/nekohy/MeowCLI/api/codex"
	codexutils "github.com/nekohy/MeowCLI/api/codex/utils"
	runtimelogs "github.com/nekohy/MeowCLI/internal/logs"
	"github.com/nekohy/MeowCLI/internal/settings"
	db "github.com/nekohy/MeowCLI/internal/store"
	"github.com/nekohy/MeowCLI/utils"
	"net/http"
	"strings"
	"time"

	coreCodex "github.com/nekohy/MeowCLI/core/codex"
	"github.com/nekohy/MeowCLI/db/postgres"
	"github.com/nekohy/MeowCLI/db/sqlite"
	"github.com/nekohy/MeowCLI/internal/auth"
	"github.com/nekohy/MeowCLI/internal/bridge"
	"github.com/nekohy/MeowCLI/internal/handler"
	"github.com/nekohy/MeowCLI/internal/router"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func Run(ctx context.Context, cfg Config) error {
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

	h := bridge.NewHandler(
		&modelStoreAdapter{store: store},
		map[utils.HandlerType]bridge.CredentialScheduler{
			utils.HandlerCodex: &codexSchedulerAdapter{s: codexScheduler},
		},
		codexClient,
	)
	h.SetSettingsProvider(settingsSvc)

	adminHandler := handler.NewAdminHandler(store, codexClient)
	adminHandler.SetAuthCache(authCache)
	adminHandler.SetCredentialRefresher(&credRefreshAdapter{s: codexScheduler})
	adminHandler.SetLogStore(logStore)
	adminHandler.SetSettingsService(settingsSvc)

	r := gin.New()
	r.Use(gin.Recovery())
	router.Setup(r, router.Deps{
		Bridge:    h,
		Admin:     adminHandler,
		AuthCache: authCache,
	})

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: r,
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

// codexSchedulerAdapter 适配 coreCodex.Scheduler → bridge.CredentialScheduler（codex 凭据池）
type codexSchedulerAdapter struct {
	s *coreCodex.Scheduler
}

func (a *codexSchedulerAdapter) Pick(ctx context.Context, planType string) (string, error) {
	return a.s.Pick(ctx, planType)
}

func (a *codexSchedulerAdapter) AuthHeaders(ctx context.Context, credID string) (http.Header, error) {
	token, err := a.s.GetAccessToken(ctx, credID)
	if err != nil {
		return nil, err
	}
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+token)
	h.Set("Chatgpt-Account-Id", credID)
	return h, nil
}

func (a *codexSchedulerAdapter) RecordSuccess(ctx context.Context, id string, code int32) {
	a.s.RecordSuccess(ctx, id, code)
}

func (a *codexSchedulerAdapter) RecordFailure(ctx context.Context, id string, code int32, text string, retry time.Duration) {
	a.s.RecordFailure(ctx, id, code, text, retry)
}

func (a *codexSchedulerAdapter) HandleUnauthorized(ctx context.Context, id string, code int32, text string) bool {
	return a.s.HandleUnauthorized(ctx, id, code, text)
}

// modelStoreAdapter 适配 db.Store → bridge.ModelStore
type modelStoreAdapter struct {
	store db.Store
}

func (a *modelStoreAdapter) ResolveModel(ctx context.Context, alias string) (string, utils.HandlerType, error) {
	row, err := a.store.ReverseInfoFromModel(ctx, alias)
	if err != nil {
		return "", "", err
	}
	ht, ok := utils.ParseHandlerType(row.Handler)
	if !ok {
		return "", "", fmt.Errorf("unknown handler type: %q", row.Handler)
	}
	return row.Origin, ht, nil
}

// credRefreshAdapter 适配 coreCodex.Scheduler → handler.CredentialRefresher
type credRefreshAdapter struct {
	s *coreCodex.Scheduler
}

func (a *credRefreshAdapter) RefreshAvailable(ctx context.Context) error {
	_, err := a.s.RefreshAvailable(ctx)
	return err
}

func (a *credRefreshAdapter) SyncQuotas(ctx context.Context, ids []string) {
	a.s.SyncCredentials(ctx, ids)
}
