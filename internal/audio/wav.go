package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

func ReadWAV(r io.Reader) (int, []float32, error) {
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, fmt.Errorf("wav header: %w", err)
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return 0, nil, errors.New("not a RIFF/WAVE file")
	}
	var format, channels, bits uint16
	var rate uint32
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return 0, nil, fmt.Errorf("wav: no data chunk: %w", err)
		}
		id, size := string(ch[0:4]), binary.LittleEndian.Uint32(ch[4:8])
		switch id {
		case "fmt ":
			buf := make([]byte, size)
			if _, err := io.ReadFull(r, buf); err != nil {
				return 0, nil, err
			}
			format = binary.LittleEndian.Uint16(buf[0:2])
			channels = binary.LittleEndian.Uint16(buf[2:4])
			rate = binary.LittleEndian.Uint32(buf[4:8])
			bits = binary.LittleEndian.Uint16(buf[14:16])
		case "data":
			if channels == 0 {
				return 0, nil, errors.New("wav: data before fmt")
			}
			buf := make([]byte, size)
			n, err := io.ReadFull(r, buf)
			if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
				return 0, nil, err
			}
			s, err := decodePCM(buf[:n], format, channels, bits)
			return int(rate), s, err
		default:
			if _, err := io.CopyN(io.Discard, r, int64(size+size%2)); err != nil {
				return 0, nil, err
			}
		}
	}
}

func decodePCM(b []byte, format, channels, bits uint16) ([]float32, error) {
	var width int
	var read func([]byte) float32
	switch {
	case format == 1 && bits == 16:
		width = 2
		read = func(p []byte) float32 { return float32(int16(binary.LittleEndian.Uint16(p))) / 32768 }
	case format == 3 && bits == 32:
		width = 4
		read = func(p []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(p)) }
	default:
		return nil, fmt.Errorf("wav: unsupported format %d/%d bits", format, bits)
	}
	frame := width * int(channels)
	out := make([]float32, len(b)/frame)
	for i := range out {
		var sum float32
		for c := 0; c < int(channels); c++ {
			off := i*frame + c*width
			sum += read(b[off : off+width])
		}
		out[i] = sum / float32(channels)
	}
	return out, nil
}

type WAVSource struct {
	Rate     int
	Samples  []float32
	Realtime bool
}

func (w *WAVSource) Run(ctx context.Context, out chan<- Block) error {
	const n = 512
	tick := time.Duration(float64(n) / float64(w.Rate) * float64(time.Second))
	next := time.Now()
	for i := 0; i < len(w.Samples); i += n {
		if w.Realtime {
			next = next.Add(tick)
			select {
			case <-time.After(time.Until(next)):
			case <-ctx.Done():
				return nil
			}
		}
		j := min(i+n, len(w.Samples))
		if !send(ctx, out, Block{Rate: w.Rate, Samples: w.Samples[i:j]}) {
			return nil
		}
	}
	return nil
}
