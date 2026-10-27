package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"slate/internal/api"
	"slate/internal/blob"
	"slate/internal/config"
	"slate/internal/media"
	"slate/internal/render"
	"slate/internal/store"
	"slate/internal/store/db"
)

const (
	schedulerBatch   = 5
	schedulerLease   = 3 * time.Minute
	schedulerIdle    = 5 * time.Minute
	schedulerMinTick = 10 * time.Second
	ttsInterval      = 5 * time.Second
	ttsLease         = 2 * time.Minute
	ttsMaxAttempts   = 3
	blobGCInterval   = 6 * time.Hour
	blobGCMinAge     = time.Hour
)

type Engine struct {
	cfg      config.Config
	store    *store.Store
	blobs    *blob.Store
	media    *media.Transcoder
	renderer *render.Renderer
	ai, tts  llm
	cache    *ttlCache

	locksMu sync.Mutex
	locks   map[string]chan struct{}

	// wake cuts the scheduler's idle sleep short after a schedule changes.
	wake chan struct{}
	// workers tracks the Start loops so shutdown can wait for them.
	workers sync.WaitGroup
}

func New(cfg config.Config, st *store.Store, blobs *blob.Store, m *media.Transcoder) (*Engine, error) {
	r, err := render.New()
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	return &Engine{
		cfg: cfg, store: st, blobs: blobs, media: m, renderer: r, cache: newCache(), locks: map[string]chan struct{}{}, wake: make(chan struct{}, 1),
		ai:  llm{cfg.AIAPIKey, cfg.AIBaseURL, cfg.AIModel, client},
		tts: llm{cfg.TTSAPIKey, cfg.TTSBaseURL, cfg.TTSModel, client},
	}, nil
}

// Mutate serializes changes and renders of one content within this process. It gives up
// waiting for the lock when ctx ends.
func (e *Engine) Mutate(ctx context.Context, contentID string, f func() error) error {
	e.locksMu.Lock()
	l, ok := e.locks[contentID]
	if !ok {
		l = make(chan struct{}, 1)
		e.locks[contentID] = l
	}
	e.locksMu.Unlock()
	select {
	case l <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-l }()
	return f()
}

// Frame is one render plus the state to store with it.
type Frame struct {
	Image        []byte
	Data         json.RawMessage
	RenderedAt   time.Time
	NextRunAt    *time.Time
	RefreshDueAt *time.Time
	// FetchError is set when the source failed and older data was rendered instead.
	FetchError *string
}

func (f Frame) SaveParams(contentID string) db.SaveDynamicRenderParams {
	at := f.RenderedAt
	return db.SaveDynamicRenderParams{ContentID: contentID, Data: f.Data, RenderedAt: &at, NextRunAt: f.NextRunAt, RefreshDueAt: f.RefreshDueAt, LastError: f.FetchError}
}

// FetchError reports a source failure with no reusable data.
type FetchError struct{ Err error }

func (e *FetchError) Error() string   { return "数据获取失败: " + e.Err.Error() }
func (e *FetchError) HTTPStatus() int { return 502 }
func (e *FetchError) Unwrap() error   { return e.Err }

// Prepare fetches data (unless override is given) and renders it. lastData is rendered
// when the fetch fails and the type allows reuse.
func (e *Engine) Prepare(ctx context.Context, cfg api.DynamicConfig, override, lastData json.RawMessage, now time.Time) (Frame, error) {
	return e.prepare(ctx, cfg, override, lastData, nil, now)
}

func (e *Engine) prepare(ctx context.Context, cfg api.DynamicConfig, override, lastData json.RawMessage, lastRenderedAt *time.Time, now time.Time) (Frame, error) {
	now = now.UTC().Truncate(time.Millisecond)
	f := Frame{RenderedAt: now}
	if len(override) > 0 {
		f.Data = override
	} else {
		v, err := e.fetchData(ctx, cfg, lastData, now)
		switch {
		case err == nil:
			if raw, ok := v.(json.RawMessage); ok {
				f.Data = raw
			} else if v != nil {
				f.Data, _ = json.Marshal(v)
			}
		case canReuse(cfg, lastData, lastRenderedAt, now):
			slog.Warn("dynamic fetch failed; reusing last data", "type", cfg.Type, "error", err.Error())
			msg := truncate(err.Error(), 512)
			f.Data, f.FetchError = lastData, &msg
		default:
			return f, &FetchError{err}
		}
	}
	img, err := e.renderer.Render(render.Input{Type: cfg.Type, Config: cfg, Data: f.Data, RenderedAt: now})
	if err != nil {
		return f, err
	}
	f.Image = img
	f.NextRunAt, f.RefreshDueAt = schedule(cfg, now)
	return f, nil
}

type Result struct {
	ImageHash  string
	RenderedAt time.Time
}

// Render refreshes a stored dynamic content. override replaces the data, which is how
// dashboard pushes arrive.
func (e *Engine) Render(ctx context.Context, contentID string, override json.RawMessage) (Result, error) {
	var res Result
	err := e.Mutate(ctx, contentID, func() error {
		c, err := e.store.Q.GetContent(ctx, contentID)
		if err != nil {
			return err
		}
		if c.DynamicType == nil {
			return invalid("该内容不是动态类型")
		}
		d, err := e.store.Q.GetDynamic(ctx, contentID)
		if err != nil {
			return err
		}
		cfg, err := NormalizeConfig(c.DynamicConfig)
		if err != nil {
			e.markError(ctx, d, "配置非法: "+err.Error())
			return err
		}
		f, err := e.prepare(ctx, cfg, override, c.DynamicData, c.RenderedAt, time.Now())
		if err != nil {
			e.markError(ctx, d, err.Error())
			return err
		}
		hash, err := e.blobs.Put(f.Image)
		if err != nil {
			return err
		}
		err = e.store.Tx(ctx, func(q *db.Queries) error {
			if hash != c.ImageHash {
				if err := q.SetContentImage(ctx, db.SetContentImageParams{ID: contentID, ImageHash: hash}); err != nil {
					return err
				}
			}
			return q.SaveDynamicRender(ctx, f.SaveParams(contentID))
		})
		res = Result{hash, f.RenderedAt}
		return err
	})
	if err == nil {
		e.Wake()
		e.SyncAudio(ctx, contentID)
	}
	return res, err
}

func (e *Engine) markError(ctx context.Context, d db.ContentDynamic, message string) {
	at := errorBackoff(d.Attempts+1, time.Now()).UTC()
	msg := truncate(message, 512)
	if err := e.store.Q.MarkDynamicError(ctx, db.MarkDynamicErrorParams{ContentID: d.ContentID, LastError: &msg, NextRunAt: &at}); err != nil {
		slog.Error("dynamic error mark failed", "contentId", d.ContentID, "error", err.Error())
	}
}

// SyncAudio queues TTS for audio-enabled dynamic content whose speech changed, and drops
// audio when it is disabled. It holds the content lock from reading the config to writing
// the audio, so a late call reconciles to the newest config instead of an older one.
// Failures are logged; the frame is already saved.
func (e *Engine) SyncAudio(ctx context.Context, contentID string) {
	if err := e.Mutate(ctx, contentID, func() error { return e.syncAudio(ctx, contentID) }); err != nil {
		slog.Warn("dynamic audio sync failed", "contentId", contentID, "error", err.Error())
	}
}

func (e *Engine) syncAudio(ctx context.Context, contentID string) error {
	c, err := e.store.Q.GetContent(ctx, contentID)
	if err != nil || c.DynamicType == nil {
		return err
	}
	cfg, err := NormalizeConfig(c.DynamicConfig)
	text := ""
	if err == nil && cfg.AudioEnabled {
		at := time.Now()
		if c.RenderedAt != nil {
			at = *c.RenderedAt
		}
		text = speechText(cfg, c.DynamicData, at)
	}
	if text == "" {
		if c.AudioSource != nil {
			_, err = e.store.Q.DeleteAudio(ctx, contentID)
		}
		return err
	}
	if c.AudioText != nil && *c.AudioText == text && c.AudioVoice != nil && *c.AudioVoice == cfg.AudioVoice &&
		c.AudioStatus != nil && *c.AudioStatus != db.AudioStatusFailed {
		return nil
	}
	return e.store.Q.SetAudio(ctx, db.SetAudioParams{ContentID: contentID, Source: db.AudioSourceTts, Status: db.AudioStatusPending, Voice: &cfg.AudioVoice, Text: &text})
}

// Start runs the refresh scheduler, the TTS worker and blob GC until ctx ends; Wait
// blocks until they have returned.
func (e *Engine) Start(ctx context.Context) {
	e.workers.Go(func() { e.loop(ctx, "dynamic scheduler", e.wake, e.runScheduler) })
	e.workers.Go(func() {
		e.loop(ctx, "tts worker", nil, func(ctx context.Context) time.Duration { e.runTTS(ctx); return ttsInterval })
	})
	e.workers.Go(func() {
		e.loop(ctx, "blob gc", nil, func(ctx context.Context) time.Duration { e.collectBlobs(ctx); return blobGCInterval })
	})
}

// Wait blocks until the Start loops return or ctx ends, whichever comes first.
func (e *Engine) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { e.workers.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wake makes the scheduler re-read the next due time; call it after saving a schedule.
func (e *Engine) Wake() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) loop(ctx context.Context, name string, wake <-chan struct{}, tick func(context.Context) time.Duration) {
	for {
		delay := func() (d time.Duration) {
			defer func() {
				if v := recover(); v != nil {
					slog.Error(name+" panic", "panic", fmt.Sprint(v))
					d = time.Minute
				}
			}()
			return tick(ctx)
		}()
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-time.After(delay):
		}
	}
}

// runScheduler claims due rows with SKIP LOCKED leases, so several processes can share
// the work; a crashed worker's lease simply expires.
func (e *Engine) runScheduler(ctx context.Context) time.Duration {
	now := time.Now().UTC().Truncate(time.Millisecond)
	lease := now.Add(schedulerLease)
	jobs, err := e.store.Q.ClaimDueDynamic(ctx, db.ClaimDueDynamicParams{LeaseUntil: &lease, Now: &now, MaxJobs: schedulerBatch})
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("dynamic scheduler claim failed", "error", err.Error())
		}
		return schedulerMinTick
	}
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Go(func() {
			rctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			if _, err := e.Render(rctx, j.ContentID, nil); err != nil {
				slog.Warn("dynamic refresh failed", "contentId", j.ContentID, "type", j.Type, "error", err.Error())
				msg := truncate(err.Error(), 512)
				due := errorBackoff(1, time.Now()).UTC()
				_ = e.store.Q.ReleaseDynamicLease(ctx, db.ReleaseDynamicLeaseParams{ContentID: j.ContentID, LeaseUntil: j.LeaseUntil, DueAt: &due, LastError: &msg})
			} else {
				slog.Info("dynamic content refreshed", "contentId", j.ContentID, "type", j.Type)
			}
		})
	}
	wg.Wait()
	if len(jobs) >= schedulerBatch {
		return 0
	}
	next, err := e.store.Q.NextDynamicDueAt(ctx)
	if err != nil || next == nil {
		return schedulerIdle
	}
	return min(max(time.Until(*next), schedulerMinTick), schedulerIdle)
}

// runTTS claims one job at a time so each lease only has to cover a single generation.
func (e *Engine) runTTS(ctx context.Context) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	if n, err := e.store.Q.FailAbandonedTTS(ctx, db.FailAbandonedTTSParams{MaxAttempts: ttsMaxAttempts, Now: &now}); err != nil {
		if ctx.Err() == nil {
			slog.Warn("tts abandoned-job cleanup failed", "error", err.Error())
		}
		return
	} else if n > 0 {
		slog.Warn("tts jobs abandoned during their last attempt marked failed", "count", n)
	}
	for ctx.Err() == nil {
		now := time.Now().UTC().Truncate(time.Millisecond)
		lease := now.Add(ttsLease)
		j, err := e.store.Q.ClaimTTSJob(ctx, db.ClaimTTSJobParams{LeaseUntil: &lease, MaxAttempts: ttsMaxAttempts, Now: &now})
		if store.Missing(err) {
			return
		}
		if err != nil {
			if ctx.Err() == nil {
				slog.Warn("tts claim failed", "error", err.Error())
			}
			return
		}
		if j.Voice == nil || j.Text == nil {
			continue
		}
		err = e.generateTTS(ctx, j)
		if err == nil {
			slog.Info("tts audio generated", "contentId", j.ContentID, "voice", *j.Voice)
			continue
		}
		slog.Warn("tts generation failed", "contentId", j.ContentID, "attempt", j.Attempts, "error", err.Error())
		status := db.AudioStatusPending
		var retry *time.Time
		if j.Attempts >= ttsMaxAttempts {
			status = db.AudioStatusFailed
		} else {
			at := time.Now().Add(min(time.Minute, 5*time.Second<<(j.Attempts-1))).UTC()
			retry = &at
		}
		msg := truncate(err.Error(), 512)
		if err := e.store.Q.FailTTS(ctx, db.FailTTSParams{ContentID: j.ContentID, LeaseUntil: j.LeaseUntil, Status: status, LastError: &msg, RetryAt: retry}); err != nil && ctx.Err() == nil {
			slog.Warn("tts failure not recorded; the lease will expire and retry", "contentId", j.ContentID, "error", err.Error())
		}
	}
}

func (e *Engine) generateTTS(ctx context.Context, j db.ClaimTTSJobRow) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	raw, err := e.tts.synthesize(ctx, *j.Text, *j.Voice)
	if err != nil {
		return err
	}
	pcm, err := e.media.Resample(ctx, raw, ttsSourceRate)
	if err != nil {
		return err
	}
	hash, err := e.blobs.Put(pcm)
	if err != nil {
		return err
	}
	size := int32(len(pcm))
	n, err := e.store.Q.CompleteTTS(ctx, db.CompleteTTSParams{ContentID: j.ContentID, LeaseUntil: j.LeaseUntil, Voice: j.Voice, Text: j.Text, Hash: &hash, Size: &size})
	if err == nil && n == 0 {
		slog.Info("tts result discarded; audio changed while generating", "contentId", j.ContentID)
	}
	return err
}

// collectBlobs deletes files no content references. The minimum age protects blobs
// written by requests whose rows are not committed yet.
func (e *Engine) collectBlobs(ctx context.Context) {
	hashes, err := e.store.Q.ReferencedBlobHashes(ctx)
	if err != nil {
		slog.Warn("blob gc query failed", "error", err.Error())
		return
	}
	keep := make(map[string]bool, len(hashes))
	for _, h := range hashes {
		keep[h] = true
	}
	removed, err := e.blobs.GC(func(h string) bool { return keep[h] }, blobGCMinAge)
	if err != nil {
		slog.Warn("blob gc failed", "error", err.Error())
	}
	if removed > 0 {
		slog.Info("blob gc removed unreferenced files", "count", removed)
	}
}
