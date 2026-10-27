package dynamic

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"slate/internal/api"
	"slate/internal/render"
)

// speechText is what TTS reads for a dynamic frame; "" means the frame has no audio.
func speechText(cfg api.DynamicConfig, data json.RawMessage, now time.Time) string {
	now = now.In(render.Location(cfg.TZ))
	switch cfg.Type {
	case api.DynamicTypeDailyCalendar:
		var d render.DailyCalendarData
		if json.Unmarshal(data, &d) != nil {
			return ""
		}
		head := fmt.Sprintf("今天是%d月%d日", d.Month, d.Day)
		if d.WeekdayCN != "" {
			head += "，" + d.WeekdayCN
		}
		return sentence(head, or(d.LunarDate, d.Lunar), prefixed("今日节气", d.SolarTerm), prefixed("今天是", d.Festival))
	case api.DynamicTypeMonthCalendar:
		var d render.MonthCalendarData
		_ = json.Unmarshal(data, &d)
		day := d.Months[fmt.Sprintf("%d-%02d", now.Year(), int(now.Month()))][now.Format("2006-01-02")]
		return sentence(fmt.Sprintf("现在是%d年%d月", now.Year(), int(now.Month())), fmt.Sprintf("今天%d月%d日", int(now.Month()), now.Day()),
			day.LunarDate, prefixed("今日节气", day.SolarTerm), prefixed("今天是", day.Festival))
	case api.DynamicTypeWeather:
		var d render.WeatherData
		_ = json.Unmarshal(data, &d)
		return sentence(or(cfg.LocationLabel, "本地")+"今天天气，"+or(d.Summary, "天气数据暂不可用"),
			suffixed(d.TempC, "度"), prefixed("体感", suffixed(d.FeelsLikeC, "度")), prefixed("湿度", suffixed(d.Humidity, "%")), dashless(d.WindDisplay))
	case api.DynamicTypeHistoryToday:
		var d render.HistoryTodayData
		if json.Unmarshal(data, &d) != nil || len(d.Items) == 0 {
			return ""
		}
		parts := []string{"历史上的" + strings.Join(strings.Fields(or(d.DateLabel, monthDayCN(now))), "")}
		for _, item := range render.SortHistory(d.Items) {
			year := "公元" + item.Year + "年"
			if strings.HasPrefix(item.Year, "前") {
				year = "公元前" + strings.TrimPrefix(item.Year, "前") + "年"
			}
			parts = append(parts, year+"，"+item.Display)
		}
		return sentence(parts...)
	case api.DynamicTypeWeatherAlert:
		var d render.WeatherAlertData
		_ = json.Unmarshal(data, &d)
		region := "全国"
		if p := or(d.Province, cfg.Province); p != "" {
			region = render.ShortRegion(p)
		}
		var lines []string
		for _, item := range firstN(d.Items, 3) {
			if item.Title != "" {
				lines = append(lines, alertSpeech(item.Title))
			}
		}
		if len(lines) == 0 {
			return region + "暂无气象预警"
		}
		return sentence(append([]string{fmt.Sprintf("%s气象预警，播报最新%s条", region, cnCount(len(lines)))}, lines...)...)
	case api.DynamicTypeEarthquakeReport:
		var d render.EarthquakeData
		_ = json.Unmarshal(data, &d)
		if len(d.Items) == 0 {
			return "暂无地震速报"
		}
		latest := d.Items[0]
		depth := ""
		if latest.DepthKm != "" && latest.DepthKm != "-" && latest.DepthKm != "--" {
			depth = "震源深度" + latest.DepthKm + "千米"
		}
		first := sentence("最新一条，"+or(latest.Location, "未知位置"), prefixed("震级", suffixed(latest.Magnitude, "级")), depth,
			prefixed("发生时间", spokenTime(latest.OccurredAt)))
		var rest []string
		for _, item := range firstN(d.Items[1:], 3) {
			brief := or(item.Location, "未知位置")
			if item.Magnitude != "" {
				brief += "，震级" + item.Magnitude + "级"
			}
			rest = append(rest, brief)
		}
		restText := ""
		if len(rest) > 0 {
			restText = fmt.Sprintf("其余%s条：%s", cnCount(len(rest)), strings.Join(rest, "；"))
		}
		return sentence("中国地震台网最新速报", first, restText)
	}
	return ""
}

var trailingPunct = regexp.MustCompile(`[。；;,，]+$`)

// sentence joins non-empty parts with "。" and caps the result at 500 characters.
func sentence(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = trailingPunct.ReplaceAllString(strings.TrimSpace(p), ""); p != "" {
			out = append(out, p)
		}
	}
	return limitRunes(strings.Join(out, "。"), 500)
}

func prefixed(prefix, v string) string {
	if strings.TrimSpace(v) == "" {
		return ""
	}
	return prefix + v
}

func suffixed(v, suffix string) string {
	if v = dashless(v); v == "" {
		return ""
	}
	return v + suffix
}

func dashless(v string) string {
	if v = strings.TrimSpace(v); v == "--" {
		return ""
	}
	return v
}

var alertTitle = regexp.MustCompile(`^(.*?)发布(.+?)预警(?:信号)?$`)

func alertSpeech(title string) string {
	normalized := strings.Join(strings.Fields(title), "")
	m := alertTitle.FindStringSubmatch(normalized)
	if m == nil {
		return strings.ReplaceAll(normalized, "预警信号", "预警")
	}
	signal := strings.TrimSuffix(strings.TrimSuffix(m[2], "信号"), "预警")
	if source := render.ShortRegion(m[1]); source != "" {
		return source + "发布" + signal + "预警"
	}
	return signal + "预警"
}

func cnCount(n int) string {
	if n >= 0 && n <= 4 {
		return []string{"零", "一", "二", "三", "四"}[n]
	}
	return strconv.Itoa(n)
}

var spokenTimePattern = regexp.MustCompile(`(?:(\d{4})[-/年])?(\d{1,2})[-/月](\d{1,2})日?\s+(\d{1,2}):(\d{2})`)

func spokenTime(v string) string {
	m := spokenTimePattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return strings.TrimSpace(v)
	}
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	return fmt.Sprintf("%d月%d日%d点%s分", atoi(m[2]), atoi(m[3]), atoi(m[4]), m[5])
}

func limitRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func firstN[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func itoa(n int) string { return strconv.Itoa(n) }
