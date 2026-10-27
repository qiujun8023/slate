package render

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var traditionalFestivals = [][2]string{
	{"除夕", "除夕"}, {"春节", "春节"}, {"元宵", "元宵"}, {"上元", "元宵"}, {"龙抬头", "龙抬头"}, {"春龙节", "龙抬头"},
	{"上巳", "上巳"}, {"寒食", "寒食"}, {"清明", "清明"}, {"端午", "端午"}, {"七夕", "七夕"}, {"中元", "中元"},
	{"中秋", "中秋"}, {"重阳", "重阳"}, {"寒衣", "寒衣"}, {"下元", "下元"}, {"腊八", "腊八"}, {"小年", "小年"},
	{"冬至", "冬至"},
}

var leadingDigits = regexp.MustCompile(`^\d+`)

// FestivalShortName maps a festival label to its short traditional name, or "".
func FestivalShortName(v string) string {
	cleaned := strings.TrimSpace(leadingDigits.ReplaceAllString(v, ""))
	if cleaned == "" {
		return ""
	}
	for _, f := range traditionalFestivals {
		if strings.Contains(cleaned, f[0]) {
			return f[1]
		}
	}
	return ""
}

func (d drawer) dailyCalendar(data DailyCalendarData) {
	f := d.fonts
	ganzhi := strings.Join(nonEmpty(data.GanzhiYear, data.GanzhiMonth, data.GanzhiDay), " ")
	festival := FestivalShortName(data.Festival)
	yi := strings.Join(firstN(data.Yi, 5), " ")
	ji := strings.Join(firstN(data.Ji, 5), " ")
	monthDay := fmt.Sprintf("%02d/%02d", data.Month, data.Day)
	term := or(data.SolarTerm, data.NextSolarTerm)
	daysLabel := ""
	if data.NextSolarTermDays != nil {
		daysLabel = fmt.Sprintf("%d天后", *data.NextSolarTermDays)
		if *data.NextSolarTermDays == 0 {
			daysLabel = "今天"
		}
	}

	ruleY := round(float64(safeTop+safeBottom) / 2)
	upperTop := safeTop + 18
	upperBottom := ruleY - 16
	dateY := upperTop + round(float64(upperBottom-upperTop-f.DisplayLarge.LineHeight)/2)
	rightY := upperTop + round(float64(upperBottom-upperTop-(f.Sans16.LineHeight*3+14*2))/2)
	rightX := 202
	d.text(f.DisplayLarge, monthDay, 18, dateY, textOpts{maxWidth: 176, ellipsis: true})
	d.text(f.Sans16, or(data.WeekdayCN, "今日"), rightX, rightY, textOpts{maxWidth: contentRight - rightX, ellipsis: true})
	if lunar := or(data.LunarDate, data.Lunar); lunar != "" {
		d.text(f.Sans16, lunar, rightX, rightY+30, textOpts{maxWidth: contentRight - rightX, ellipsis: true})
	}
	if ganzhi != "" {
		d.text(f.Sans16, ganzhi, rightX, rightY+60, textOpts{maxWidth: contentRight - rightX, ellipsis: true})
	}
	d.rule(contentLeft, ruleY, contentWidth, true, Black)

	withDays := func(name string) string {
		if daysLabel == "" {
			return name
		}
		return name + " " + daysLabel
	}
	var label, value string
	switch {
	case data.SolarTerm != "":
		label, value = "今日节气", withDays(data.SolarTerm)
	case festival != "":
		label, value = "今日节日", festival
	case term != "":
		label, value = "下个节气", withDays(term)
	case ganzhi != "":
		label, value = "干支", ganzhi
	}
	if label != "" {
		d.infoRow(label, value, contentLeft, ruleY+20, contentWidth)
	}
	if yi != "" {
		d.labeledText("宜", yi, contentLeft, ruleY+56, contentWidth)
	}
	if ji != "" {
		d.labeledText("忌", ji, contentLeft, ruleY+98, contentWidth)
	}
}

func (d drawer) labeledText(label, value string, x, y, w int) {
	f := d.fonts.Sans16
	const boxW, boxH = 24, 22
	textY := y + round(float64(boxH-f.LineHeight)/2)
	d.c.StrokeRect(x, y, boxW, boxH, Black)
	d.text(f, label, x+boxW/2, textY, textOpts{align: alignCenter, maxWidth: boxW - 4})
	d.text(f, value, x+boxW+12, textY, textOpts{maxWidth: w - boxW - 12, ellipsis: true})
}

func (d drawer) infoRow(label, value string, x, y, w int) {
	f := d.fonts.Sans16
	labelText := label + ":"
	labelW := min(94, max(24, f.Width(labelText)+4))
	d.text(f, labelText, x, y+1, textOpts{maxWidth: labelW})
	d.text(f, value, x+labelW+8, y+1, textOpts{maxWidth: w - labelW - 8, ellipsis: true})
}

var weekdayShort = []string{"日", "一", "二", "三", "四", "五", "六"}

func (d drawer) monthCalendar(data MonthCalendarData, now time.Time) {
	f := d.fonts
	year, month, today := now.Date()
	days := data.Months[fmt.Sprintf("%d-%02d", year, int(month))]
	first := int(time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Weekday())
	total := DaysInMonth(year, month)
	const x0, y0, headerH = 14, 32, 24
	colW := (frameW - 28) / 7
	weekRows := (first + total + 6) / 7
	rowH := 42
	if weekRows > 1 {
		rowH = (frameH - (y0 + headerH) - 38) / (weekRows - 1)
	}
	for i, label := range weekdayShort {
		d.text(f.Sans16, label, x0+i*colW+colW/2, y0, textOpts{align: alignCenter, maxWidth: colW})
	}
	for day := 1; day <= total; day++ {
		pos := first + day - 1
		x := x0 + pos%7*colW
		y := y0 + headerH + pos/7*rowH
		color := Black
		if day == today {
			d.c.FillRect(x+colW/2-11, y+1, 22, 18, Black)
			color = White
		}
		d.text(f.Sans16, strconv.Itoa(day), x+colW/2, y+2, textOpts{align: alignCenter, maxWidth: colW - 4, color: color})
		info, ok := days[fmt.Sprintf("%d-%02d-%02d", year, int(month), day)]
		if !ok {
			continue
		}
		if sub := monthCellSubtitle(info); sub != "" {
			d.text(f.CalendarSub10, sub, x+colW/2, y+15, textOpts{align: alignCenter, maxWidth: colW - 2, ellipsis: true})
		}
	}
}

var (
	lunarYearPrefix = regexp.MustCompile(`^[甲乙丙丁戊己庚辛壬癸子丑寅卯辰巳午未申酉戌亥]+年\s*`)
	lunarMonthDay   = regexp.MustCompile(`^(闰?[正一二三四五六七八九十冬腊]+)月(.+)$`)
)

func monthCellSubtitle(day CalendarDay) string {
	if day.SolarTerm != "" {
		return limitRunes(day.SolarTerm, 3)
	}
	if festival := FestivalShortName(day.Festival); festival != "" {
		return festival
	}
	cleaned := strings.TrimPrefix(or(day.LunarDate, day.Lunar), "农历")
	cleaned = lunarYearPrefix.ReplaceAllString(cleaned, "")
	m := lunarMonthDay.FindStringSubmatch(cleaned)
	if m == nil {
		return limitRunes(cleaned, 3)
	}
	if m[2] == "初一" {
		return m[1] + "月"
	}
	return m[2]
}

// SortHistory orders items newest first and keeps five.
func SortHistory(items []HistoryTodayItem) []HistoryTodayItem {
	out := append([]HistoryTodayItem(nil), items...)
	key := func(y string) int {
		if strings.HasPrefix(y, "前") {
			n, _ := strconv.Atoi(strings.TrimPrefix(y, "前"))
			return -n
		}
		n, _ := strconv.Atoi(y)
		return n
	}
	sort.SliceStable(out, func(i, j int) bool { return key(out[i].Year) > key(out[j].Year) })
	return firstN(out, 5)
}

func (d drawer) historyToday(data HistoryTodayData) {
	f := d.fonts.Sans16
	items := SortHistory(data.Items)
	if len(items) == 0 {
		items = []HistoryTodayItem{{Display: fallbackText}}
	}
	const timelineX, dotSize = 58, 7
	textX := timelineX + 18
	rowStep := map[int]int{5: 49, 4: 58, 3: 70}[len(items)]
	if rowStep == 0 {
		rowStep = 82
	}
	firstCenterY := safeTop + 36
	dotRadius := dotSize / 2
	top := firstCenterY - 18
	bottom := firstCenterY + rowStep*(len(items)-1) + 18
	d.c.VLine(timelineX, top, bottom-top+1, Black)
	for i, item := range items {
		centerY := firstCenterY + i*rowStep
		textY := centerY - round(float64(f.LineHeight*2+2)/2)
		if item.Year != "" {
			d.text(f, item.Year, timelineX-12, centerY-round(float64(f.LineHeight)/2), textOpts{align: alignRight, maxWidth: 42, ellipsis: true})
		}
		d.c.FillRect(timelineX-dotRadius, centerY-dotRadius, dotSize, dotSize, Black)
		d.text(f, item.Display, textX, textY, textOpts{maxWidth: contentRight - textX, maxLines: 2, ellipsis: true, lineGap: 2})
	}
}

func DaysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func firstN[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}
