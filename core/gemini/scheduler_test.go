package gemini

import (
	"testing"
	"time"
)

// 修复核心:某档位 quota 置 0 后,CalcScore 立即判该档不可用(-1),调度自动跳过。
func TestCalcScore_ZeroQuotaUnavailable(t *testing.T) {
	var zero time.Time
	if score := CalcScore(0, 1, 1, zero, zero, zero, ModelTierPro, 3600); score != -1 {
		t.Fatalf("expected pro score=-1 when quotaPro=0, got %v", score)
	}
}

// 不误伤:只把 pro 置 0 时,flash 档位(quota 仍满)依旧可用(score>=0)。
func TestCalcScore_OtherTierUnaffected(t *testing.T) {
	var zero time.Time
	if score := CalcScore(0, 1, 1, zero, zero, zero, ModelTierFlash, 3600); score < 0 {
		t.Fatalf("expected flash score>=0 when flash quota=1 (pro exhausted), got %v", score)
	}
	if score := CalcScore(0, 1, 1, zero, zero, zero, ModelTierFlashLite, 3600); score < 0 {
		t.Fatalf("expected flashlite score>=0 when its quota=1, got %v", score)
	}
}

// 满额时该档位可用。
func TestCalcScore_FullQuotaAvailable(t *testing.T) {
	var zero time.Time
	if score := CalcScore(1, 1, 1, zero, zero, zero, ModelTierPro, 3600); score < 0 {
		t.Fatalf("expected pro score>=0 when quotaPro=1, got %v", score)
	}
}

// resolveExhaustedReset:带 quotaResetDelay 的 429 体 → 命中且按 tier 返回对应 reset。
func TestResolveExhaustedReset_GenericDelay(t *testing.T) {
	errText := `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","metadata":{"quotaResetDelay":"3600s"}}]}}`
	reset, exhausted := resolveExhaustedReset(errText, ModelTierPro)
	if !exhausted {
		t.Fatalf("expected exhausted=true")
	}
	if d := time.Until(reset); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatalf("expected reset ~1h ahead, got %v", d)
	}
}

// resolveExhaustedReset:简洁 RESOURCE_EXHAUSTED(无 details) → 命中但 reset 为零(交由兜底)。
func TestResolveExhaustedReset_Terse(t *testing.T) {
	errText := `{"error":{"code":429,"message":"Resource has been exhausted.","status":"RESOURCE_EXHAUSTED"}}`
	reset, exhausted := resolveExhaustedReset(errText, ModelTierPro)
	if !exhausted {
		t.Fatalf("expected exhausted=true for terse RESOURCE_EXHAUSTED")
	}
	if !reset.IsZero() {
		t.Fatalf("expected zero reset (fallback applied later), got %v", reset)
	}
}

// resolveExhaustedReset:普通瞬时 429(非耗尽) → 不命中,维持原短熔断路径。
func TestResolveExhaustedReset_NonExhausted(t *testing.T) {
	errText := `{"error":{"code":429,"message":"slow down","status":"UNAVAILABLE"}}`
	if _, exhausted := resolveExhaustedReset(errText, ModelTierPro); exhausted {
		t.Fatalf("expected exhausted=false for non RESOURCE_EXHAUSTED 429")
	}
	if _, exhausted := resolveExhaustedReset("", ModelTierPro); exhausted {
		t.Fatalf("expected exhausted=false for empty error text")
	}
}
