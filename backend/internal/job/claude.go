package job

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type quotaLimits struct {
	pct5h, pct7d     float64
	reset5h, reset7d int64
}

// claudeQuota probes the Anthropic API once and reads the rate-limit headers. The token
// comes from ANTHROPIC_API_KEY or ~/.claude/.credentials.json.
func claudeQuota(ctx context.Context) error {
	slate, contentID, loc, err := quotaTarget()
	if err != nil {
		return err
	}
	token, err := claudeToken()
	if err != nil {
		return err
	}
	limits, status, err := probeQuota(ctx, token)
	data := quotaData(limits, status, loc)
	if err != nil {
		data["status_label"] = "未知"
	}
	return push(ctx, slate, contentID, data)
}

func quotaTarget() (string, string, *time.Location, error) {
	slate, err := requireEnv("SLATE_API_BASE")
	if err != nil {
		return "", "", nil, err
	}
	contentID, err := requireEnv("CLAUDE_QUOTA_CONTENT_ID")
	if err != nil {
		return "", "", nil, err
	}
	loc, err := timeZone()
	return slate, contentID, loc, err
}

func claudeToken() (string, error) {
	if v := env("ANTHROPIC_API_KEY"); v != "" {
		return v, nil
	}
	home, _ := os.UserHomeDir()
	raw, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		return "", fmt.Errorf("set ANTHROPIC_API_KEY or sign in to Claude Code: %w", err)
	}
	var creds struct {
		AccessToken string `json:"accessToken"`
	}
	if json.Unmarshal(raw, &creds) != nil || creds.AccessToken == "" {
		return "", fmt.Errorf("no accessToken in ~/.claude/.credentials.json")
	}
	return creds.AccessToken, nil
}

func probeQuota(ctx context.Context, token string) (quotaLimits, string, error) {
	base := strings.TrimRight(env("ANTHROPIC_API_BASE"), "/")
	if base == "" {
		base = "https://api.anthropic.com"
	}
	body := `{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", strings.NewReader(body))
	if err != nil {
		return quotaLimits{}, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("User-Agent", "claude-code/2.1.5")
	resp, err := client.Do(req)
	if err != nil {
		return quotaLimits{}, "", err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	h := resp.Header
	f := func(k string) float64 { v, _ := strconv.ParseFloat(h.Get(k), 64); return v }
	i := func(k string) int64 { v, _ := strconv.ParseInt(h.Get(k), 10, 64); return v }
	if h.Get("anthropic-ratelimit-unified-5h-utilization") != "" && h.Get("anthropic-ratelimit-unified-5h-reset") != "" {
		return quotaLimits{
			pct5h: f("anthropic-ratelimit-unified-5h-utilization") * 100, reset5h: i("anthropic-ratelimit-unified-5h-reset"),
			pct7d: f("anthropic-ratelimit-unified-7d-utilization") * 100, reset7d: i("anthropic-ratelimit-unified-7d-reset"),
		}, h.Get("anthropic-ratelimit-unified-status"), nil
	}
	// API keys report request limits instead of the unified subscription windows.
	if limit, remaining := f("anthropic-ratelimit-requests-limit"), h.Get("anthropic-ratelimit-requests-remaining"); limit > 0 && remaining != "" {
		r, _ := strconv.ParseFloat(remaining, 64)
		var reset int64
		if t, err := time.Parse(time.RFC3339, h.Get("anthropic-ratelimit-requests-reset")); err == nil {
			reset = t.Unix()
		}
		return quotaLimits{pct5h: (limit - r) / limit * 100, reset5h: reset}, "", nil
	}
	return quotaLimits{}, "", fmt.Errorf("no rate-limit headers in HTTP %d response", resp.StatusCode)
}

var weekdaysCN = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

func quotaData(l quotaLimits, apiStatus string, loc *time.Location) map[string]any {
	p5 := math.Round(clampPct(l.pct5h))
	p7 := math.Round(clampPct(l.pct7d))
	status := "正常"
	switch {
	case apiStatus == "rejected" || p5 >= 100 || p7 >= 100:
		status = "已限流"
	case apiStatus == "allowed_warning" || p5 >= 80 || p7 >= 80:
		status = "警告"
	}
	return map[string]any{
		"service_label": "Claude Code", "plan_label": planLabel(), "status_label": status,
		"primary_window_label": "5h窗口", "primary_used_percent": p5, "primary_reset_at_label": reset5h(l.reset5h, loc),
		"secondary_window_label": "周限额", "secondary_used_percent": p7, "secondary_reset_at_label": reset7d(l.reset7d, loc),
		"updated_label": time.Now().In(loc).Format("01-02 15:04"),
	}
}

func planLabel() string {
	if v, ok := os.LookupEnv("CLAUDE_PLAN_LABEL"); ok {
		return v
	}
	return "Max 20x"
}

func clampPct(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Max(0, math.Min(100, v))
}

func reset5h(sec int64, loc *time.Location) string {
	if sec <= 0 {
		return "--"
	}
	t, now := time.Unix(sec, 0).In(loc), time.Now().In(loc)
	if t.Format("20060102") == now.Format("20060102") {
		return t.Format("15:04")
	}
	return "明 " + t.Format("15:04")
}

func reset7d(sec int64, loc *time.Location) string {
	if sec <= 0 {
		return "--"
	}
	t := time.Unix(sec, 0).In(loc)
	return weekdaysCN[t.Weekday()] + " " + t.Format("15:04")
}

// StatusLine implements the Claude Code statusLine command: it prints the quota line from
// stdin and pushes at most once per CLAUDE_QUOTA_PUSH_INTERVAL_MS in a detached child.
func StatusLine(ctx context.Context, args []string) error {
	slate, contentID, loc, err := quotaTarget()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cache", "slate-claude-quota")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payloadPath := filepath.Join(dir, "pending-payload.json")
	if len(args) > 0 && args[0] == "--push" {
		raw, err := os.ReadFile(payloadPath)
		if err != nil {
			return err
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
		return push(ctx, slate, contentID, data)
	}

	var in struct {
		RateLimits *struct {
			FiveHour struct {
				UsedPercentage float64 `json:"used_percentage"`
				ResetsAt       int64   `json:"resets_at"`
			} `json:"five_hour"`
			SevenDay struct {
				UsedPercentage float64 `json:"used_percentage"`
				ResetsAt       int64   `json:"resets_at"`
			} `json:"seven_day"`
		} `json:"rate_limits"`
	}
	raw := readStdin(500 * time.Millisecond)
	if json.Unmarshal(bytes.TrimSpace(raw), &in) != nil || in.RateLimits == nil {
		return claudeQuota(ctx)
	}
	l := quotaLimits{
		pct5h: clampPct(in.RateLimits.FiveHour.UsedPercentage), reset5h: in.RateLimits.FiveHour.ResetsAt,
		pct7d: clampPct(in.RateLimits.SevenDay.UsedPercentage), reset7d: in.RateLimits.SevenDay.ResetsAt,
	}
	fmt.Printf("5h %s %.0f%% %s | 7d %s %.0f%% %s", bar(l.pct5h), l.pct5h, reset5h(l.reset5h, loc), bar(l.pct7d), l.pct7d, reset7d(l.reset7d, loc))

	stampPath := filepath.Join(dir, "last-push-ts")
	interval := time.Duration(positiveEnv("CLAUDE_QUOTA_PUSH_INTERVAL_MS", 60000)) * time.Millisecond
	if stamp, err := os.ReadFile(stampPath); err == nil {
		if last, err := strconv.ParseInt(strings.TrimSpace(string(stamp)), 10, 64); err == nil && time.Since(time.UnixMilli(last)) < interval {
			return nil
		}
	}
	payload, _ := json.Marshal(quotaData(l, "", loc))
	_ = os.WriteFile(stampPath, []byte(strconv.FormatInt(time.Now().UnixMilli(), 10)), 0o644)
	_ = os.WriteFile(payloadPath, payload, 0o644)
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	cmd := exec.Command(self, "claude-statusline", "--push")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

func readStdin(timeout time.Duration) []byte {
	if info, err := os.Stdin.Stat(); err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(os.Stdin)
		done <- b
	}()
	select {
	case b := <-done:
		return b
	case <-time.After(timeout):
		return nil
	}
}

func bar(pct float64) string {
	n := int(math.Max(0, math.Min(10, math.Round(pct/10))))
	return strings.Repeat("▓", n) + strings.Repeat("░", 10-n)
}
