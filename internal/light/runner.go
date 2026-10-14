package light

import (
	"context"
	"sync"
	"time"
)

const defaultFPS = 20

type Stats struct {
	Status Status
	Note   string
	FPS    float64
	Errors int
	Last   Frame
}

// Runner owns one lamp: it keeps only the newest frame, rate-limits to
// Info().MaxFPS and skips frames identical to the last one sent.
type Runner struct {
	l    Light
	wake chan struct{}

	mu      sync.Mutex
	pending Frame
	has     bool
	last    Frame
	sentAny bool
	failed  bool
	errors  int
	sends   []time.Time
}

func NewRunner(l Light) *Runner {
	return &Runner{l: l, wake: make(chan struct{}, 1)}
}

func (r *Runner) Light() Light { return r.l }

func (r *Runner) Set(f Frame) {
	r.mu.Lock()
	r.pending, r.has = f, true
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runner) Run(ctx context.Context) {
	fps := r.l.Info().MaxFPS
	if fps <= 0 {
		fps = defaultFPS
	}
	interval := time.Duration(float64(time.Second) / fps)
	var lastSend time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		}
		if wait := interval - time.Since(lastSend); wait > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
		r.mu.Lock()
		f, has := r.pending, r.has
		r.has = false
		skip := !has || (r.sentAny && !r.failed && f == r.last)
		r.mu.Unlock()
		if skip {
			continue
		}
		actx, cancel := context.WithTimeout(ctx, time.Second)
		err := r.l.Apply(actx, f)
		cancel()
		lastSend = time.Now()
		r.mu.Lock()
		if err != nil {
			r.failed = true
			r.errors++
		} else {
			r.failed, r.last, r.sentAny = false, f, true
			r.sends = append(r.sends, lastSend)
		}
		r.mu.Unlock()
	}
}

func (r *Runner) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	cut := time.Now().Add(-time.Second)
	i := 0
	for i < len(r.sends) && r.sends[i].Before(cut) {
		i++
	}
	r.sends = r.sends[i:]
	s := Stats{FPS: float64(len(r.sends)), Errors: r.errors, Last: r.last, Status: StatusOK}
	if r.failed {
		s.Status = StatusLost
	}
	if st, ok := r.l.(Statuser); ok {
		s.Status, s.Note = st.Status()
	}
	return s
}
