package dynamic

import (
	"container/list"
	"fmt"
	"sort"
	"time"

	"github.com/6tail/lunar-go/calendar"

	"slate/internal/render"
)

var weekdayCN = []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

// The jieqi table repeats a few terms of the next cycle under pinyin keys.
var jieQiAliases = map[string]string{
	"DONG_ZHI": "冬至", "XIAO_HAN": "小寒", "DA_HAN": "大寒", "LI_CHUN": "立春", "YU_SHUI": "雨水", "JING_ZHE": "惊蛰",
}

func calendarDay(year, month, day int) render.CalendarDay {
	solar := calendar.NewSolarFromYmd(year, month, day)
	lunar := solar.GetLunar()
	out := render.CalendarDay{
		GanzhiYear:  lunar.GetYearInGanZhiExact() + "年",
		GanzhiMonth: lunar.GetMonthInGanZhiExact() + "月",
		GanzhiDay:   lunar.GetDayInGanZhiExact() + "日",
		LunarDate:   "农历" + lunar.GetMonthInChinese() + "月" + lunar.GetDayInChinese(),
		Yi:          firstStrings(lunar.GetDayYi(), 5),
		Ji:          firstStrings(lunar.GetDayJi(), 5),
	}
	out.Lunar = out.GanzhiYear + " " + lunar.GetMonthInChinese() + "月" + lunar.GetDayInChinese()

	type term struct {
		name string
		key  int
		days int
	}
	today := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	key := year*10000 + month*100 + day
	var terms []term
	for name, s := range lunar.GetJieQiTable() {
		k := s.GetYear()*10000 + s.GetMonth()*100 + s.GetDay()
		if k < key {
			continue
		}
		if alias, ok := jieQiAliases[name]; ok {
			name = alias
		}
		at := time.Date(s.GetYear(), time.Month(s.GetMonth()), s.GetDay(), 0, 0, 0, 0, time.UTC)
		terms = append(terms, term{name, k, int(at.Sub(today).Hours() / 24)})
	}
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].key != terms[j].key {
			return terms[i].key < terms[j].key
		}
		return terms[i].name < terms[j].name
	})
	if len(terms) > 0 {
		out.NextSolarTerm = terms[0].name
		out.NextSolarTermDays = &terms[0].days
		if terms[0].days == 0 {
			out.SolarTerm = terms[0].name
		}
	}
	for _, l := range []*list.List{lunar.GetFestivals(), lunar.GetOtherFestivals(), solar.GetFestivals(), solar.GetOtherFestivals()} {
		for e := l.Front(); e != nil && out.Festival == ""; e = e.Next() {
			if s, ok := e.Value.(string); ok {
				out.Festival = render.FestivalShortName(s)
			}
		}
	}
	return out
}

func firstStrings(l *list.List, n int) []string {
	out := []string{}
	for e := l.Front(); e != nil && len(out) < n; e = e.Next() {
		if s, ok := e.Value.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func dailyCalendar(now time.Time) render.DailyCalendarData {
	y, m, d := now.Date()
	return render.DailyCalendarData{Year: y, Month: int(m), Day: d, WeekdayCN: weekdayCN[now.Weekday()], CalendarDay: calendarDay(y, int(m), d)}
}

// monthCalendar covers the current and the next month.
func monthCalendar(now time.Time) render.MonthCalendarData {
	out := render.MonthCalendarData{Months: map[string]map[string]render.CalendarDay{}}
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for _, start := range []time.Time{first, first.AddDate(0, 1, 0)} {
		y, m := start.Year(), start.Month()
		days := map[string]render.CalendarDay{}
		for d := 1; d <= render.DaysInMonth(y, m); d++ {
			days[fmt.Sprintf("%d-%02d-%02d", y, int(m), d)] = calendarDay(y, int(m), d)
		}
		out.Months[fmt.Sprintf("%d-%02d", y, int(m))] = days
	}
	return out
}
