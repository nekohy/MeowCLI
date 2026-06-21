package codex

import "time"

type CachedQuotaMetric struct {
	Available      bool
	Quota5h        float64
	Quota7d        float64
	Quota1mo       float64
	Reset5h        time.Time
	Reset7d        time.Time
	Reset1mo       time.Time
	ThrottledUntil time.Time
	Score          float64
	Weight         float64
}

type CachedQuotaSnapshot struct {
	Default CachedQuotaMetric
	Spark   CachedQuotaMetric
}

func (s *Scheduler) CachedQuota(id string) (CachedQuotaSnapshot, bool) {
	if id == "" {
		return CachedQuotaSnapshot{}, false
	}
	snap := s.available.Load()
	if snap == nil {
		return CachedQuotaSnapshot{}, false
	}
	for _, row := range snap.rows {
		if row.ID != id {
			continue
		}
		return CachedQuotaSnapshot{
			Default: CachedQuotaMetric{
				Available:      row.Score >= 0,
				Quota5h:        row.Quota5h,
				Quota7d:        row.Quota7d,
				Quota1mo:       row.Quota1mo,
				Reset5h:        row.Reset5h,
				Reset7d:        row.Reset7d,
				Reset1mo:       row.Reset1mo,
				ThrottledUntil: row.ThrottledUntil,
				Score:          row.Score,
				Weight:         row.Weight,
			},
			Spark: CachedQuotaMetric{
				Available:      row.ScoreSpark >= 0,
				Quota5h:        row.QuotaSpark5h,
				Quota7d:        row.QuotaSpark7d,
				Quota1mo:       row.QuotaSpark1mo,
				Reset5h:        row.ResetSpark5h,
				Reset7d:        row.ResetSpark7d,
				Reset1mo:       row.ResetSpark1mo,
				ThrottledUntil: row.ThrottledUntilSpark,
				Score:          row.ScoreSpark,
				Weight:         row.WeightSpark,
			},
		}, true
	}
	return CachedQuotaSnapshot{}, false
}
