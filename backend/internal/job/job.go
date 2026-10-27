// Package job runs sidecar jobs that push data to dashboard contents.
package job

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Job struct {
	Description string
	Run         func(ctx context.Context) error
}

var jobs = map[string]Job{
	"claude-code-quota-monitor": {"Push Claude Code quota usage to a dashboard.", claudeQuota},
}

// Run executes a job every SLATE_JOB_INTERVAL_SECONDS (600) until ctx ends, or once
// when SLATE_JOB_RUN_ONCE=1.
func Run(ctx context.Context, name string) error {
	j, ok := jobs[name]
	if !ok {
		names := make([]string, 0, len(jobs))
		for n := range jobs {
			names = append(names, n)
		}
		return fmt.Errorf("unknown job %q; available: %s", name, strings.Join(names, ", "))
	}
	interval := time.Duration(positiveEnv("SLATE_JOB_INTERVAL_SECONDS", 600)) * time.Second
	once := env("SLATE_JOB_RUN_ONCE") == "1"
	slog.Info("job started", "job", name, "description", j.Description, "interval", interval.String(), "once", once)
	for {
		start := time.Now()
		if err := j.Run(ctx); err != nil {
			slog.Error("job failed", "job", name, "error", err.Error())
		} else {
			slog.Info("job completed", "job", name, "durationMs", time.Since(start).Milliseconds())
		}
		if once {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

func env(name string) string { return strings.TrimSpace(os.Getenv(name)) }

func requireEnv(name string) (string, error) {
	if v := env(name); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("missing env %s", name)
}

func positiveEnv(name string, fallback int) int {
	if n, err := strconv.Atoi(env(name)); err == nil && n > 0 {
		return n
	}
	return fallback
}

func timeZone() (*time.Location, error) {
	name := env("SLATE_JOB_TIME_ZONE")
	if name == "" {
		name = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("invalid SLATE_JOB_TIME_ZONE %s: %w", name, err)
	}
	return loc, nil
}

var client = &http.Client{Timeout: 30 * time.Second}

// doJSON sends body as JSON (when not nil) and decodes the response. Responses shaped
// {code, data, message} are unwrapped when code is 0 or 200.
func doJSON(ctx context.Context, method, url string, body any, headers map[string]string, out any) error {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, url, resp.StatusCode, truncate(string(raw), 500))
	}
	var wrapped struct {
		Code    *int            `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && wrapped.Code != nil {
		if *wrapped.Code != 0 && *wrapped.Code != 200 {
			return fmt.Errorf("%s %s: error %d: %s", method, url, *wrapped.Code, wrapped.Message)
		}
		raw = wrapped.Data
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// push sends dashboard data to POST /api/contents/{id}/data.
func push(ctx context.Context, apiBase, contentID string, data map[string]any) error {
	var resp struct {
		Success bool `json:"success"`
	}
	url := strings.TrimRight(apiBase, "/") + "/api/contents/" + contentID + "/data"
	if err := doJSON(ctx, http.MethodPost, url, map[string]any{"version": 1, "data": data}, nil, &resp); err != nil {
		return err
	}
	slog.Info("dashboard data pushed", "contentId", contentID)
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
