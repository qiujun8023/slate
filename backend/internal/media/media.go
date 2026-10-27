// Package media converts audio to the device format, 16 kHz mono s16le PCM, with ffmpeg.
package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"slate/internal/api"
)

const (
	maxDurationSec = 60
	maxOutputBytes = api.AudioSampleRate * 2 * maxDurationSec
	concurrency    = 2
	queueTimeout   = 30 * time.Second
	runTimeout     = 30 * time.Second
)

// Error is shown to the API caller.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string   { return e.Message }
func (e *Error) HTTPStatus() int { return e.Status }

// Transcoder runs at most two ffmpeg processes and lets eight more callers wait; others
// are rejected so bursts cannot pile up memory.
type Transcoder struct {
	slots   chan struct{}
	waiting chan struct{}
}

func New() *Transcoder {
	return &Transcoder{slots: make(chan struct{}, concurrency), waiting: make(chan struct{}, concurrency*4)}
}

// Transcode converts any audio ffmpeg can read, truncated to 60 seconds.
func (t *Transcoder) Transcode(ctx context.Context, input []byte) ([]byte, error) {
	return t.run(ctx, input, []string{"-i", "pipe:0"})
}

// Resample converts mono s16le PCM at rate Hz to the device rate.
func (t *Transcoder) Resample(ctx context.Context, pcm []byte, rate int) ([]byte, error) {
	if rate == api.AudioSampleRate {
		if len(pcm) > maxOutputBytes {
			return nil, &Error{400, "音频时长超出限制"}
		}
		return pcm, nil
	}
	return t.run(ctx, pcm, []string{"-f", "s16le", "-ac", "1", "-ar", strconv.Itoa(rate), "-i", "pipe:0"})
}

func (t *Transcoder) run(ctx context.Context, input []byte, inputArgs []string) ([]byte, error) {
	select {
	case t.waiting <- struct{}{}:
	default:
		return nil, &Error{429, "音频转码繁忙，请稍后重试"}
	}
	timer := time.NewTimer(queueTimeout)
	defer timer.Stop()
	select {
	case t.slots <- struct{}{}:
		<-t.waiting
	case <-timer.C:
		<-t.waiting
		return nil, &Error{429, "音频转码排队超时，请稍后重试"}
	case <-ctx.Done():
		<-t.waiting
		return nil, ctx.Err()
	}
	defer func() { <-t.slots }()

	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	args := append([]string{"-v", "error"}, inputArgs...)
	args = append(args, "-f", "s16le", "-acodec", "pcm_s16le", "-ac", "1", "-ar", strconv.Itoa(api.AudioSampleRate),
		"-t", strconv.Itoa(maxDurationSec), "pipe:1")
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, &Error{503, "音频处理服务不可用"}
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{400, fmt.Sprintf("音频转码失败: %s", firstLine(stderr.String()))}
	}
	if len(out) == 0 {
		return nil, &Error{400, "音频转码失败：输出为空"}
	}
	if len(out) > maxOutputBytes {
		out = out[:maxOutputBytes]
	}
	return out, nil
}

func firstLine(s string) string {
	if i := bytes.IndexByte([]byte(s), '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
