package api

import "encoding/json"

type DynamicType string

const (
	DynamicTypeDailyCalendar    DynamicType = "daily_calendar"
	DynamicTypeMonthCalendar    DynamicType = "month_calendar"
	DynamicTypeWeather          DynamicType = "weather"
	DynamicTypeHistoryToday     DynamicType = "history_today"
	DynamicTypeWeatherAlert     DynamicType = "weather_alert"
	DynamicTypeEarthquakeReport DynamicType = "earthquake_report"
	DynamicTypeDashboard        DynamicType = "dashboard"
	DynamicTypeFontTest         DynamicType = "font_test"
	DynamicTypeHotList          DynamicType = "hot_list"
)

var DynamicTypes = []DynamicType{
	DynamicTypeDailyCalendar, DynamicTypeMonthCalendar, DynamicTypeWeather, DynamicTypeHistoryToday, DynamicTypeWeatherAlert,
	DynamicTypeEarthquakeReport, DynamicTypeDashboard, DynamicTypeFontTest, DynamicTypeHotList,
}

// DynamicConfig is one flat shape for every dynamic type; fields that do not apply to
// Type are dropped by normalization. Field use per type:
//   - daily_calendar, month_calendar: tz, audio*, refreshIntervalSec
//   - weather: tz, locationId, locationLabel, audio*, refreshIntervalSec
//   - history_today: tz, source (wikipedia | baidu_baike), audio*, refreshIntervalSec
//   - weather_alert: province, audio*, refreshIntervalSec
//   - earthquake_report: audio*, refreshIntervalSec
//   - dashboard: template, refreshIntervalSec
//   - font_test: fontId, invert
//   - hot_list: source (hot-list source ID), refreshIntervalSec
type DynamicConfig struct {
	Type               DynamicType           `json:"type"`
	TZ                 string                `json:"tz,omitempty"`
	AudioEnabled       bool                  `json:"audioEnabled,omitempty"`
	AudioVoice         string                `json:"audioVoice,omitempty"`
	RefreshIntervalSec int                   `json:"refreshIntervalSec,omitempty"`
	LocationID         string                `json:"locationId,omitempty"`
	LocationLabel      string                `json:"locationLabel,omitempty"`
	Source             string                `json:"source,omitempty"`
	Province           string                `json:"province,omitempty"`
	Template           *DashboardTemplateRef `json:"template,omitempty"`
	FontID             string                `json:"fontId,omitempty"`
	Invert             bool                  `json:"invert,omitempty"`
}

type DashboardTemplateRef struct {
	Kind     string             `json:"kind" tstype:"'system' | 'custom'"`
	ID       string             `json:"id,omitempty"`
	Template *DashboardTemplate `json:"template,omitempty"`
}

type DashboardTemplate struct {
	Version int              `json:"version"`
	Name    string           `json:"name,omitempty"`
	Blocks  []DashboardBlock `json:"blocks"`
}

// DashboardBlock is one flat shape for text, metric, progress, sparkline, line and rect
// blocks. Bindings are strings like "{path.to.value|format}".
type DashboardBlock struct {
	Type string `json:"type" tstype:"'text' | 'metric' | 'progress' | 'sparkline' | 'line' | 'rect'"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    int    `json:"w"`
	H    int    `json:"h"`
	X1   int    `json:"x1"`
	Y1   int    `json:"y1"`
	X2   int    `json:"x2"`
	Y2   int    `json:"y2"`

	Value         string          `json:"value,omitempty"`
	Label         string          `json:"label,omitempty"`
	FontSize      int             `json:"fontSize,omitempty"`
	Align         string          `json:"align,omitempty"`
	Color         string          `json:"color,omitempty"`
	MaxLines      int             `json:"maxLines,omitempty"`
	Sparkline     json.RawMessage `json:"sparkline,omitempty" tstype:"string | number[]"`
	Values        json.RawMessage `json:"values,omitempty" tstype:"string | number[]"`
	Max           string          `json:"max,omitempty"`
	ValueText     string          `json:"valueText,omitempty"`
	Percentage    json.RawMessage `json:"percentage,omitempty" tstype:"string | number"`
	LabelFontSize int             `json:"labelFontSize,omitempty"`
	ValueFontSize int             `json:"valueFontSize,omitempty"`
	BarHeight     int             `json:"barHeight,omitempty"`
	Style         string          `json:"style,omitempty"`
	Stroke        *bool           `json:"stroke,omitempty"`
	Fill          string          `json:"fill,omitempty"`
}

// Catalog lists the fixed choices the web UI offers.
type Catalog struct {
	TTSVoices             []string                  `json:"ttsVoices"`
	DefaultTTSVoice       string                    `json:"defaultTtsVoice"`
	HotListSources        []HotListSourceInfo       `json:"hotListSources"`
	Fonts                 []FontInfo                `json:"fonts"`
	DashboardTemplates    []DashboardSystemTemplate `json:"dashboardTemplates"`
	DashboardStarter      DashboardTemplate         `json:"dashboardStarter"`
	WeatherAlertProvinces []string                  `json:"weatherAlertProvinces"`
}

type HotListSourceInfo struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	ShortLabel   string `json:"shortLabel"`
	ChannelLabel string `json:"channelLabel,omitempty"`
	Kind         string `json:"kind" tstype:"'general' | 'news' | 'tech' | 'community' | 'commerce'"`
	Mainstream   bool   `json:"mainstream"`
}

type FontInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	SizePx  int    `json:"sizePx"`
	Kind    string `json:"kind" tstype:"'cjk' | 'latin' | 'display' | 'icon'"`
	Hint    string `json:"hint"`
	Note    string `json:"note"`
	Source  string `json:"source"`
	License string `json:"license"`
}

type DashboardSystemTemplate struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	Description string            `json:"description"`
	Template    DashboardTemplate `json:"template"`
}
