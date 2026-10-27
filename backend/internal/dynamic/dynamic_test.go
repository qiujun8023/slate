package dynamic

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"slate/internal/api"
)

func TestNormalizeConfigDefaults(t *testing.T) {
	cases := map[string]string{
		`{"type":"hot_list"}`: `{"type":"hot_list","refreshIntervalSec":600,"source":"weibo"}`,
		`{"type":"weather_alert","province":"广西","audioEnabled":true}`: `{"type":"weather_alert","audioEnabled":true,"audioVoice":"冰糖","refreshIntervalSec":600,"province":"广西壮族自治区"}`,
		`{"type":"font_test"}`:                              `{"type":"font_test","fontId":"fusion_pixel_12"}`,
		`{"type":"daily_calendar","tz":"Asia/Shanghai"}`:    `{"type":"daily_calendar","tz":"Asia/Shanghai","audioVoice":"冰糖"}`,
		`{"type":"dashboard","template":{"kind":"system"}}`: `{"type":"dashboard","refreshIntervalSec":600,"template":{"kind":"system","id":"ai_usage_stats"}}`,
	}
	for in, want := range cases {
		cfg, err := NormalizeConfig(json.RawMessage(in))
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		got, _ := json.Marshal(cfg)
		if string(got) != want {
			t.Errorf("%s:\n got %s\nwant %s", in, got, want)
		}
	}
}

func TestNormalizeConfigRejects(t *testing.T) {
	for _, in := range []string{
		`{"type":"nope"}`,
		`{"type":"weather","tz":"Asia/Shanghai"}`,
		`{"type":"daily_calendar","tz":"Mars/Base"}`,
		`{"type":"hot_list","source":"unknown"}`,
		`{"type":"hot_list","refreshIntervalSec":30}`,
		`{"type":"hot_list","extra":1}`,
		`{"type":"dashboard","template":{"kind":"custom","template":{"blocks":[{"type":"text","x":390,"y":30,"w":20,"h":10,"value":"x"}]}}}`,
	} {
		if _, err := NormalizeConfig(json.RawMessage(in)); err == nil {
			t.Errorf("%s: accepted", in)
		}
	}
}

func TestCustomTemplateFillsBlockDefaults(t *testing.T) {
	cfg, err := NormalizeConfig(json.RawMessage(`{"type":"dashboard","template":{"kind":"custom","template":{"blocks":[{"type":"rect","x":20,"y":30,"w":10,"h":10},{"type":"line","x1":0,"y1":40,"x2":399,"y2":40}]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	blocks := cfg.Template.Template.Blocks
	if blocks[0].Stroke == nil || !*blocks[0].Stroke || blocks[0].Fill != "none" || blocks[1].Style != "solid" {
		t.Fatalf("defaults not applied: %+v", blocks)
	}
}

func TestScheduleCalendarWakesAfterLocalMidnight(t *testing.T) {
	now := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC) // 23:00 in Shanghai
	next, due := schedule(api.DynamicConfig{Type: api.DynamicTypeDailyCalendar, TZ: "Asia/Shanghai"}, now)
	if want := time.Date(2026, 10, 26, 16, 1, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next %v, want %v", next, want)
	}
	if want := time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC); !due.Equal(want) {
		t.Fatalf("due %v, want %v", due, want)
	}
	if next, _ := schedule(api.DynamicConfig{Type: api.DynamicTypeFontTest}, now); next != nil {
		t.Fatalf("font test should never refresh")
	}
}

func TestErrorBackoffCapsAtOneHour(t *testing.T) {
	now := time.Now()
	if got := errorBackoff(1, now).Sub(now); got != time.Minute {
		t.Fatalf("first failure waits %v", got)
	}
	if got := errorBackoff(20, now).Sub(now); got != time.Hour {
		t.Fatalf("backoff not capped: %v", got)
	}
}

func TestStatusBarText(t *testing.T) {
	at := time.Date(2026, 10, 26, 2, 0, 0, 0, time.UTC)
	typ := string(api.DynamicTypeMonthCalendar)
	if got := StatusBarText(nil, &typ, json.RawMessage(`{"type":"month_calendar","tz":"Asia/Shanghai"}`), nil, &at); got != "2026年10月" {
		t.Fatalf("got %q", got)
	}
	typ = string(api.DynamicTypeHotList)
	if got := StatusBarText(nil, &typ, json.RawMessage(`{"type":"hot_list","source":"zhihu"}`), nil, nil); got != "知乎热榜热榜" {
		t.Fatalf("got %q", got)
	}
}

func TestSpeechText(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"items": []map[string]any{{"title": "浙江省杭州市气象台发布暴雨橙色预警信号"}}})
	got := speechText(api.DynamicConfig{Type: api.DynamicTypeWeatherAlert}, data, time.Now())
	if got != "全国气象预警，播报最新一条。浙江杭州发布暴雨橙色预警" {
		t.Fatalf("got %q", got)
	}
	if got := speechText(api.DynamicConfig{Type: api.DynamicTypeHotList}, nil, time.Now()); got != "" {
		t.Fatalf("hot lists have no speech, got %q", got)
	}
}

func TestDecodeFirstObject(t *testing.T) {
	var v struct{ A int }
	if !decodeFirstObject("noise {not json} then {\"A\": 3} tail", &v) || v.A != 3 {
		t.Fatalf("got %+v", v)
	}
}

func TestCalendarDayFindsSolarTerms(t *testing.T) {
	d := calendarDay(2026, 11, 7)
	if d.SolarTerm != "立冬" || *d.NextSolarTermDays != 0 {
		t.Fatalf("unexpected term %+v", d)
	}
	d = calendarDay(2026, 10, 26)
	if d.NextSolarTerm != "立冬" || *d.NextSolarTermDays != 12 || !strings.HasPrefix(d.LunarDate, "农历") {
		t.Fatalf("unexpected day %+v", d)
	}
}
