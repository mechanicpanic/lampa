package modes

import (
	"math"
	"sort"
	"time"
)

// stackPos is a lamp's place in its fixture: pos 0 = bottom, of n lamps.
// n == 0 means the lamp isn't part of a stacked group.
type stackPos struct{ pos, n int }

// stacks places every lamp with a Group and Level inside its group,
// ordered by Level (1 = bottom).
func stacks(c Context) []stackPos {
	out := make([]stackPos, len(c.Lights))
	members := map[string][]int{}
	for i, l := range c.Lights {
		if l.Group != "" && l.Level > 0 {
			members[l.Group] = append(members[l.Group], i)
		}
	}
	for _, idx := range members {
		sort.SliceStable(idx, func(a, b int) bool { return c.Lights[idx[a]].Level < c.Lights[idx[b]].Level })
		for pos, i := range idx {
			out[i] = stackPos{pos: pos, n: len(idx)}
		}
	}
	return out
}

// height: 0 at the bottom of the fixture, 1 at the top (0 when not stacked).
func (s stackPos) height() float64 {
	if s.n < 2 {
		return 0
	}
	return float64(s.pos) / float64(s.n-1)
}

// verticalBand picks bass at the bottom … high at the top.
func (s stackPos) verticalBand() int {
	bands := []int{1, 3, 4} // bass, mid, high
	if s.n > 3 {
		bands = []int{1, 2, 3, 4} // + lowmid
	}
	return bands[int(math.Round(s.height()*float64(len(bands)-1)))]
}

// fountainStep: how much later each floor of a stack flashes after the one below.
const fountainStep = 70 * time.Millisecond
