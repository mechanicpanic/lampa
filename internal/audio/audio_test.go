package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"testing"
	"testing/iotest"
	"time"
)

func tapBytes(rate uint32, samples []float32) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, rate)
	binary.Write(&b, binary.LittleEndian, samples)
	return b.Bytes()
}

func collect(t *testing.T, data []byte) (int, []float32) {
	t.Helper()
	var got []float32
	rate := 0
	err := decodeStream(iotest.OneByteReader(bytes.NewReader(data)), 100, func(b Block) {
		rate = b.Rate
		got = append(got, b.Samples...)
	})
	if err != nil {
		t.Fatal(err)
	}
	return rate, got
}

func TestDecodeStreamOneByte(t *testing.T) {
	want := make([]float32, 1003)
	for i := range want {
		want[i] = float32(math.Sin(float64(i)))
	}
	rate, got := collect(t, tapBytes(44100, want))
	if rate != 44100 {
		t.Fatalf("rate %d", rate)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d samples, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d: %v != %v", i, got[i], want[i])
		}
	}
}

func TestDecodeStreamDropsTrailingPartialSample(t *testing.T) {
	data := append(tapBytes(48000, []float32{1, 2}), 0x01, 0x02)
	_, got := collect(t, data)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func wavBytes(format, channels uint16, rate uint32, frames [][]float64) []byte {
	bits := uint16(16)
	if format == 3 {
		bits = 32
	}
	var data bytes.Buffer
	for _, fr := range frames {
		for _, v := range fr {
			if format == 3 {
				binary.Write(&data, binary.LittleEndian, float32(v))
			} else {
				binary.Write(&data, binary.LittleEndian, int16(v*32767))
			}
		}
	}
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + data.Len()))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	w(uint32(16))
	w(format)
	w(channels)
	w(rate)
	w(rate * uint32(channels) * uint32(bits/8))
	w(channels * bits / 8)
	w(bits)
	b.WriteString("LIST") // an extra chunk the reader must skip
	w(uint32(4))
	b.WriteString("INFO")
	b.WriteString("data")
	w(uint32(data.Len()))
	b.Write(data.Bytes())
	return b.Bytes()
}

func TestReadWAVStereo16(t *testing.T) {
	rate, s, err := ReadWAV(bytes.NewReader(wavBytes(1, 2, 44100, [][]float64{{0.5, 0.5}, {1, -1}, {-0.5, -0.5}})))
	if err != nil {
		t.Fatal(err)
	}
	if rate != 44100 || len(s) != 3 {
		t.Fatalf("rate %d len %d", rate, len(s))
	}
	if math.Abs(float64(s[0])-0.5) > 1e-3 || math.Abs(float64(s[1])) > 1e-3 || math.Abs(float64(s[2])+0.5) > 1e-3 {
		t.Fatalf("samples %v", s)
	}
}

func TestReadWAVFloatMono(t *testing.T) {
	rate, s, err := ReadWAV(bytes.NewReader(wavBytes(3, 1, 48000, [][]float64{{0.25}, {-0.75}})))
	if err != nil || rate != 48000 || len(s) != 2 || s[0] != 0.25 || s[1] != -0.75 {
		t.Fatalf("rate %d s %v err %v", rate, s, err)
	}
}

func TestReadWAVRejectsGarbage(t *testing.T) {
	if _, _, err := ReadWAV(bytes.NewReader([]byte("not a wav file at all"))); err == nil {
		t.Fatal("want error")
	}
}

func TestWAVSourceEmitsAll(t *testing.T) {
	src := &WAVSource{Rate: 48000, Samples: make([]float32, 1500)}
	out := make(chan Block, 10)
	if err := src.Run(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	close(out)
	n := 0
	for b := range out {
		if b.Rate != 48000 {
			t.Fatalf("rate %d", b.Rate)
		}
		n += len(b.Samples)
	}
	if n != 1500 {
		t.Fatalf("emitted %d", n)
	}
}

func TestTapSourceStopsOnCancel(t *testing.T) {
	// /bin/cat with no stdin exits immediately; the source must keep
	// restarting until ctx is cancelled and then return promptly.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	src := &TapSource{Path: "/usr/bin/true"}
	done := make(chan error)
	go func() { done <- src.Run(ctx, make(chan Block, 1)) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
