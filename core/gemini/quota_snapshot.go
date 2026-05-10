package gemini

import "time"

type CachedQuotaMetric struct {
	Available bool
	Quota     float64
	Reset     time.Time
	Score     float64
	Weight    float64
}

type CachedQuotaSnapshot struct {
	Pro       CachedQuotaMetric
	Flash     CachedQuotaMetric
	FlashLite CachedQuotaMetric
}

func (s *Scheduler) CachedQuota(id string) (CachedQuotaSnapshot, bool) {
	if s == nil || id == "" {
		return CachedQuotaSnapshot{}, false
	}
	rows := s.available.Load()
	if rows == nil {
		return CachedQuotaSnapshot{}, false
	}
	for _, row := range *rows {
		if row.ID != id {
			continue
		}
		return CachedQuotaSnapshot{
			Pro: CachedQuotaMetric{
				Available: row.ScorePro >= 0,
				Quota:     row.QuotaPro,
				Reset:     row.ResetPro,
				Score:     row.ScorePro,
				Weight:    row.WeightPro,
			},
			Flash: CachedQuotaMetric{
				Available: row.ScoreFlash >= 0,
				Quota:     row.QuotaFlash,
				Reset:     row.ResetFlash,
				Score:     row.ScoreFlash,
				Weight:    row.WeightFlash,
			},
			FlashLite: CachedQuotaMetric{
				Available: row.ScoreFlashLite >= 0,
				Quota:     row.QuotaFlashLite,
				Reset:     row.ResetFlashLite,
				Score:     row.ScoreFlashLite,
				Weight:    row.WeightFlashlite,
			},
		}, true
	}
	return CachedQuotaSnapshot{}, false
}
