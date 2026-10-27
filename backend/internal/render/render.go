package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"slate/internal/api"
)

// Input is everything a frame depends on; the same input always renders the same bytes.
type Input struct {
	Type       api.DynamicType
	Config     api.DynamicConfig
	Data       json.RawMessage
	RenderedAt time.Time
}

type Renderer struct{ fonts *Fonts }

func New() (*Renderer, error) {
	fonts, err := LoadFonts()
	if err != nil {
		return nil, err
	}
	return &Renderer{fonts}, nil
}

// Render returns a packed 400×300 frame. The status bar strip stays white for the
// firmware to draw over.
func (r *Renderer) Render(in Input) ([]byte, error) {
	c := NewCanvas(frameW, frameH)
	d := drawer{c, r.fonts}
	loc := Location(in.Config.TZ)
	var err error
	switch in.Type {
	case api.DynamicTypeDailyCalendar:
		var data DailyCalendarData
		if err = decode(in.Data, &data); err == nil {
			d.dailyCalendar(data)
		}
	case api.DynamicTypeMonthCalendar:
		var data MonthCalendarData
		if err = decode(in.Data, &data); err == nil {
			d.monthCalendar(data, in.RenderedAt.In(loc))
		}
	case api.DynamicTypeHistoryToday:
		var data HistoryTodayData
		if err = decode(in.Data, &data); err == nil {
			d.historyToday(data)
		}
	case api.DynamicTypeWeather:
		var data WeatherData
		if err = decode(in.Data, &data); err == nil {
			d.weather(data)
		}
	case api.DynamicTypeWeatherAlert:
		var data WeatherAlertData
		if err = decode(in.Data, &data); err == nil {
			d.weatherAlert(data, in.RenderedAt.In(loc))
		}
	case api.DynamicTypeEarthquakeReport:
		var data EarthquakeData
		if err = decode(in.Data, &data); err == nil {
			d.earthquake(data, in.RenderedAt.In(loc))
		}
	case api.DynamicTypeDashboard:
		var data map[string]any
		if len(in.Data) > 0 && string(in.Data) != "null" {
			err = json.Unmarshal(in.Data, &data)
		}
		if err == nil {
			d.dashboard(in.Config, data)
		}
	case api.DynamicTypeFontTest:
		err = d.fontTest(in.Config)
	case api.DynamicTypeHotList:
		var data HotListData
		if err = decode(in.Data, &data); err == nil {
			d.hotList(data)
		}
	default:
		d.text(r.fonts.Sans16, "未知动态类型 "+string(in.Type), 200, 140, textOpts{align: alignCenter, maxWidth: 320, maxLines: 2, ellipsis: true})
	}
	if err != nil {
		return nil, fmt.Errorf("动态数据格式错误: %w", err)
	}
	c.FillRect(0, 0, frameW, statusBarH, White)
	return c.Packed(), nil
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// Location resolves an IANA zone, defaulting to Asia/Shanghai.
func Location(tz string) *time.Location {
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}

type CalendarDay struct {
	Lunar             string   `json:"lunar"`
	LunarDate         string   `json:"lunarDate"`
	GanzhiYear        string   `json:"ganzhiYear"`
	GanzhiMonth       string   `json:"ganzhiMonth"`
	GanzhiDay         string   `json:"ganzhiDay"`
	SolarTerm         string   `json:"solarTerm,omitempty"`
	NextSolarTerm     string   `json:"nextSolarTerm,omitempty"`
	NextSolarTermDays *int     `json:"nextSolarTermDays,omitempty"`
	Festival          string   `json:"festival,omitempty"`
	Yi                []string `json:"yi"`
	Ji                []string `json:"ji"`
}

type DailyCalendarData struct {
	Year      int    `json:"year"`
	Month     int    `json:"month"`
	Day       int    `json:"day"`
	WeekdayCN string `json:"weekdayCN"`
	CalendarDay
}

// MonthCalendarData holds the current and next month keyed "2006-01" then "2006-01-02".
type MonthCalendarData struct {
	Months map[string]map[string]CalendarDay `json:"months"`
}

type HistoryTodayData struct {
	DateLabel string             `json:"dateLabel"`
	Items     []HistoryTodayItem `json:"items"`
}

type HistoryTodayItem struct {
	Year    string `json:"year"`
	Display string `json:"display"`
}

type WeatherData struct {
	TempC       string       `json:"tempC"`
	FeelsLikeC  string       `json:"feelsLikeC"`
	Humidity    string       `json:"humidity"`
	Pressure    string       `json:"pressure"`
	WindDisplay string       `json:"windDisplay"`
	Summary     string       `json:"summary"`
	Code        int          `json:"code"`
	ObsTime     string       `json:"obsTime"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	Forecast    []WeatherDay `json:"forecast"`
}

type WeatherDay struct {
	Label   string `json:"label"`
	Text    string `json:"text"`
	TempMin string `json:"tempMin"`
	TempMax string `json:"tempMax"`
	Code    int    `json:"code"`
}

type WeatherAlertData struct {
	Province  string             `json:"province"`
	UpdatedAt time.Time          `json:"updatedAt"`
	Items     []WeatherAlertItem `json:"items"`
}

type WeatherAlertItem struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	IssuedAt *time.Time `json:"issuedAt,omitempty"`
	URL      string     `json:"url,omitempty"`
}

type EarthquakeData struct {
	UpdatedAt time.Time        `json:"updatedAt"`
	Items     []EarthquakeItem `json:"items"`
}

type EarthquakeItem struct {
	ID         string `json:"id"`
	OccurredAt string `json:"occurredAt"`
	Longitude  string `json:"longitude"`
	Latitude   string `json:"latitude"`
	DepthKm    string `json:"depthKm"`
	Magnitude  string `json:"magnitude"`
	Location   string `json:"location"`
	EventType  string `json:"eventType"`
}

type HotListData struct {
	Source      string        `json:"source"`
	SourceLabel string        `json:"sourceLabel"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	Items       []HotListItem `json:"items"`
}

type HotListItem struct {
	Rank  int    `json:"rank"`
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
}

func or(v, fallback string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return fallback
}

func limitRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
