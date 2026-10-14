// Package audio provides mono float32 sample streams.
package audio

import "context"

type Block struct {
	Rate    int
	Samples []float32
}

// Source produces blocks until ctx is done or the input ends. It never closes out.
type Source interface {
	Run(ctx context.Context, out chan<- Block) error
}

func send(ctx context.Context, out chan<- Block, b Block) bool {
	select {
	case out <- b:
		return true
	case <-ctx.Done():
		return false
	}
}
