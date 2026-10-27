package render

import (
	"regexp"
	"strings"
	"time"
)

func (d drawer) weather(data WeatherData) {
	f := d.fonts
	temp := or(data.TempC, "--")
	feels := or(data.FeelsLikeC, "")
	humidity := or(data.Humidity, "--")
	wind := or(data.WindDisplay, "--")

	forecastTop := safeTop + 132
	heroCenterY := round(float64(safeTop+forecastTop) / 2)
	const leftAreaW = 232
	halfW := leftAreaW / 2
	iconCx := contentLeft + round(float64(halfW)/2)
	tempCenterX := contentLeft + halfW + round(float64(halfW)/2) - 8
	tempFB := f.fallback(f.DisplayLarge)
	top, bottom, ok := visualBounds(f.DisplayLarge, tempFB, temp)
	if !ok {
		top, bottom = 0, 0
	}
	tempVisualH := bottom - top
	if tempVisualH == 0 {
		tempVisualH = 40
	}
	tempY := round(float64(heroCenterY)-float64(tempVisualH)/2) - top

	d.icon(WeatherIcon(data.Code, 70), iconCx, heroCenterY)
	tempX := round(float64(tempCenterX) - float64(f.DisplayLarge.Width(temp))/2)
	d.text(f.DisplayLarge, temp, tempX, tempY, textOpts{maxWidth: 92, ellipsis: true})
	visible := ellipsize(f.DisplayLarge, tempFB, temp, 92)
	d.text(f.Sans16, "°", tempX+widthFallback(f.DisplayLarge, tempFB, visible)+4, tempY+18, textOpts{maxWidth: 20})

	metrics := []string{"体感 --", "湿度 --", wind}
	if feels != "" && feels != "--" {
		metrics[0] = "体感 " + feels + "°"
	}
	if humidity != "--" {
		metrics[1] = "湿度 " + humidity + "%"
	}
	const metricX, metricGap = 274, 30
	metricY := round(float64(heroCenterY) - float64(f.Sans16.LineHeight+metricGap*(len(metrics)-1))/2)
	for i, line := range metrics {
		d.text(f.Sans16, line, metricX, metricY+i*metricGap, textOpts{maxWidth: contentRight - metricX, ellipsis: true})
	}

	d.rule(contentLeft, forecastTop, contentWidth, true, Black)
	colW := contentWidth / 3
	labels := []string{"今日", "明日", "后天"}
	for i := 0; i < 3; i++ {
		var day WeatherDay
		if i < len(data.Forecast) {
			day = data.Forecast[i]
		} else {
			day.Code = 999
		}
		x := contentLeft + i*colW
		if i > 0 {
			d.vrule(x, forecastTop+10, 110, true)
		}
		center := round(float64(x) + float64(colW)/2)
		d.text(f.Sans16, or(day.Label, labels[i]), center, forecastTop+14, textOpts{align: alignCenter, maxWidth: colW - 12, ellipsis: true})
		d.icon(WeatherIcon(day.Code, 30), center, forecastTop+55)
		d.text(f.Metric12, or(temperatureRange(day.TempMin, day.TempMax), "--"), center, forecastTop+84, textOpts{align: alignCenter, maxWidth: colW - 12, ellipsis: true})
		d.text(f.Sans16, or(day.Text, "--"), center, forecastTop+108, textOpts{align: alignCenter, maxWidth: colW - 12, ellipsis: true})
	}
}

func (d drawer) icon(m *Mask, cx, cy int) {
	if m != nil {
		d.c.Mask(m, round(float64(cx)-float64(m.W)/2), round(float64(cy)-float64(m.H)/2), Black)
	}
}

func temperatureRange(lo, hi string) string {
	bound := func(v string) string {
		v = strings.TrimRight(strings.TrimSpace(v), "°")
		if v == "--" {
			return ""
		}
		return v
	}
	l, h := bound(lo), bound(hi)
	switch {
	case l != "" && h != "":
		return l + "°~" + h + "°"
	case l != "":
		return l + "°"
	case h != "":
		return h + "°"
	}
	return ""
}

func (d drawer) weatherAlert(data WeatherAlertData, now time.Time) {
	f := d.fonts
	items := firstN(data.Items, 9)
	if len(items) == 0 {
		text := "暂无气象预警"
		if data.Province != "" {
			text = AlertSourceLabel(data.Province) + "暂无预警"
		}
		d.text(f.Sans16, text, frameW/2, 146, textOpts{align: alignCenter, maxWidth: contentWidth})
		return
	}
	startY := statusBarH + 7
	rowH := min(30, (frameH-startY-4)/len(items))
	const badgeW, badgeH, timeW = 34, 22, 44
	titleX := contentLeft + 44
	titleW := contentRight - titleX - timeW - 10
	for i, item := range items {
		y := startY + i*rowH
		centerY := y + rowH/2
		s := SummarizeAlert(or(item.Title, fallbackText))
		label := s.Kind
		if label == "" {
			label = s.LevelShort
		}
		d.badge(label, contentLeft, round(float64(centerY)-badgeH/2.0), badgeW, badgeH, s.Filled)
		d.textCenteredY(f.Sans16, s.Line(), titleX, float64(centerY), textOpts{maxWidth: titleW, ellipsis: true})
		if item.IssuedAt != nil {
			d.textCenteredY(f.Metric12, item.IssuedAt.In(now.Location()).Format("15:04"), contentRight, float64(centerY), textOpts{align: alignRight, maxWidth: timeW, ellipsis: true})
		}
		if i < len(items)-1 {
			d.rule(titleX, y+rowH-1, contentRight-titleX, true, Black)
		}
	}
}

type AlertSummary struct {
	Headline, Source, SourceLabel, Kind, LevelShort string
	Filled                                          bool
}

func (s AlertSummary) Line() string {
	if s.LevelShort != "" && s.SourceLabel != "" {
		return s.LevelShort + " · " + s.SourceLabel
	}
	return or(s.SourceLabel, or(s.LevelShort, s.Headline))
}

var (
	alertPublish  = regexp.MustCompile(`^(.*?)发布(.+?)预警(?:信号)?$`)
	alertSuffix   = regexp.MustCompile(`预警(?:信号)?$`)
	alertLevel    = regexp.MustCompile(`(红色|橙色|黄色|蓝色)`)
	alertLevelEnd = regexp.MustCompile(`(红色|橙色|黄色|蓝色)$`)
	alertNoise    = regexp.MustCompile(`灾害|气象|预警|信号`)
	whitespace    = regexp.MustCompile(`\s+`)
)

// SummarizeAlert splits "XX气象台发布暴雨橙色预警信号" into source, kind and level.
func SummarizeAlert(title string) AlertSummary {
	normalized := whitespace.ReplaceAllString(title, "")
	var source, signal string
	if m := alertPublish.FindStringSubmatch(normalized); m != nil {
		source, signal = m[1], m[2]
	} else {
		signal = alertSuffix.ReplaceAllString(normalized, "")
	}
	level := alertLevel.FindString(signal)
	if level == "" {
		level = alertLevel.FindString(normalized)
	}
	kindSignal := alertLevelEnd.ReplaceAllString(signal, "")
	if kindSignal == "" {
		kindSignal = signal
	}
	if kindSignal == "" {
		kindSignal = "气象"
	}
	short := map[string]string{"红色": "红", "橙色": "橙", "黄色": "黄", "蓝色": "蓝"}[level]
	if short == "" {
		short = "警"
	}
	return AlertSummary{
		Headline:    level + kindSignal + "预警",
		Source:      source,
		SourceLabel: AlertSourceLabel(source),
		Kind:        alertKind(kindSignal),
		LevelShort:  short,
		Filled:      strings.Contains(normalized, "红色") || strings.Contains(normalized, "橙色"),
	}
}

func alertKind(signal string) string {
	compact := alertNoise.ReplaceAllString(signal, "")
	for _, k := range [][2]string{
		{"雷雨大风", "雷暴"}, {"雷电", "雷电"}, {"暴雨", "暴雨"}, {"大风", "大风"}, {"台风", "台风"}, {"高温", "高温"},
		{"雾", "大雾"}, {"山洪", "山洪"}, {"暴雪", "暴雪"}, {"寒潮", "寒潮"}, {"冰雹", "冰雹"},
	} {
		if strings.Contains(compact, k[0]) {
			return k[1]
		}
	}
	if len([]rune(compact)) <= 2 {
		return or(compact, "预警")
	}
	return limitRunes(compact, 2)
}

var (
	regionReplacer = strings.NewReplacer("广西壮族自治区", "广西", "宁夏回族自治区", "宁夏", "新疆维吾尔自治区", "新疆",
		"内蒙古自治区", "内蒙古", "西藏自治区", "西藏", "特别行政区", "")
	officeSuffix = regexp.MustCompile(`(气象台|气象局)$`)
	regionNoise  = regexp.MustCompile(`省|市|气象台|气象局`)
)

// AlertSourceLabel shortens an issuing office such as "广西壮族自治区气象台" to "广西".
func AlertSourceLabel(source string) string {
	text := whitespace.ReplaceAllString(source, "")
	if text == "" {
		return ""
	}
	if strings.Contains(text, "中央气象台") {
		return "中央气象台"
	}
	return officeSuffix.ReplaceAllString(regionReplacer.Replace(text), "")
}

// ShortRegion shortens a region for speech, dropping province and city suffixes.
func ShortRegion(source string) string {
	text := whitespace.ReplaceAllString(source, "")
	if text == "" {
		return ""
	}
	if strings.Contains(text, "中央气象台") {
		return "中央气象台"
	}
	return limitRunes(regionNoise.ReplaceAllString(regionReplacer.Replace(text), ""), 12)
}
