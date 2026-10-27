package render

import (
	"bytes"
	"encoding/json"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"slate/internal/api"
)

var update = flag.Bool("update", false, "rewrite testdata/frames/*.png")

var renderedAt = time.Date(2026, 10, 26, 2, 30, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// Golden frames are PNGs so changes can be reviewed as pictures; run
// `go test ./internal/render -update` and inspect the diff after layout changes.
func TestFramesMatchGolden(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	day := CalendarDay{LunarDate: "农历九月十七", GanzhiYear: "丙午年", GanzhiMonth: "戊戌月", GanzhiDay: "癸酉日",
		NextSolarTerm: "立冬", NextSolarTermDays: ptr(12), Yi: []string{"嫁娶", "裁衣", "冠笄"}, Ji: []string{"开市", "栽种"}}
	issued := time.Date(2026, 10, 26, 1, 20, 0, 0, time.UTC)
	cases := map[string]struct {
		cfg  api.DynamicConfig
		data any
	}{
		"daily_calendar": {api.DynamicConfig{Type: api.DynamicTypeDailyCalendar, TZ: "Asia/Shanghai"},
			DailyCalendarData{Year: 2026, Month: 10, Day: 26, WeekdayCN: "星期一", CalendarDay: day}},
		"month_calendar": {api.DynamicConfig{Type: api.DynamicTypeMonthCalendar, TZ: "Asia/Shanghai"},
			MonthCalendarData{Months: map[string]map[string]CalendarDay{"2026-10": {"2026-10-01": {LunarDate: "农历八月廿一"}, "2026-10-08": {SolarTerm: "寒露"}, "2026-10-18": {LunarDate: "农历九月初九", Festival: "重阳节"}}}}},
		"history_today": {api.DynamicConfig{Type: api.DynamicTypeHistoryToday, TZ: "Asia/Shanghai"},
			HistoryTodayData{DateLabel: "10月26日", Items: []HistoryTodayItem{{"1863", "英格兰足球总会在伦敦成立，现代足球规则由此统一"}, {"前221", "秦统一六国"}, {"1955", "奥地利宣布永久中立，此后每年这一天为国庆日"}}}},
		"weather": {api.DynamicConfig{Type: api.DynamicTypeWeather, TZ: "Asia/Shanghai", LocationLabel: "杭州"},
			WeatherData{TempC: "23", FeelsLikeC: "24", Humidity: "68", WindDisplay: "东北风3级", Code: 101,
				Forecast: []WeatherDay{{"今日", "多云", "17", "24", 101}, {"明日", "小雨", "15", "20", 305}, {"后天", "晴", "14", "22", 100}}}},
		"weather_alert": {api.DynamicConfig{Type: api.DynamicTypeWeatherAlert},
			WeatherAlertData{Items: []WeatherAlertItem{{Title: "浙江省杭州市气象台发布暴雨橙色预警信号", IssuedAt: &issued}, {Title: "中央气象台发布台风蓝色预警"}}}},
		"earthquake_report": {api.DynamicConfig{Type: api.DynamicTypeEarthquakeReport},
			EarthquakeData{UpdatedAt: renderedAt, Items: []EarthquakeItem{{OccurredAt: "2026-10-26 09:12:33", Longitude: "102.3", Latitude: "28.1", DepthKm: "10", Magnitude: "4.6", Location: "四川凉山州会理市"}, {OccurredAt: "2026-10-25 22:01:02", DepthKm: "8", Magnitude: "3.1", Location: "新疆喀什地区伽师县"}}}},
		"hot_list": {api.DynamicConfig{Type: api.DynamicTypeHotList, Source: "weibo"},
			HotListData{Items: []HotListItem{{Rank: 1, Title: "第一条热搜标题比较长会被省略号截断显示在墨水屏上"}, {Rank: 2, Title: "第二条"}, {Rank: 3, Title: "第三条"}, {Rank: 4, Title: "第四条"}}}},
		"dashboard": {api.DynamicConfig{Type: api.DynamicTypeDashboard, Template: &api.DashboardTemplateRef{Kind: "system", ID: "ai_quota_monitor"}},
			map[string]any{"service_label": "Claude Code", "plan_label": "Max 20x", "status_label": "正常", "primary_window_label": "5h窗口", "primary_used_percent": 68,
				"secondary_window_label": "周限额", "secondary_used_percent": 23, "primary_reset_at_label": "14:00", "secondary_reset_at_label": "周四 09:00", "updated_label": "10-26 10:30"}},
		"font_test": {api.DynamicConfig{Type: api.DynamicTypeFontTest, FontID: "fusion_pixel_12"}, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(tc.data)
			frame, err := r.Render(Input{Type: tc.cfg.Type, Config: tc.cfg, Data: data, RenderedAt: renderedAt})
			if err != nil {
				t.Fatal(err)
			}
			if len(frame) != api.FrameBytes {
				t.Fatalf("frame is %d bytes", len(frame))
			}
			path := filepath.Join("testdata", "frames", name+".png")
			if *update {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, framePNG(frame), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden; run with -update: %v", err)
			}
			if !bytes.Equal(framePNG(frame), want) {
				t.Fatalf("frame differs from %s; run with -update and review the image", path)
			}
		})
	}
}

func framePNG(frame []byte) []byte {
	img := image.NewGray(image.Rect(0, 0, api.FrameWidth, api.FrameHeight))
	for i := 0; i < api.FrameWidth*api.FrameHeight; i++ {
		if frame[i>>3]&(0x80>>(i&7)) != 0 {
			img.Pix[i] = 255
		}
	}
	var buf bytes.Buffer
	_ = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	return buf.Bytes()
}

func TestResolveTemplateFormats(t *testing.T) {
	data := map[string]any{"cost": 12.5, "n": 1234567.0, "tokens": 15300.0, "pct": 66.66, "ms": 1520.0, "list": []any{1.0, 2.0}, "rows": []any{map[string]any{"name": "a"}}}
	for tpl, want := range map[string]string{
		"{cost|usd2}":     "$12.50",
		"{n|int}":         "1,234,567",
		"{tokens|tokens}": "15.3K",
		"{pct|percent}":   "66.7%",
		"{ms|duration}":   "1.52s",
		"{list}":          "1 2",
		"{rows.0.name}!":  "a!",
		"{missing|usd2}x": "x",
		"{cost}":          "12.5",
	} {
		if got := ResolveTemplate(tpl, data); got != want {
			t.Errorf("%s: got %q, want %q", tpl, got, want)
		}
	}
}

func TestSummarizeAlert(t *testing.T) {
	s := SummarizeAlert("广西壮族自治区气象台发布暴雨橙色预警信号")
	if s.Kind != "暴雨" || s.LevelShort != "橙" || !s.Filled || s.SourceLabel != "广西" {
		t.Fatalf("unexpected summary %+v", s)
	}
}
