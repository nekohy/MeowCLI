package codex

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	codexAPI "github.com/nekohy/MeowCLI/api/codex"
	"github.com/nekohy/MeowCLI/core/scheduling"
)

// ListRateLimitResetCredits 实时拉取指定凭证的重置额度列表（转发给前端的精简结构）
func (s *Scheduler) ListRateLimitResetCredits(ctx context.Context, credentialID string) (*codexAPI.RateLimitResetCredits, error) {
	if s.fetcher == nil || s.manager == nil {
		return nil, fmt.Errorf("codex scheduler not ready")
	}
	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}
	return s.fetcher.FetchRateLimitResetCredits(ctx, credentialID, token)
}

// ConsumeRateLimitResetCredit 消耗一次重置额度，成功后同步刷新该账号的 quota 缓存
func (s *Scheduler) ConsumeRateLimitResetCredit(ctx context.Context, credentialID string) (*codexAPI.ConsumeRateLimitResetCreditResponse, error) {
	if s.fetcher == nil || s.manager == nil {
		return nil, fmt.Errorf("codex scheduler not ready")
	}
	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}
	resp, err := s.fetcher.ConsumeRateLimitResetCredit(ctx, credentialID, token, uuid.NewString())
	if err != nil {
		return nil, err
	}
	// 用独立 context 同步刷新，避免请求 ctx 取消导致刷新失败
	s.refreshQuotaNow(context.Background(), credentialID)
	return resp, nil
}

// refreshQuotaNow 同步拉取一次 quota 并写入缓存（含 reset-credits 计数）
func (s *Scheduler) refreshQuotaNow(ctx context.Context, credentialID string) {
	if s.fetcher == nil || s.manager == nil {
		return
	}
	token, err := s.manager.AccessToken(ctx, credentialID, scheduling.UseCached)
	if err != nil {
		return
	}
	q, err := s.fetcher.FetchQuota(ctx, credentialID, token)
	if err != nil {
		return
	}
	s.StoreQuota(ctx, credentialID, q)
}
