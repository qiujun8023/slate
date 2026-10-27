package render

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

func (d drawer) hotList(data HotListData) {
	f := d.fonts
	items := firstN(data.Items, 8)
	if len(items) == 0 {
		d.text(f.Sans16, "暂无榜单数据", frameW/2, 136, textOpts{align: alignCenter, maxWidth: contentWidth})
		return
	}
	const rowH, rankW, rankH = 33, 28, 18
	listTop := statusBarH + 10
	titleX := contentLeft + 42
	for i, item := range items {
		y := listTop + i*rowH
		centerY := float64(y) + float64(rowH-2)/2
		rankY := round(centerY - rankH/2.0)
		rank := item.Rank
		if rank == 0 {
			rank = i + 1
		}
		rank = min(99, max(1, rank))
		color := Black
		if i < 3 {
			d.c.FillRect(contentLeft, rankY, rankW, rankH, Black)
			color = White
		} else {
			d.c.StrokeRect(contentLeft, rankY, rankW, rankH, Black)
		}
		d.textInBox(f.Metric12, fmt.Sprintf("%02d", rank), contentLeft, rankY, rankW, rankH, color)
		d.textCenteredY(f.Sans16, or(item.Title, fallbackText), titleX, centerY, textOpts{maxWidth: contentRight - titleX, ellipsis: true})
		if i < len(items)-1 {
			d.rule(contentLeft, y+rowH-1, contentWidth, true, Black)
		}
	}
}

func (d drawer) earthquake(data EarthquakeData, now time.Time) {
	f := d.fonts
	items := firstN(data.Items, 4)
	updated := ""
	if !data.UpdatedAt.IsZero() {
		updated = data.UpdatedAt.In(now.Location()).Format("15:04")
	}
	if len(items) == 0 {
		if updated != "" {
			d.text(f.Sans12, "更新 "+updated, contentRight, statusBarH+12, textOpts{align: alignRight, maxWidth: 116, ellipsis: true})
		}
		d.text(f.Sans16, "暂无地震速报", frameW/2, 146, textOpts{align: alignCenter, maxWidth: contentWidth})
		return
	}
	latest := items[0]
	heroY := statusBarH + 10
	const heroH, magW = 106, 120
	fields := quakeFields(latest)
	d.c.FillRect(contentLeft, heroY, magW, heroH, Black)
	d.text(f.Sans12, "震级", contentLeft+8, heroY+8, textOpts{maxWidth: magW - 16, ellipsis: true, color: White})
	d.magnitude(or(latest.Magnitude, "--"), contentLeft+4, heroY+22, magW-8, 58)
	footer := "最新速报"
	if updated != "" {
		footer = "更新 " + updated
	}
	d.text(f.Sans12, footer, contentLeft+8, heroY+82, textOpts{maxWidth: magW - 16, ellipsis: true, color: White})

	textX := contentLeft + magW + 14
	textW := contentRight - textX
	locH := d.strongText(f.Sans16, or(latest.Location, fallbackText), textX, heroY+4, textOpts{maxWidth: textW, maxLines: 2, ellipsis: true, lineGap: 2})
	fieldY := heroY + max(34, locH+8)
	d.fieldPair("时间", or(fields.time, "--"), textX, fieldY, textW)
	d.fieldPair("深度", fields.depth, textX, fieldY+21, textW)
	d.fieldPair("坐标", or(fields.coords, "--"), textX, fieldY+42, textW)

	rest := items[1:]
	if len(rest) == 0 {
		return
	}
	startY := heroY + heroH + 12
	rowH := min(46, (frameH-startY-8)/len(rest))
	for i, item := range rest {
		y := startY + i*rowH
		fields := quakeFields(item)
		d.badge("M"+or(item.Magnitude, "--"), contentLeft, y+11, 52, 24, i == 0)
		d.text(f.Sans16, or(item.Location, fallbackText), contentLeft+66, y+5, textOpts{maxWidth: contentRight - contentLeft - 66, ellipsis: true})
		if fields.time != "" {
			d.text(f.Metric12, fields.time, contentLeft+66, y+26, textOpts{maxWidth: 62, ellipsis: true})
		}
		d.text(f.Metric12, fields.depth, contentLeft+138, y+26, textOpts{maxWidth: contentRight - contentLeft - 138, ellipsis: true})
		if i < len(rest)-1 {
			d.rule(contentLeft+66, y+rowH-1, contentRight-contentLeft-66, true, Black)
		}
	}
}

func (d drawer) fieldPair(label, value string, x, y, w int) {
	const labelW = 38
	d.text(d.fonts.Sans16, label, x, y, textOpts{maxWidth: labelW, ellipsis: true})
	d.text(d.fonts.Sans16, value, x+labelW+8, y, textOpts{maxWidth: w - labelW - 8, ellipsis: true})
}

var compactMagnitude = regexp.MustCompile(`^(\d+)\.(\d)$`)

func (d drawer) magnitude(value string, x, y, w, h int) {
	f := d.fonts
	if m := compactMagnitude.FindStringSubmatch(value); m != nil {
		const digitW = 42
		centerX := x + round(float64(w)/2)
		d.textInBox(f.DisplayLarge, m[1], centerX-digitW-7, y, digitW, h, White)
		d.textInBox(f.DisplayLarge, m[2], centerX+7, y, digitW, h, White)
		d.c.FillRect(centerX-3, y+h-20, 7, 7, White)
		return
	}
	font := f.Sans16
	if f.DisplayLarge.Width(value) <= w-4 {
		font = f.DisplayLarge
	}
	d.textInBox(font, value, x, y, w, h, White)
}

type quake struct{ time, depth, coords string }

var (
	quakeDateTime = regexp.MustCompile(`(\d{1,2})-(\d{1,2})\s+(\d{1,2}):(\d{2})`)
	quakeTime     = regexp.MustCompile(`(\d{1,2}):(\d{2})`)
)

func quakeFields(item EarthquakeItem) quake {
	q := quake{depth: "--"}
	if m := quakeDateTime.FindStringSubmatch(item.OccurredAt); m != nil {
		mo, _ := strconv.Atoi(m[1])
		day, _ := strconv.Atoi(m[2])
		q.time = fmt.Sprintf("%d/%d %s:%s", mo, day, m[3], m[4])
	} else if m := quakeTime.FindStringSubmatch(item.OccurredAt); m != nil {
		q.time = m[1] + ":" + m[2]
	} else {
		q.time = item.OccurredAt
	}
	if dep := or(item.DepthKm, ""); dep != "" && dep != "-" && dep != "--" {
		q.depth = dep + "千米"
	}
	var coords []string
	if item.Longitude != "" {
		coords = append(coords, "经"+item.Longitude)
	}
	if item.Latitude != "" {
		coords = append(coords, "纬"+item.Latitude)
	}
	for i, c := range coords {
		if i > 0 {
			q.coords += "  "
		}
		q.coords += c
	}
	return q
}
