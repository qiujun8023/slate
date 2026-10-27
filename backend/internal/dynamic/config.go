// Package dynamic fetches, renders and schedules dynamic content.
package dynamic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"slate/internal/api"
	"slate/internal/hotlist"
	"slate/internal/render"
)

// ValidationError is a 400 response whose message names the bad field.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string   { return e.Message }
func (e *ValidationError) HTTPStatus() int { return 400 }

func invalid(format string, args ...any) error {
	return &ValidationError{fmt.Sprintf(format, args...)}
}

var WeatherAlertProvinces = []string{
	"北京市", "上海市", "天津市", "重庆市", "黑龙江省", "吉林省", "辽宁省", "内蒙古自治区", "河北省", "山西省", "陕西省", "山东省",
	"新疆维吾尔自治区", "西藏自治区", "青海省", "甘肃省", "宁夏回族自治区", "河南省", "江苏省", "湖北省", "浙江省", "安徽省", "福建省",
	"江西省", "湖南省", "贵州省", "四川省", "广东省", "云南省", "广西壮族自治区", "海南省",
}

var audioTypes = map[api.DynamicType]bool{
	api.DynamicTypeDailyCalendar: true, api.DynamicTypeMonthCalendar: true, api.DynamicTypeWeather: true,
	api.DynamicTypeHistoryToday: true, api.DynamicTypeWeatherAlert: true, api.DynamicTypeEarthquakeReport: true,
}

// refreshBounds is the allowed refreshIntervalSec range and its default (0 means unset).
var refreshBounds = map[api.DynamicType][3]int{
	api.DynamicTypeDailyCalendar:    {300, 86400, 0},
	api.DynamicTypeMonthCalendar:    {300, 86400, 0},
	api.DynamicTypeWeather:          {300, 86400, 0},
	api.DynamicTypeHistoryToday:     {300, 86400, 0},
	api.DynamicTypeWeatherAlert:     {300, 86400, 600},
	api.DynamicTypeEarthquakeReport: {300, 86400, 600},
	api.DynamicTypeHotList:          {300, 86400, 600},
	api.DynamicTypeDashboard:        {60, 86400, 600},
}

// NormalizeConfig validates a config, fills defaults and drops fields the type does not use.
func NormalizeConfig(raw json.RawMessage) (api.DynamicConfig, error) {
	var in api.DynamicConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if len(raw) == 0 || dec.Decode(&in) != nil {
		return in, invalid("动态配置格式错误")
	}
	out := api.DynamicConfig{Type: in.Type}
	switch in.Type {
	case api.DynamicTypeDailyCalendar, api.DynamicTypeMonthCalendar, api.DynamicTypeWeather, api.DynamicTypeHistoryToday:
		if _, err := time.LoadLocation(in.TZ); in.TZ == "" || len(in.TZ) > 48 || err != nil {
			return out, invalid("时区无效: %s", in.TZ)
		}
		out.TZ = in.TZ
	}
	switch in.Type {
	case api.DynamicTypeDailyCalendar, api.DynamicTypeMonthCalendar, api.DynamicTypeEarthquakeReport:
	case api.DynamicTypeWeather:
		out.LocationID = strings.TrimSpace(in.LocationID)
		out.LocationLabel = strings.TrimSpace(in.LocationLabel)
		if out.LocationID == "" || len(out.LocationID) > 32 || out.LocationLabel == "" || len([]rune(out.LocationLabel)) > 32 {
			return out, invalid("请选择天气城市")
		}
	case api.DynamicTypeHistoryToday:
		out.Source = in.Source
		if out.Source == "" {
			out.Source = "wikipedia"
		}
		if out.Source != "wikipedia" && out.Source != "baidu_baike" {
			return out, invalid("历史数据源无效: %s", in.Source)
		}
	case api.DynamicTypeWeatherAlert:
		out.Province = NormalizeProvince(in.Province)
	case api.DynamicTypeDashboard:
		ref, err := normalizeTemplateRef(in.Template)
		if err != nil {
			return out, err
		}
		out.Template = ref
	case api.DynamicTypeFontTest:
		out.FontID = in.FontID
		if out.FontID == "" {
			out.FontID = "fusion_pixel_12"
		}
		if render.FontEntryByID(out.FontID) == nil {
			return out, invalid("字体无效: %s", in.FontID)
		}
		out.Invert = in.Invert
	case api.DynamicTypeHotList:
		out.Source = in.Source
		if out.Source == "" {
			out.Source = "weibo"
		}
		if hotlist.Info(out.Source) == nil {
			return out, invalid("热榜数据源无效: %s", in.Source)
		}
	default:
		return out, invalid("未知动态类型: %s", in.Type)
	}
	if b, ok := refreshBounds[in.Type]; ok {
		out.RefreshIntervalSec = in.RefreshIntervalSec
		if out.RefreshIntervalSec == 0 {
			out.RefreshIntervalSec = b[2]
		}
		if out.RefreshIntervalSec != 0 && (out.RefreshIntervalSec < b[0] || out.RefreshIntervalSec > b[1]) {
			return out, invalid("刷新间隔须为 %d-%d 秒", b[0], b[1])
		}
	}
	if audioTypes[in.Type] {
		out.AudioEnabled = in.AudioEnabled
		out.AudioVoice = in.AudioVoice
		if out.AudioVoice == "" {
			out.AudioVoice = api.DefaultTTSVoice
		}
		if !api.IsTTSVoice(out.AudioVoice) {
			return out, invalid("未知音色: %s", in.AudioVoice)
		}
	}
	return out, nil
}

// NormalizeProvince maps aliases like "广西" to a full province name, or "" for nationwide.
func NormalizeProvince(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "全国" || v == "全部" || v == "中国" {
		return ""
	}
	for _, p := range WeatherAlertProvinces {
		short := strings.TrimSuffix(strings.TrimSuffix(p, "省"), "市")
		for _, suffix := range []string{"壮族自治区", "回族自治区", "维吾尔自治区", "自治区"} {
			short = strings.TrimSuffix(short, suffix)
		}
		if v == p || v == short {
			return p
		}
	}
	return ""
}

func normalizeTemplateRef(ref *api.DashboardTemplateRef) (*api.DashboardTemplateRef, error) {
	if ref == nil {
		starter := render.StarterTemplate()
		return &api.DashboardTemplateRef{Kind: "custom", Template: &starter}, nil
	}
	switch ref.Kind {
	case "system":
		id := ref.ID
		if id == "" {
			id = "ai_usage_stats"
		}
		if render.SystemTemplate(id) == nil {
			return nil, invalid("系统模板不存在: %s", ref.ID)
		}
		return &api.DashboardTemplateRef{Kind: "system", ID: id}, nil
	case "custom":
		if ref.Template == nil {
			return nil, invalid("自定义模板不能为空")
		}
		t, err := normalizeTemplate(*ref.Template)
		if err != nil {
			return nil, err
		}
		return &api.DashboardTemplateRef{Kind: "custom", Template: &t}, nil
	}
	return nil, invalid("模板类型无效: %s", ref.Kind)
}

func normalizeTemplate(t api.DashboardTemplate) (api.DashboardTemplate, error) {
	if t.Version == 0 {
		t.Version = 1
	}
	if t.Version != 1 {
		return t, invalid("模板版本须为 1")
	}
	if len([]rune(t.Name)) > 48 {
		return t, invalid("模板名称最多 48 个字符")
	}
	if len(t.Blocks) == 0 || len(t.Blocks) > 32 {
		return t, invalid("模板须包含 1-32 个区块")
	}
	for i := range t.Blocks {
		if err := normalizeBlock(&t.Blocks[i]); err != nil {
			return t, invalid("区块 %d: %s", i+1, err.Error())
		}
	}
	return t, nil
}

func normalizeBlock(b *api.DashboardBlock) error {
	checkBinding := func(name, v string, required bool) error {
		if (required && v == "") || len([]rune(v)) > 160 {
			return fmt.Errorf("%s 须为 1-160 个字符", name)
		}
		return nil
	}
	checkSeries := func(name string, raw json.RawMessage, required bool) error {
		if len(raw) == 0 {
			if required {
				return fmt.Errorf("%s 不能为空", name)
			}
			return nil
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return checkBinding(name, s, true)
		}
		var nums []float64
		if json.Unmarshal(raw, &nums) != nil || len(nums) < 2 || len(nums) > 60 {
			return fmt.Errorf("%s 须为绑定表达式或 2-60 个数字", name)
		}
		return nil
	}
	oneOf := func(name, v, def string, allowed ...string) (string, error) {
		if v == "" {
			return def, nil
		}
		for _, a := range allowed {
			if v == a {
				return v, nil
			}
		}
		return "", fmt.Errorf("%s 无效", name)
	}
	fontSize := func(v, def int) (int, error) {
		if v == 0 {
			return def, nil
		}
		if v != 12 && v != 16 {
			return 0, fmt.Errorf("字号须为 12 或 16")
		}
		return v, nil
	}
	if b.Type == "line" {
		for _, v := range []int{b.X1, b.X2} {
			if v < 0 || v > 399 {
				return fmt.Errorf("x 坐标须为 0-399")
			}
		}
		for _, v := range []int{b.Y1, b.Y2} {
			if v < 24 || v > 299 {
				return fmt.Errorf("y 坐标须为 24-299")
			}
		}
		var err error
		b.Style, err = oneOf("style", b.Style, "solid", "solid", "dashed")
		*b = api.DashboardBlock{Type: b.Type, X1: b.X1, Y1: b.Y1, X2: b.X2, Y2: b.Y2, Style: b.Style}
		return err
	}
	switch {
	case b.X < 0 || b.X > 399, b.Y < 24 || b.Y > 299, b.W < 1 || b.W > 400, b.H < 1 || b.H > 276:
		return fmt.Errorf("区块位置超出范围")
	case b.X+b.W > 400:
		return fmt.Errorf("x + w 超出屏幕宽度 400")
	case b.Y+b.H > 300:
		return fmt.Errorf("y + h 超出屏幕高度 300")
	}
	rect := api.DashboardBlock{Type: b.Type, X: b.X, Y: b.Y, W: b.W, H: b.H}
	var err error
	switch b.Type {
	case "text":
		if err = checkBinding("value", b.Value, true); err != nil {
			return err
		}
		rect.Value = b.Value
		if rect.FontSize, err = fontSize(b.FontSize, 16); err != nil {
			return err
		}
		if rect.Align, err = oneOf("align", b.Align, "left", "left", "center", "right"); err != nil {
			return err
		}
		if rect.Color, err = oneOf("color", b.Color, "black", "black", "white"); err != nil {
			return err
		}
		rect.MaxLines = b.MaxLines
		if rect.MaxLines == 0 {
			rect.MaxLines = 1
		}
		if rect.MaxLines < 1 || rect.MaxLines > 4 {
			return fmt.Errorf("maxLines 须为 1-4")
		}
	case "metric":
		if err = checkBinding("label", b.Label, true); err != nil {
			return err
		}
		if err = checkBinding("value", b.Value, true); err != nil {
			return err
		}
		if err = checkSeries("sparkline", b.Sparkline, false); err != nil {
			return err
		}
		rect.Label, rect.Value, rect.Sparkline = b.Label, b.Value, b.Sparkline
	case "progress":
		if err = checkBinding("label", b.Label, true); err != nil {
			return err
		}
		for name, v := range map[string]string{"value": b.Value, "max": b.Max, "valueText": b.ValueText} {
			if err = checkBinding(name, v, false); err != nil {
				return err
			}
		}
		if len(b.Percentage) > 0 {
			var s string
			var n float64
			if json.Unmarshal(b.Percentage, &s) != nil && (json.Unmarshal(b.Percentage, &n) != nil || n < 0 || n > 100) {
				return fmt.Errorf("percentage 须为绑定表达式或 0-100")
			}
		}
		rect.Label, rect.Value, rect.Max, rect.ValueText, rect.Percentage = b.Label, b.Value, b.Max, b.ValueText, b.Percentage
		if rect.LabelFontSize, err = fontSize(b.LabelFontSize, 12); err != nil {
			return err
		}
		if rect.ValueFontSize, err = fontSize(b.ValueFontSize, 12); err != nil {
			return err
		}
		rect.BarHeight = b.BarHeight
		if rect.BarHeight == 0 {
			rect.BarHeight = 9
		}
		if rect.BarHeight < 4 || rect.BarHeight > 24 {
			return fmt.Errorf("barHeight 须为 4-24")
		}
	case "sparkline":
		if err = checkSeries("values", b.Values, true); err != nil {
			return err
		}
		rect.Values = b.Values
	case "rect":
		stroke := b.Stroke == nil || *b.Stroke
		rect.Stroke = &stroke
		if rect.Fill, err = oneOf("fill", b.Fill, "none", "none", "black", "white"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知区块类型 %s", b.Type)
	}
	*b = rect
	return nil
}

// ValidateDashboardData requires a JSON object with 1+ fields and keys of at most 64 chars.
func ValidateDashboardData(raw json.RawMessage) error {
	var data map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &data) != nil || len(data) == 0 {
		return invalid("dashboard 数据须为至少包含一个字段的 JSON 对象")
	}
	for k := range data {
		if len([]rune(k)) > 64 {
			return invalid("dashboard 数据字段名最多 64 个字符")
		}
	}
	return nil
}

func DefaultFrameName(cfg api.DynamicConfig) *string {
	name := map[api.DynamicType]string{
		api.DynamicTypeDailyCalendar: "日历", api.DynamicTypeMonthCalendar: "月历", api.DynamicTypeHistoryToday: "历史上的今天",
		api.DynamicTypeEarthquakeReport: "地震速报",
	}[cfg.Type]
	switch cfg.Type {
	case api.DynamicTypeWeatherAlert, api.DynamicTypeWeather, api.DynamicTypeDashboard, api.DynamicTypeFontTest, api.DynamicTypeHotList:
		name = configLabel(cfg)
	}
	if name == "" {
		return nil
	}
	return &name
}

// configLabel is the status bar text that depends only on configuration.
func configLabel(cfg api.DynamicConfig) string {
	switch cfg.Type {
	case api.DynamicTypeWeather:
		if cfg.LocationLabel == "" || cfg.LocationLabel == "天气" {
			return "天气"
		}
		return cfg.LocationLabel + "天气"
	case api.DynamicTypeWeatherAlert:
		return or(NormalizeProvince(cfg.Province), "全国") + "气象预警"
	case api.DynamicTypeEarthquakeReport:
		return "地震速报"
	case api.DynamicTypeDashboard:
		if cfg.Template != nil && cfg.Template.Kind == "system" {
			if t := render.SystemTemplate(cfg.Template.ID); t != nil {
				return t.Label
			}
			return "外部数据"
		}
		if cfg.Template != nil && cfg.Template.Template != nil {
			return or(cfg.Template.Template.Name, "自定义模板")
		}
		return "外部数据"
	case api.DynamicTypeFontTest:
		if e := render.FontEntryByID(cfg.FontID); e != nil {
			return e.Label
		}
		return "字体测试"
	case api.DynamicTypeHotList:
		if info := hotlist.Info(cfg.Source); info != nil {
			return info.ShortLabel + "热榜"
		}
		return "热榜"
	}
	return ""
}

// StatusBarText is the label the firmware draws above a frame. Date-dependent labels use
// the render time, so they stay stable between renders.
func StatusBarText(frameName, dynamicType *string, config, data json.RawMessage, renderedAt *time.Time) string {
	name := ""
	if frameName != nil {
		name = *frameName
	}
	if dynamicType == nil {
		return name
	}
	var cfg api.DynamicConfig
	_ = json.Unmarshal(config, &cfg)
	switch api.DynamicType(*dynamicType) {
	case api.DynamicTypeDailyCalendar, api.DynamicTypeMonthCalendar, api.DynamicTypeHistoryToday:
		if renderedAt == nil {
			return name
		}
		t := renderedAt.In(render.Location(cfg.TZ))
		switch api.DynamicType(*dynamicType) {
		case api.DynamicTypeDailyCalendar:
			return fmt.Sprintf("%d月%d日", int(t.Month()), t.Day())
		case api.DynamicTypeMonthCalendar:
			return fmt.Sprintf("%d年%d月", t.Year(), int(t.Month()))
		}
		var h render.HistoryTodayData
		_ = json.Unmarshal(data, &h)
		label := strings.Join(strings.Fields(h.DateLabel), "")
		if label == "" {
			label = fmt.Sprintf("%d月%d日", int(t.Month()), t.Day())
		}
		return "历史上的" + label
	case api.DynamicTypeDashboard:
		if frameName != nil {
			return name
		}
		return configLabel(cfg)
	case api.DynamicTypeWeather, api.DynamicTypeWeatherAlert, api.DynamicTypeEarthquakeReport, api.DynamicTypeFontTest, api.DynamicTypeHotList:
		return configLabel(cfg)
	}
	return name
}

func Catalog() api.Catalog {
	fonts := make([]api.FontInfo, 0, len(render.FontCatalog()))
	for _, f := range render.FontCatalog() {
		fonts = append(fonts, f.FontInfo)
	}
	return api.Catalog{
		TTSVoices:             api.TTSVoices,
		DefaultTTSVoice:       api.DefaultTTSVoice,
		HotListSources:        hotlist.Catalog(),
		Fonts:                 fonts,
		DashboardTemplates:    render.SystemTemplates(),
		DashboardStarter:      render.StarterTemplate(),
		WeatherAlertProvinces: WeatherAlertProvinces,
	}
}

func or(v, fallback string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return fallback
}
