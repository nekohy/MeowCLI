package antigravity

import "time"

type CachedQuotaMetric struct {
	Available      bool
	Quota          float64
	Reset          time.Time
	ThrottledUntil time.Time
	Score          float64
	Weight         float64
}

type CachedQuotaSnapshot struct {
	Claude    CachedQuotaMetric
	Pro       CachedQuotaMetric
	Flash     CachedQuotaMetric
	FlashLite CachedQuotaMetric
	Tab       CachedQuotaMetric
	Image     CachedQuotaMetric
}

func (s *Scheduler) CachedQuota(id string) (CachedQuotaSnapshot, bool) {
	if id == "" {
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
			Claude: CachedQuotaMetric{
				Available:      row.ScoreClaude >= 0,
				Quota:          row.QuotaClaude,
				Reset:          row.ResetClaude,
				ThrottledUntil: row.ThrottledClaude,
				Score:          row.ScoreClaude,
				Weight:         row.WeightClaude,
			},
			Pro: CachedQuotaMetric{
				Available:      row.ScorePro >= 0,
				Quota:          row.QuotaPro,
				Reset:          row.ResetPro,
				ThrottledUntil: row.ThrottledPro,
				Score:          row.ScorePro,
				Weight:         row.WeightPro,
			},
			Flash: CachedQuotaMetric{
				Available:      row.ScoreFlash >= 0,
				Quota:          row.QuotaFlash,
				Reset:          row.ResetFlash,
				ThrottledUntil: row.ThrottledFlash,
				Score:          row.ScoreFlash,
				Weight:         row.WeightFlash,
			},
			FlashLite: CachedQuotaMetric{
				Available:      row.ScoreFlashlite >= 0,
				Quota:          row.QuotaFlashlite,
				Reset:          row.ResetFlashlite,
				ThrottledUntil: row.ThrottledLite,
				Score:          row.ScoreFlashlite,
				Weight:         row.WeightFlashlite,
			},
			Tab: CachedQuotaMetric{
				Available:      row.ScoreTab >= 0,
				Quota:          row.QuotaTab,
				Reset:          row.ResetTab,
				ThrottledUntil: row.ThrottledTab,
				Score:          row.ScoreTab,
				Weight:         row.WeightTab,
			},
			Image: CachedQuotaMetric{
				Available:      row.ScoreImage >= 0,
				Quota:          row.QuotaImage,
				Reset:          row.ResetImage,
				ThrottledUntil: row.ThrottledImage,
				Score:          row.ScoreImage,
				Weight:         row.WeightImage,
			},
		}, true
	}
	return CachedQuotaSnapshot{}, false
}
