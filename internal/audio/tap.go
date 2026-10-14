package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// decodeStream reads the lampa-tap protocol and emits blocks of up to
// `block` samples. A trailing partial sample is dropped.
func decodeStream(r io.Reader, block int, emit func(Block)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var hdr [4]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil {
		return fmt.Errorf("tap header: %w", err)
	}
	rate := int(binary.LittleEndian.Uint32(hdr[:]))
	if rate < 8000 || rate > 384000 {
		return fmt.Errorf("tap: implausible sample rate %d", rate)
	}
	buf := make([]byte, block*4)
	for {
		n, err := io.ReadFull(br, buf)
		if k := n / 4; k > 0 {
			s := make([]float32, k)
			for i := range s {
				s[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
			}
			emit(Block{Rate: rate, Samples: s})
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// TapSource runs the lampa-tap helper; PID 0 means the whole system.
type TapSource struct {
	Path    string
	PID     int
	OnError func(error)
}

func (t *TapSource) report(err error) {
	if err != nil && t.OnError != nil {
		t.OnError(err)
	}
}

func (t *TapSource) Run(ctx context.Context, out chan<- Block) error {
	for ctx.Err() == nil {
		t.report(t.once(ctx, out))
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
	return nil
}

func (t *TapSource) once(ctx context.Context, out chan<- Block) error {
	args := []string{"capture"}
	if t.PID != 0 {
		args = append(args, "--pid", strconv.Itoa(t.PID))
	}
	cmd := exec.CommandContext(ctx, t.Path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", t.Path, err)
	}
	derr := decodeStream(stdout, 512, func(b Block) { send(ctx, out, b) })
	werr := cmd.Wait()
	if ctx.Err() != nil {
		return nil
	}
	if msg := lastLine(stderr.String()); msg != "" {
		return errors.New(msg) // e.g. the permission hint printed by lampa-tap
	}
	if derr != nil {
		return derr
	}
	if werr != nil {
		return fmt.Errorf("lampa-tap exited: %w", werr)
	}
	return errors.New("lampa-tap exited")
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

type App struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	BundleID string `json:"bundleID"`
}

func ListApps(ctx context.Context, path string) ([]App, error) {
	out, err := exec.CommandContext(ctx, path, "list").Output()
	if err != nil {
		return nil, fmt.Errorf("lampa-tap list: %w", err)
	}
	var apps []App
	if err := json.Unmarshal(out, &apps); err != nil {
		return nil, fmt.Errorf("lampa-tap list: %w", err)
	}
	return apps, nil
}
