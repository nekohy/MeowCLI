package gemini

import (
	"testing"
	"time"
)

// 带通用 quotaResetDelay 的 429 体 → 三档全部置 0,reset≈now+delay
func TestParseQuotaFromError_GenericDelay(t *testing.T) {
	body := []byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","metadata":{"quotaResetDelay":"3600s"}}]}}`)
	q, found := ParseQuotaFromError(body)
	if !found || q == nil {
		t.Fatalf("expected found=true, got found=%v q=%v", found, q)
	}
	if q.QuotaPro != 0 || q.QuotaFlash != 0 || q.QuotaFlashlite != 0 {
		t.Fatalf("expected all tiers quota=0, got pro=%v flash=%v lite=%v", q.QuotaPro, q.QuotaFlash, q.QuotaFlashlite)
	}
	if d := time.Until(q.ResetPro); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatalf("expected ResetPro ~1h ahead, got %v", d)
	}
}

// 带 per-tier proResetDelay → 只 pro 置 0,flash/flashlite 不动
func TestParseQuotaFromError_PerTierDelay(t *testing.T) {
	body := []byte(`{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","metadata":{"proResetDelay":"1800s"}}]}}`)
	q, found := ParseQuotaFromError(body)
	if !found || q == nil {
		t.Fatalf("expected found=true, got found=%v", found)
	}
	if q.QuotaPro != 0 {
		t.Fatalf("expected pro quota=0, got %v", q.QuotaPro)
	}
	if q.ResetPro.IsZero() {
		t.Fatalf("expected ResetPro set")
	}
	// flash/flashlite 未在错误体出现,reset 应保持零值
	if !q.ResetFlash.IsZero() || !q.ResetFlashlite.IsZero() {
		t.Fatalf("expected flash/flashlite reset untouched (zero), got flash=%v lite=%v", q.ResetFlash, q.ResetFlashlite)
	}
}

// 简洁 429 体(无 error.details) → ParseQuotaFromError 不命中,但 IsResourceExhausted 能识别
func TestTerseResourceExhausted(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`)
	if _, found := ParseQuotaFromError(body); found {
		t.Fatalf("expected ParseQuotaFromError found=false for terse body")
	}
	if !IsResourceExhausted(body) {
		t.Fatalf("expected IsResourceExhausted=true for terse RESOURCE_EXHAUSTED body")
	}
}

// 非耗尽的普通 429(瞬时限流) → 两者都不应判为耗尽
func TestIsResourceExhausted_NonExhausted(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"Too many requests","status":"UNAVAILABLE"}}`)
	if IsResourceExhausted(body) {
		t.Fatalf("expected IsResourceExhausted=false for non-RESOURCE_EXHAUSTED body")
	}
	if _, found := ParseQuotaFromError(body); found {
		t.Fatalf("expected ParseQuotaFromError found=false for body without quotaResetDelay")
	}
}
