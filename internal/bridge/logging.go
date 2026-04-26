package bridge

import (
	"io"
	"math"
	"time"
)

func logSeconds(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	return math.Round(duration.Seconds()*100) / 100
}

type responseTiming struct {
	firstByte time.Duration
	duration  time.Duration
}

type timedReadCloser struct {
	io.ReadCloser
	start     time.Time
	firstByte time.Duration
	duration  time.Duration
}

func newTimedReadCloser(body io.ReadCloser, start time.Time) *timedReadCloser {
	return &timedReadCloser{
		ReadCloser: body,
		start:      start,
	}
}

func (r *timedReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	now := time.Now()
	if n > 0 && r.firstByte <= 0 {
		r.firstByte = now.Sub(r.start)
	}
	if err == io.EOF && r.duration <= 0 {
		r.duration = now.Sub(r.start)
	}
	return n, err
}

func (r *timedReadCloser) Close() error {
	err := r.ReadCloser.Close()
	if r.duration <= 0 {
		r.duration = time.Since(r.start)
	}
	return err
}

func (r *timedReadCloser) timing() responseTiming {
	return responseTiming{
		firstByte: r.firstByte,
		duration:  r.duration,
	}
}
