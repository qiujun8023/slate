package dynamic

import (
	"encoding/json"
	"strings"
	"time"

	"slate/internal/api"
	"slate/internal/render"
)

const (
	minDueDelay = 10 * time.Second
	// calendarWakeLag wakes devices a minute after local midnight so the new day is ready.
	calendarWakeLag = time.Minute
	backoffBase     = time.Minute
	backoffMax      = time.Hour
)

var calendarTypes = map[api.DynamicType]bool{
	api.DynamicTypeDailyCalendar: true, api.DynamicTypeMonthCalendar: true, api.DynamicTypeHistoryToday: true,
}

// schedule returns when devices should next show a new frame and when the server should
// start rendering it. Calendars follow local midnight; others use refreshIntervalSec.
// Font tests never refresh.
func schedule(cfg api.DynamicConfig, now time.Time) (nextRunAt, refreshDueAt *time.Time) {
	var next time.Time
	switch {
	case calendarTypes[cfg.Type]:
		loc := render.Location(cfg.TZ)
		local := now.In(loc)
		midnight := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)
		next = midnight.Add(calendarWakeLag)
		due := midnight
		if !due.After(now) {
			due = now.Add(minDueDelay)
		}
		return &next, &due
	case cfg.RefreshIntervalSec > 0:
		next = now.Add(time.Duration(cfg.RefreshIntervalSec) * time.Second)
		return &next, &next
	case cfg.Type == api.DynamicTypeWeather:
		next = now.Add(10 * time.Minute)
		return &next, &next
	}
	return nil, nil
}

// errorBackoff doubles from one minute to one hour with consecutive failures, so a
// failing source does not wake devices at the minimum interval.
func errorBackoff(attempts int32, now time.Time) time.Time {
	d := backoffBase
	for i := int32(1); i < attempts && d < backoffMax; i++ {
		d *= 2
	}
	return now.Add(min(d, backoffMax))
}

// canReuse reports whether previously fetched data may be rendered again after a fetch
// failure. Calendars are computed locally and never reuse; dashboards and font tests
// always do; network data is reused while it is fresh enough.
// SameSource reports whether data fetched for config a can be rendered under config b,
// so a source change never shows old data under the new title.
func SameSource(a, b api.DynamicConfig) bool {
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	case api.DynamicTypeWeather:
		return a.LocationID == b.LocationID && a.TZ == b.TZ
	case api.DynamicTypeWeatherAlert:
		return a.Province == b.Province
	case api.DynamicTypeHistoryToday, api.DynamicTypeHotList:
		return a.Source == b.Source
	}
	return true
}

func canReuse(cfg api.DynamicConfig, data json.RawMessage, renderedAt *time.Time, now time.Time) bool {
	if len(data) == 0 || string(data) == "null" {
		return false
	}
	switch cfg.Type {
	case api.DynamicTypeDailyCalendar, api.DynamicTypeMonthCalendar:
		return false
	case api.DynamicTypeDashboard, api.DynamicTypeFontTest:
		return true
	case api.DynamicTypeHistoryToday:
		var h render.HistoryTodayData
		if json.Unmarshal(data, &h) != nil || len(h.Items) == 0 {
			return false
		}
		t := now.In(render.Location(cfg.TZ))
		return strings.Join(strings.Fields(h.DateLabel), "") == monthDayCN(t)
	}
	var stamp struct {
		UpdatedAt time.Time `json:"updatedAt"`
	}
	fresh := time.Time{}
	if json.Unmarshal(data, &stamp) == nil && !stamp.UpdatedAt.IsZero() {
		fresh = stamp.UpdatedAt
	} else if renderedAt != nil {
		fresh = *renderedAt
	}
	if fresh.IsZero() {
		return false
	}
	return now.Sub(fresh) <= maxReuseAge(cfg)
}

func maxReuseAge(cfg api.DynamicConfig) time.Duration {
	interval := cfg.RefreshIntervalSec
	if interval == 0 {
		interval = 600
	}
	age := max(interval*3, 900)
	limit := map[api.DynamicType]int{api.DynamicTypeWeather: 43200}[cfg.Type]
	if limit == 0 {
		limit = 3600
	}
	return time.Duration(min(age, limit)) * time.Second
}

func monthDayCN(t time.Time) string {
	return itoa(int(t.Month())) + "月" + itoa(t.Day()) + "日"
}
