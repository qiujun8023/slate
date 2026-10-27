package job

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
)

type sub2apiSession struct {
	access, refresh string
	expires         time.Time
}

// cachedSession lives for the process; a restart logs in again.
var cachedSession *sub2apiSession

type sub2apiStats struct {
	TotalRequests            float64 `json:"total_requests"`
	TotalInputTokens         float64 `json:"total_input_tokens"`
	TotalCacheCreationTokens float64 `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     float64 `json:"total_cache_read_tokens"`
	TotalTokens              float64 `json:"total_tokens"`
	TotalActualCost          float64 `json:"total_actual_cost"`
	TodayRequests            float64 `json:"today_requests"`
	TodayInputTokens         float64 `json:"today_input_tokens"`
	TodayCacheCreationTokens float64 `json:"today_cache_creation_tokens"`
	TodayCacheReadTokens     float64 `json:"today_cache_read_tokens"`
	TodayTokens              float64 `json:"today_tokens"`
	TodayActualCost          float64 `json:"today_actual_cost"`
	AverageDurationMs        float64 `json:"average_duration_ms"`
	ByPlatform               []struct {
		Platform        string  `json:"platform"`
		TodayRequests   float64 `json:"today_requests"`
		TodayTokens     float64 `json:"today_tokens"`
		TodayActualCost float64 `json:"today_actual_cost"`
	} `json:"by_platform"`
}

// sub2apiUsageStats logs in with email and password (2FA and Turnstile are not
// supported), refreshes the access token while it can, and pushes the user stats.
func sub2apiUsageStats(ctx context.Context) error {
	base, err := requireEnv("SUB2API_BASE")
	if err != nil {
		return err
	}
	base = strings.TrimRight(base, "/")
	slate, err := requireEnv("SLATE_API_BASE")
	if err != nil {
		return err
	}
	contentID, err := requireEnv("SUB2API_CONTENT_ID")
	if err != nil {
		return err
	}
	loc, err := timeZone()
	if err != nil {
		return err
	}
	var stats sub2apiStats
	get := func() error {
		token, err := sub2apiToken(ctx, base)
		if err != nil {
			return err
		}
		return doJSON(ctx, http.MethodGet, base+"/api/v1/usage/dashboard/stats", nil, map[string]string{"Authorization": "Bearer " + token}, &stats)
	}
	if err := get(); err != nil {
		// A rejected token is replaced by a refresh or a fresh login on the retry.
		if cachedSession != nil {
			cachedSession.expires = time.Time{}
		}
		if err2 := get(); err2 != nil {
			cachedSession = nil
			return err2
		}
	}
	now := time.Now().In(loc)
	platforms := stats.ByPlatform
	sort.SliceStable(platforms, func(i, j int) bool {
		a, b := platforms[i], platforms[j]
		if a.TodayActualCost != b.TodayActualCost {
			return a.TodayActualCost > b.TodayActualCost
		}
		if a.TodayTokens != b.TodayTokens {
			return a.TodayTokens > b.TodayTokens
		}
		return a.TodayRequests > b.TodayRequests
	})
	breakdown := []map[string]any{}
	for i, p := range platforms {
		if i == 2 {
			break
		}
		breakdown = append(breakdown, map[string]any{
			"platform_name": p.Platform, "today_cost_usd": p.TodayActualCost,
			"today_request_count": p.TodayRequests, "today_token_count": p.TodayTokens,
		})
	}
	return push(ctx, slate, contentID, map[string]any{
		"today_cost_usd":                    stats.TodayActualCost,
		"today_request_count":               stats.TodayRequests,
		"today_token_count":                 stats.TodayTokens,
		"today_cache_hit_rate_percent":      cacheHitRate(stats.TodayInputTokens, stats.TodayCacheCreationTokens, stats.TodayCacheReadTokens),
		"total_cost_usd":                    stats.TotalActualCost,
		"total_request_count":               stats.TotalRequests,
		"total_token_count":                 stats.TotalTokens,
		"total_cache_hit_rate_percent":      cacheHitRate(stats.TotalInputTokens, stats.TotalCacheCreationTokens, stats.TotalCacheReadTokens),
		"average_latency_ms":                stats.AverageDurationMs,
		"today_cost_per_million_tokens_usd": perMillion(stats.TodayActualCost, stats.TodayTokens),
		"total_cost_per_million_tokens_usd": perMillion(stats.TotalActualCost, stats.TotalTokens),
		"last_updated_time_label":           now.Format("15:04"),
		"last_updated_label":                now.Format("01-02 15:04"),
		"platform_breakdown":                breakdown,
	})
}

func cacheHitRate(input, creation, read float64) float64 {
	if d := input + creation + read; d > 0 {
		return read / d * 100
	}
	return 0
}

func perMillion(cost, tokens float64) float64 {
	if tokens > 0 {
		return cost / tokens * 1e6
	}
	return 0
}

type sub2apiTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Requires2FA  bool   `json:"requires_2fa"`
}

func sub2apiToken(ctx context.Context, base string) (string, error) {
	if s := cachedSession; s != nil && time.Now().Add(2*time.Minute).Before(s.expires) {
		return s.access, nil
	}
	if s := cachedSession; s != nil && s.refresh != "" {
		var r sub2apiTokenResponse
		err := doJSON(ctx, http.MethodPost, base+"/api/v1/auth/refresh", map[string]string{"refresh_token": s.refresh}, nil, &r)
		if err == nil && r.AccessToken != "" {
			cachedSession = newSession(r)
			slog.Info("sub2api token refreshed")
			return r.AccessToken, nil
		}
		slog.Warn("sub2api refresh failed; logging in again", "error", errText(err))
	}
	email, err := requireEnv("SUB2API_EMAIL")
	if err != nil {
		return "", err
	}
	password, err := requireEnv("SUB2API_PASSWORD")
	if err != nil {
		return "", err
	}
	var r sub2apiTokenResponse
	if err := doJSON(ctx, http.MethodPost, base+"/api/v1/auth/login", map[string]string{"email": email, "password": password}, nil, &r); err != nil {
		return "", err
	}
	if r.Requires2FA {
		return "", errors.New("sub2api login requires 2FA, which password automation cannot complete")
	}
	if r.AccessToken == "" {
		return "", errors.New("sub2api login returned no access_token")
	}
	cachedSession = newSession(r)
	slog.Info("sub2api logged in")
	return r.AccessToken, nil
}

func newSession(r sub2apiTokenResponse) *sub2apiSession {
	ttl := r.ExpiresIn
	if ttl <= 0 {
		ttl = 3600
	}
	return &sub2apiSession{r.AccessToken, r.RefreshToken, time.Now().Add(time.Duration(ttl) * time.Second)}
}

func errText(err error) string {
	if err == nil {
		return "empty access_token"
	}
	return err.Error()
}
