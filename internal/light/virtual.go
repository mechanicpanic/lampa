package light

import (
	"context"
	"sync"
)

// Virtual is a lamp that only exists on screen (--dry, tests).
type Virtual struct {
	info  Info
	mu    sync.Mutex
	last  Frame
	count int
}

func NewVirtual(info Info) *Virtual {
	if info.Kind == "" {
		info.Kind = KindVirtual
	}
	return &Virtual{info: info}
}

func (v *Virtual) Info() Info { return v.info }

func (v *Virtual) Apply(_ context.Context, f Frame) error {
	v.mu.Lock()
	v.last, v.count = f, v.count+1
	v.mu.Unlock()
	return nil
}

func (v *Virtual) Last() (Frame, int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.last, v.count
}

func (v *Virtual) Snapshot(context.Context) (State, error) { return State{}, nil }
func (v *Virtual) Restore(context.Context, State) error    { return nil }
func (v *Virtual) Close() error                            { return nil }
