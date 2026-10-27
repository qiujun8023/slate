package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"slate/internal/api"
	"slate/internal/fetch"
	"slate/internal/hotlist"
	"slate/internal/render"
)

// fetchData returns the data a frame renders; lastData is the previous successful value.
func (e *Engine) fetchData(ctx context.Context, cfg api.DynamicConfig, lastData json.RawMessage, now time.Time) (any, error) {
	local := now.In(render.Location(cfg.TZ))
	ttl := time.Duration(max(cfg.RefreshIntervalSec, 300)) * time.Second
	switch cfg.Type {
	case api.DynamicTypeDailyCalendar:
		return dailyCalendar(local), nil
	case api.DynamicTypeMonthCalendar:
		return monthCalendar(local), nil
	case api.DynamicTypeFontTest:
		return nil, nil
	case api.DynamicTypeDashboard:
		if ValidateDashboardData(lastData) != nil {
			return nil, errors.New("dashboard 数据为空，请先提供初始数据或推送数据")
		}
		return lastData, nil
	case api.DynamicTypeWeather:
		return e.cache.get(ctx, "weather:"+cfg.LocationID+":"+cfg.TZ, ttl, now, func(ctx context.Context) (any, error) {
			return e.weather(ctx, cfg, now)
		})
	case api.DynamicTypeWeatherAlert:
		return e.cache.get(ctx, "alert:"+cfg.Province, ttl, now, func(ctx context.Context) (any, error) {
			return weatherAlerts(ctx, cfg.Province, now)
		})
	case api.DynamicTypeEarthquakeReport:
		return e.cache.get(ctx, "earthquake", ttl, now, func(ctx context.Context) (any, error) {
			return earthquakes(ctx, now)
		})
	case api.DynamicTypeHistoryToday:
		key := fmt.Sprintf("history:%s:%02d-%02d:%s", cfg.Source, int(local.Month()), local.Day(), e.ai.cacheKey())
		return e.cache.get(ctx, key, 24*time.Hour, now, func(ctx context.Context) (any, error) {
			if cfg.Source == "baidu_baike" {
				return baiduHistory(ctx, local)
			}
			return e.wikipediaHistory(ctx, local)
		})
	case api.DynamicTypeHotList:
		v, err := e.cache.get(ctx, "hot:"+cfg.Source, ttl, now, func(ctx context.Context) (any, error) {
			items, err := hotlist.Fetch(ctx, cfg.Source)
			if err == nil && len(items) == 0 {
				err = errors.New("热榜数据为空")
			}
			return items, err
		})
		if err != nil {
			return nil, err
		}
		info := hotlist.Info(cfg.Source)
		return render.HotListData{Source: cfg.Source, SourceLabel: info.Label, UpdatedAt: now.UTC(), Items: v.([]render.HotListItem)}, nil
	}
	return nil, fmt.Errorf("未知动态类型: %s", cfg.Type)
}

type qweatherNow struct {
	Code       string `json:"code"`
	UpdateTime string `json:"updateTime"`
	Now        struct {
		ObsTime   string `json:"obsTime"`
		Temp      string `json:"temp"`
		FeelsLike string `json:"feelsLike"`
		Text      string `json:"text"`
		Icon      string `json:"icon"`
		WindDir   string `json:"windDir"`
		WindScale string `json:"windScale"`
		WindSpeed string `json:"windSpeed"`
		Humidity  string `json:"humidity"`
		Pressure  string `json:"pressure"`
	} `json:"now"`
}

type qweatherDaily struct {
	Code  string `json:"code"`
	Daily []struct {
		FxDate    string `json:"fxDate"`
		TempMax   string `json:"tempMax"`
		TempMin   string `json:"tempMin"`
		TextDay   string `json:"textDay"`
		TextNight string `json:"textNight"`
		IconDay   string `json:"iconDay"`
	} `json:"daily"`
}

type qweatherLookup struct {
	Code     string `json:"code"`
	Location []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Adm1 string `json:"adm1"`
		Adm2 string `json:"adm2"`
	} `json:"location"`
}

func (e *Engine) qweather(ctx context.Context, path string, query url.Values, v any) error {
	if e.cfg.QWeatherAPIKey == "" {
		return errors.New("QWEATHER_API_KEY 未配置")
	}
	if e.cfg.QWeatherAPIHost == "" {
		return errors.New("QWEATHER_API_HOST 未配置，请在和风天气控制台-设置中复制你的 API Host")
	}
	query.Set("lang", "zh")
	return fetch.JSON(ctx, e.cfg.QWeatherAPIHost+path+"?"+query.Encode(),
		fetch.Options{Headers: map[string]string{"X-QW-Api-Key": e.cfg.QWeatherAPIKey}, NoUserAgent: true}, v)
}

var digitsOnly = regexp.MustCompile(`^\d+$`)

func (e *Engine) weather(ctx context.Context, cfg api.DynamicConfig, now time.Time) (render.WeatherData, error) {
	location := cfg.LocationID
	if !digitsOnly.MatchString(location) {
		cities, err := e.SearchCities(ctx, location)
		if err != nil {
			return render.WeatherData{}, err
		}
		if len(cities) == 0 {
			return render.WeatherData{}, fmt.Errorf("和风天气找不到城市 %s", location)
		}
		location = cities[0].ID
	}
	var current qweatherNow
	var daily qweatherDaily
	q := url.Values{"location": {location}, "unit": {"m"}}
	errs := make(chan error, 2)
	go func() { errs <- e.qweather(ctx, "/v7/weather/now", q, &current) }()
	go func() {
		errs <- e.qweather(ctx, "/v7/weather/3d", url.Values{"location": {location}, "unit": {"m"}}, &daily)
	}()
	for range 2 {
		if err := <-errs; err != nil {
			return render.WeatherData{}, err
		}
	}
	if current.Code != "200" {
		return render.WeatherData{}, fmt.Errorf("QWeather now code %s", current.Code)
	}
	if daily.Code != "200" {
		return render.WeatherData{}, fmt.Errorf("QWeather forecast code %s", daily.Code)
	}
	n := current.Now
	wind := "--"
	switch {
	case n.WindDir != "" && n.WindScale != "":
		wind = n.WindDir + n.WindScale + "级"
	case n.WindDir != "":
		wind = n.WindDir
	case displayNumber(n.WindSpeed) != "--":
		wind = displayNumber(n.WindSpeed) + "km/h"
	}
	out := render.WeatherData{
		TempC: displayNumber(n.Temp), FeelsLikeC: displayNumber(n.FeelsLike), Humidity: displayNumber(n.Humidity),
		Pressure: displayNumber(n.Pressure), WindDisplay: wind, Summary: or(n.Text, "--"), Code: iconCode(n.Icon),
		ObsTime: or(n.ObsTime, current.UpdateTime), UpdatedAt: now.UTC(),
	}
	labels := []string{"今日", "明日", "后天"}
	local := now.In(render.Location(cfg.TZ))
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	for i, d := range firstN(daily.Daily, 3) {
		text := or(d.TextDay, or(d.TextNight, "--"))
		if d.TextNight != "" && d.TextNight != text {
			text += "/" + d.TextNight
		}
		label := labels[i]
		if day, err := time.Parse("2006-01-02", d.FxDate); err == nil {
			delta := int(day.Sub(today).Hours() / 24)
			if delta >= 0 && delta < 3 {
				label = labels[delta]
			} else {
				label = fmt.Sprintf("%d/%d", int(day.Month()), day.Day())
			}
		}
		out.Forecast = append(out.Forecast, render.WeatherDay{Label: label, Text: text, TempMin: displayNumber(d.TempMin), TempMax: displayNumber(d.TempMax), Code: iconCode(d.IconDay)})
	}
	if out.Code == 999 && len(out.Forecast) > 0 {
		out.Code = out.Forecast[0].Code
	}
	return out, nil
}

func displayNumber(v string) string {
	n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return or(v, "--")
	}
	return strconv.Itoa(int(math.Floor(n + 0.5)))
}

func iconCode(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 999
	}
	return n
}

func (e *Engine) SearchCities(ctx context.Context, query string) ([]api.WeatherCity, error) {
	v, err := e.cache.get(ctx, "city:"+query, time.Hour, time.Now(), func(ctx context.Context) (any, error) {
		var res qweatherLookup
		if err := e.qweather(ctx, "/geo/v2/city/lookup", url.Values{"location": {query}, "range": {"cn"}, "number": {"8"}}, &res); err != nil {
			return nil, err
		}
		if res.Code != "200" && res.Code != "404" {
			return nil, fmt.Errorf("QWeather city lookup code %s", res.Code)
		}
		out := []api.WeatherCity{}
		for _, l := range res.Location {
			if c := (api.WeatherCity{ID: strings.TrimSpace(l.ID), Name: strings.TrimSpace(l.Name), Adm1: strings.TrimSpace(l.Adm1), Adm2: strings.TrimSpace(l.Adm2)}); c.ID != "" && c.Name != "" {
				out = append(out, c)
			}
		}
		return out, nil
	})
	if err != nil {
		return nil, &ValidationError{"城市搜索失败: " + err.Error()}
	}
	return v.([]api.WeatherCity), nil
}

func weatherAlerts(ctx context.Context, province string, now time.Time) (render.WeatherAlertData, error) {
	var res struct {
		Data struct {
			Page struct {
				List []struct {
					AlertID   string `json:"alertid"`
					Title     string `json:"title"`
					IssueTime string `json:"issuetime"`
					URL       string `json:"url"`
				} `json:"list"`
			} `json:"page"`
		} `json:"data"`
	}
	q := url.Values{"pageNo": {"1"}, "pageSize": {"20"}, "signaltype": {""}, "signallevel": {""}, "province": {province}}
	err := fetch.JSON(ctx, "https://www.nmc.cn/rest/findAlarm?"+q.Encode(), fetch.Options{Headers: map[string]string{"Referer": "https://www.nmc.cn/publish/alarm.html"}}, &res)
	if err != nil {
		return render.WeatherAlertData{}, err
	}
	out := render.WeatherAlertData{Province: province, UpdatedAt: now.UTC(), Items: []render.WeatherAlertItem{}}
	cst := render.Location("Asia/Shanghai")
	for i, row := range res.Data.Page.List {
		title := fetch.StripHTML(row.Title)
		if title == "" {
			continue
		}
		item := render.WeatherAlertItem{ID: or(fetch.StripHTML(row.AlertID), fmt.Sprintf("%s:%d", title, i)), Title: title}
		for _, layout := range []string{"2006/01/02 15:04", "2006/01/02 15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
			if t, err := time.ParseInLocation(layout, strings.TrimSpace(row.IssueTime), cst); err == nil {
				ut := t.UTC()
				item.IssuedAt = &ut
				break
			}
		}
		if u, err := url.Parse(row.URL); err == nil && row.URL != "" {
			if abs := (&url.URL{Scheme: "https", Host: "www.nmc.cn"}).ResolveReference(u); abs.Host == "www.nmc.cn" {
				item.URL = abs.String()
			}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

var (
	quakeRow  = regexp.MustCompile(`(?s)<tr id="earthquake_subao_guid_catalog_tr_\d+".*?</tr>`)
	quakeCell = regexp.MustCompile(`(?s)<div[^>]*class=['"]cls-data-content-list['"][^>]*>(.*?)</div>`)
)

func earthquakes(ctx context.Context, now time.Time) (render.EarthquakeData, error) {
	html, err := fetch.Text(ctx, "https://data.earthquake.cn/datashare/report.shtml?PAGEID=earthquake_subao", fetch.Options{Timeout: 15 * time.Second})
	if err != nil {
		return render.EarthquakeData{}, err
	}
	items := ParseEarthquakes(html)
	if len(items) == 0 && !strings.Contains(html, "earthquake_subao") && !strings.Contains(html, "cls-data-content-list") {
		return render.EarthquakeData{}, errors.New("地震速报页面结构已变化，无法解析列表")
	}
	return render.EarthquakeData{UpdatedAt: now.UTC(), Items: firstN(items, 20)}, nil
}

func ParseEarthquakes(html string) []render.EarthquakeItem {
	items := []render.EarthquakeItem{}
	for i, row := range quakeRow.FindAllString(html, -1) {
		var cells []string
		for _, m := range quakeCell.FindAllStringSubmatch(row, -1) {
			cells = append(cells, fetch.StripHTML(m[1]))
		}
		if len(cells) < 8 || cells[1] == "" || cells[6] == "" || cells[5] == "" {
			continue
		}
		items = append(items, render.EarthquakeItem{
			ID: or(cells[0], fmt.Sprintf("%s:%s:%s:%d", cells[1], cells[2], cells[3], i)), OccurredAt: cells[1], Longitude: cells[2],
			Latitude: cells[3], DepthKm: cells[4], Magnitude: cells[5], Location: cells[6], EventType: cells[7],
		})
	}
	return items
}

var (
	historyYearPrefix  = regexp.MustCompile(`^[\d前公元\s]+年?\s*[·.。:：、-]\s*`)
	historyTrailing    = regexp.MustCompile(`[。；;]+$`)
	historyYearPattern = regexp.MustCompile(`^(前)?(\d{1,4})$`)
)

// historyYear normalizes "公元前221年" or "-221" to "前221" and "1904年" to "1904".
func historyYear(v string) string {
	t := strings.Join(strings.Fields(v), "")
	t = strings.TrimSuffix(strings.Replace(strings.Replace(t, "公元前", "前", 1), "公元", "", 1), "年")
	if strings.HasPrefix(t, "-") {
		t = "前" + t[1:]
	}
	m := historyYearPattern.FindStringSubmatch(t)
	if m == nil {
		return ""
	}
	n, _ := strconv.Atoi(m[2])
	if n == 0 {
		return ""
	}
	return m[1] + strconv.Itoa(n)
}

func historyDisplay(v string) string {
	v = historyYearPrefix.ReplaceAllString(strings.TrimSpace(v), "")
	return strings.Join(strings.Fields(historyTrailing.ReplaceAllString(v, "")), " ")
}

func historyData(label string, years, displays []string) (render.HistoryTodayData, error) {
	out := render.HistoryTodayData{DateLabel: label}
	for i := range years {
		y, d := historyYear(years[i]), historyDisplay(displays[i])
		if y != "" && d != "" && len([]rune(d)) <= 120 {
			out.Items = append(out.Items, render.HistoryTodayItem{Year: y, Display: d})
		}
	}
	if len(out.Items) == 0 {
		return out, errors.New("历史上的今天数据为空")
	}
	out.Items = render.SortHistory(out.Items)
	return out, nil
}

func baiduHistory(ctx context.Context, local time.Time) (render.HistoryTodayData, error) {
	month := fmt.Sprintf("%02d", int(local.Month()))
	var res map[string]map[string][]struct {
		Title string `json:"title"`
		Desc  string `json:"desc"`
		Year  string `json:"year"`
	}
	u := fmt.Sprintf("https://baike.baidu.com/cms/home/eventsOnHistory/%s.json?_=%d", month, time.Now().UnixMilli())
	if err := fetch.JSON(ctx, u, fetch.Options{}, &res); err != nil {
		return render.HistoryTodayData{}, err
	}
	var years, displays []string
	for _, item := range res[month][fmt.Sprintf("%s%02d", month, local.Day())] {
		years = append(years, item.Year)
		displays = append(displays, fetch.StripHTML(or(item.Title, item.Desc)))
	}
	return historyData(monthDayCN(local), years, displays)
}

// wikipediaHistory asks the AI model to pick and rewrite events; raw Wikipedia text is
// never rendered because it mixes scripts and runs far too long for the screen.
func (e *Engine) wikipediaHistory(ctx context.Context, local time.Time) (render.HistoryTodayData, error) {
	var res struct {
		Events []struct {
			Year  *int   `json:"year"`
			Text  string `json:"text"`
			Pages []struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Extract     string `json:"extract"`
			} `json:"pages"`
		} `json:"events"`
	}
	u := fmt.Sprintf("https://zh.wikipedia.org/api/rest_v1/feed/onthisday/events/%d/%d?variant=zh-cn", int(local.Month()), local.Day())
	if err := fetch.JSON(ctx, u, fetch.Options{Headers: map[string]string{"User-Agent": "Slate/1.0 (+https://github.com/qiujun8023/slate; history-today-provider)"}}, &res); err != nil {
		return render.HistoryTodayData{}, err
	}
	var events []historyEvent
	for _, ev := range res.Events {
		if ev.Year == nil || ev.Text == "" {
			continue
		}
		label := strconv.Itoa(*ev.Year)
		if *ev.Year < 0 {
			label = "前" + strconv.Itoa(-*ev.Year)
		}
		item := historyEvent{Year: *ev.Year, YearLabel: label, Text: ev.Text}
		for _, p := range firstN(ev.Pages, 3) {
			if p.Title != "" || p.Description != "" || p.Extract != "" {
				item.Pages = append(item.Pages, historyPage(p))
			}
		}
		events = append(events, item)
	}
	label := monthDayCN(local)
	picked, err := e.ai.optimizeHistory(ctx, label, firstN(events, 100))
	if err != nil {
		return render.HistoryTodayData{}, err
	}
	years := make([]string, len(picked.Items))
	displays := make([]string, len(picked.Items))
	for i, it := range picked.Items {
		years[i], displays[i] = it.Year, it.Display
	}
	return historyData(or(picked.DateLabel, label), years, displays)
}
