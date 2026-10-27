package httpapi

import (
	"crypto/rand"
	"math"
	"sync"
	"time"
)

const idAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// newID returns 24 random base36 characters starting with a letter, the same shape as
// the cuid2 IDs already stored.
func newID() string {
	return randomString(idAlphabet, 24, true)
}

// Pair codes avoid I, L and O so they are easy to copy from the screen.
const pairAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func randomString(alphabet string, n int, letterFirst bool) string {
	limit := 256 / len(alphabet) * len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n*2)
	for len(out) < n {
		_, _ = rand.Read(buf)
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			c := alphabet[int(b)%len(alphabet)]
			if letterFirst && len(out) == 0 && (c < 'a' || c > 'z') {
				continue
			}
			out = append(out, c)
			if len(out) == n {
				break
			}
		}
	}
	return string(out)
}

const rateWindow = time.Minute

type bucket struct {
	start time.Time
	count int
}

// rateLimits is a per-process fixed one-minute window keyed by caller-chosen strings.
type rateLimits struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

func newRateLimits() *rateLimits { return &rateLimits{buckets: map[string]*bucket{}} }

func (l *rateLimits) check(key string, limit int, message string) error {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) > rateWindow {
		for k, b := range l.buckets {
			if now.Sub(b.start) >= rateWindow {
				delete(l.buckets, k)
			}
		}
		l.swept = now
	}
	b := l.buckets[key]
	if b == nil || now.Sub(b.start) >= rateWindow {
		b = &bucket{start: now}
		l.buckets[key] = b
	}
	b.count++
	if b.count <= limit {
		return nil
	}
	retry := int(math.Ceil((rateWindow - now.Sub(b.start)).Seconds()))
	return &httpError{status: 429, message: message, retryAfter: max(retry, 1)}
}
