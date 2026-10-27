package render

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"

	"slate/internal/api"
)

func (d drawer) dashboard(cfg api.DynamicConfig, data map[string]any) {
	f := d.fonts
	tpl := TemplateFor(cfg.Template)
	if tpl == nil || data == nil {
		centerY := round(float64(safeTop+safeBottom) / 2)
		d.text(f.Sans16, "等待外部数据", frameW/2, centerY-20, textOpts{align: alignCenter, maxWidth: contentWidth})
		d.text(f.Sans12, "POST /api/contents/:id/data", frameW/2, centerY+12, textOpts{align: alignCenter, maxWidth: contentWidth})
		return
	}
	for _, b := range tpl.Blocks {
		switch b.Type {
		case "text":
			x, y, w, _, ok := blockRect(b)
			if !ok {
				continue
			}
			font, color := f.Sans16, Black
			if b.FontSize == 12 {
				font = f.Sans12
			}
			if b.Color == "white" {
				color = White
			}
			a, anchor := alignLeft, x
			switch b.Align {
			case "center":
				a, anchor = alignCenter, x+round(float64(w)/2)
			case "right":
				a, anchor = alignRight, x+w
			}
			d.text(font, ResolveTemplate(b.Value, data), anchor, y, textOpts{align: a, maxWidth: w, maxLines: min(4, max(1, b.MaxLines)), ellipsis: true, color: color})
		case "metric":
			x, y, w, h, ok := blockRect(b)
			if !ok {
				continue
			}
			d.c.StrokeRect(x, y, w, h, Black)
			d.text(f.Sans12, ResolveTemplate(b.Label, data), x+7, y+4, textOpts{maxWidth: w - 14, ellipsis: true})
			d.text(f.Sans16, ResolveTemplate(b.Value, data), x+7, y+27, textOpts{maxWidth: w - 14, ellipsis: true})
			if series := resolveSeries(b.Sparkline, data); len(series) >= 2 && h >= 54 {
				d.sparkline(x+7, y+h-18, w-14, 10, series)
			}
		case "progress":
			x, y, w, h, ok := blockRect(b)
			if !ok {
				continue
			}
			d.progress(b, data, x, y, w, h)
		case "sparkline":
			x, y, w, h, ok := blockRect(b)
			if !ok {
				continue
			}
			if series := resolveSeries(b.Values, data); len(series) >= 2 {
				d.sparkline(x, y, w, h, series)
			}
		case "line":
			x1, x2 := min(frameW-1, max(0, b.X1)), min(frameW-1, max(0, b.X2))
			y1, y2 := min(frameH-1, max(statusBarH, b.Y1)), min(frameH-1, max(statusBarH, b.Y2))
			if b.Style == "dashed" && y1 == y2 {
				d.rule(min(x1, x2), y1, abs(x2-x1), true, Black)
			} else {
				d.c.Line(x1, y1, x2, y2, Black)
			}
		case "rect":
			x, y, w, h, ok := blockRect(b)
			if !ok {
				continue
			}
			switch b.Fill {
			case "black":
				d.c.FillRect(x, y, w, h, Black)
			case "white":
				d.c.FillRect(x, y, w, h, White)
			}
			if b.Stroke == nil || *b.Stroke {
				d.c.StrokeRect(x, y, w, h, Black)
			}
		}
	}
}

func (d drawer) progress(b api.DashboardBlock, data map[string]any, x, y, w, h int) {
	f := d.fonts
	label := ResolveTemplate(b.Label, data)
	valueText := ResolveTemplate(b.ValueText, data)
	pct := resolvePercentage(b.Percentage, b.Value, b.Max, data)
	labelFont, valueFont := f.Sans12, f.Sans12
	if b.LabelFontSize == 16 {
		labelFont = f.Sans16
	}
	if b.ValueFontSize == 16 {
		valueFont = f.Sans16
	}
	labelW := min(max(d.width(labelFont, label)+8, 58), round(float64(w)*0.36))
	barX := x + labelW
	valueW := 0
	if valueText != "" {
		valueW = min(max(d.width(valueFont, valueText)+4, 34), 78)
	}
	barW := max(12, w-labelW-valueW-10)
	barHeight := b.BarHeight
	if barHeight == 0 {
		barHeight = 9
	}
	barH := max(4, min(barHeight, h-4))
	centerY := y + round(float64(h)/2)
	barY := centerY - barH/2
	d.textCenteredY(labelFont, label, x, float64(centerY), textOpts{maxWidth: labelW - 6, ellipsis: true})
	d.c.StrokeRect(barX, barY, barW, barH, Black)
	if fillW := max(0, min(barW-2, round(float64(barW-2)*pct/100))); fillW > 0 {
		d.c.FillRect(barX+1, barY+1, fillW, barH-2, Black)
	}
	if valueText != "" {
		d.textCenteredY(valueFont, valueText, x+w, float64(centerY), textOpts{align: alignRight, maxWidth: valueW, ellipsis: true})
	}
}

func blockRect(b api.DashboardBlock) (x, y, w, h int, ok bool) {
	if b.X < 0 || b.X >= frameW || b.Y < statusBarH || b.Y >= frameH || b.W < 1 || b.H < 1 {
		return 0, 0, 0, 0, false
	}
	return b.X, b.Y, min(b.W, frameW-b.X), min(b.H, frameH-b.Y), true
}

var binding = regexp.MustCompile(`\{([a-zA-Z0-9_.-]+)(?:\|([a-zA-Z0-9_]+))?\}`)

// ResolveTemplate replaces {path|format} bindings with values from data.
func ResolveTemplate(tpl string, data map[string]any) string {
	return binding.ReplaceAllStringFunc(tpl, func(m string) string {
		parts := binding.FindStringSubmatch(m)
		v := resolvePath(data, parts[1])
		if arr, ok := v.([]any); ok {
			items := make([]string, len(arr))
			for i, item := range arr {
				if item != nil {
					items[i] = jsString(item)
				}
			}
			return strings.Join(items, " ")
		}
		if v == nil {
			return ""
		}
		return formatValue(v, parts[2])
	})
}

func resolvePath(data any, path string) any {
	parts := strings.Split(path, ".")
	cur := data
	for i, part := range parts {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[part]
		case []any:
			if part == "*" {
				rest := strings.Join(parts[i+1:], ".")
				if rest == "" {
					return v
				}
				out := make([]any, len(v))
				for j, item := range v {
					out[j] = resolvePath(item, rest)
				}
				return out
			}
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil
			}
			cur = v[idx]
		default:
			return nil
		}
	}
	return cur
}

func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	case string:
		if strings.TrimSpace(n) == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return 0, false
}

// jsString formats like JavaScript String(value) for JSON values.
func jsString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return "null"
	case []any:
		items := make([]string, len(t))
		for i, item := range t {
			if item != nil {
				items[i] = jsString(item)
			}
		}
		return strings.Join(items, ",")
	}
	return "[object Object]"
}

func formatValue(v any, format string) string {
	n, ok := toNumber(v)
	if format == "" || !ok {
		return jsString(v)
	}
	switch format {
	case "int":
		return thousands(int64(math.Trunc(n)))
	case "tokens", "compact":
		return compact(n)
	case "usd", "usd2":
		return "$" + strconv.FormatFloat(n, 'f', 2, 64)
	case "usd4":
		return "$" + strconv.FormatFloat(n, 'f', 4, 64)
	case "usd_per_million":
		return "$" + trimFixed(n, 2) + "/M"
	case "percent":
		return trimFixed(n, 1) + "%"
	case "duration":
		if n >= 1000 {
			return trimFixed(n/1000, 2) + "s"
		}
		return strconv.Itoa(round(n)) + "ms"
	}
	return jsString(v)
}

func compact(v float64) string {
	a := math.Abs(v)
	switch {
	case a >= 1e9:
		return trimFixed(v/1e9, 1) + "B"
	case a >= 1e6:
		return trimFixed(v/1e6, 1) + "M"
	case a >= 1e3:
		return trimFixed(v/1e3, 1) + "K"
	}
	return strconv.FormatInt(int64(math.Trunc(v)), 10)
}

func trimFixed(v float64, digits int) string {
	s := strconv.FormatFloat(v, 'f', digits, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}

func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

var wholeBinding = regexp.MustCompile(`^\{([a-zA-Z0-9_.*-]+)\}$`)

func resolveSeries(raw json.RawMessage, data map[string]any) []float64 {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		return numbers(t)
	case string:
		if m := wholeBinding.FindStringSubmatch(t); m != nil {
			arr, _ := resolvePath(data, m[1]).([]any)
			return numbers(arr)
		}
		var parts []any
		for _, p := range strings.Split(t, ",") {
			if p = strings.TrimSpace(p); p != "" {
				parts = append(parts, p)
			}
		}
		return numbers(parts)
	}
	return nil
}

func numbers(values []any) []float64 {
	var out []float64
	for _, v := range values {
		if n, ok := toNumber(v); ok {
			out = append(out, n)
		}
	}
	return out
}

func resolvePercentage(raw json.RawMessage, used, maxValue string, data map[string]any) float64 {
	var v any
	if len(raw) > 0 && json.Unmarshal(raw, &v) == nil {
		switch t := v.(type) {
		case float64:
			return math.Max(0, math.Min(100, t))
		case string:
			if n, ok := toNumber(strings.TrimSuffix(ResolveTemplate(t, data), "%")); ok {
				return math.Max(0, math.Min(100, n))
			}
		}
	}
	u, ok1 := toNumber(ResolveTemplate(used, data))
	m, ok2 := toNumber(ResolveTemplate(maxValue, data))
	if !ok1 || !ok2 || m <= 0 {
		return 0
	}
	return math.Max(0, math.Min(100, u/m*100))
}
