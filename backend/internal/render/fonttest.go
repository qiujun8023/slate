package render

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"slate/internal/api"
)

//go:embed assets/fonts.json
var fontCatalogJSON []byte

type FontEntry struct {
	api.FontInfo
	File string `json:"file"`
}

var fontCatalog = func() struct {
	Fonts      []FontEntry `json:"fonts"`
	IconSample string      `json:"iconSample"`
} {
	var c struct {
		Fonts      []FontEntry `json:"fonts"`
		IconSample string      `json:"iconSample"`
	}
	if err := json.Unmarshal(fontCatalogJSON, &c); err != nil {
		panic(err)
	}
	return c
}()

// FontCatalog lists the fonts the font test frame can show.
func FontCatalog() []FontEntry { return fontCatalog.Fonts }

func FontEntryByID(id string) *FontEntry {
	for i := range fontCatalog.Fonts {
		if fontCatalog.Fonts[i].ID == id {
			return &fontCatalog.Fonts[i]
		}
	}
	return nil
}

// Some pixel fonts declare loose metrics; the specimen uses their real cell size.
var compactMetrics = map[string][2]int{
	"fusion_pixel_10": {10, 2}, "fusion_pixel_12": {12, 2}, "ark_pixel_10": {10, 2},
	"ark_pixel_12": {12, 2}, "ark_pixel_16": {16, 3},
}

func (d drawer) fontTest(cfg api.DynamicConfig) error {
	entry := FontEntryByID(cfg.FontID)
	if entry == nil {
		entry = &fontCatalog.Fonts[0]
	}
	src, err := LoadFont(entry.File)
	if err != nil {
		return err
	}
	font := src
	if m, ok := compactMetrics[entry.ID]; ok {
		compacted := *src
		compacted.LineHeight, compacted.BaseLine = m[0], m[1]
		font = &compacted
	}
	kind := entry.Kind
	if kind == "latin" && font.LineHeight >= 28 {
		kind = "display"
	}
	sp := specimenFor(kind, entry.ID)
	missing := missingGlyphs(font, sampleText(kind, sp))
	fg := Black
	if cfg.Invert {
		d.c.FillRect(0, statusBarH, frameW, frameH-statusBarH, Black)
		fg = White
	}
	switch kind {
	case "icon":
		d.iconSpecimen(font, fg)
	case "display":
		d.displaySpecimen(font, entry, sp, fg, missing)
	default:
		d.readingSpecimen(font, entry, sp, fg, missing)
	}
	return nil
}

type specimen struct {
	hero                  string
	body, metrics, glyphs []string
}

func specimenFor(kind, id string) specimen {
	switch kind {
	case "cjk":
		if id == "ark_pixel_16" {
			return specimen{"中文 0123456789 ABC abc", []string{"中文标点，。！？～", "23:59 100% OK", "A0 iIl1 O0 8B"}, []string{"0123456789 23:59"}, []string{"一二三 口日目 黑墨屏"}}
		}
		return specimen{"墨水屏字体测试 中文点阵 0123456789", []string{"今日天气 多云 23°C  风力 2 级", "简繁中文 标点，。！？～", "ABC abc 0123456789 +12.8%"}, []string{"0123456789 23:59"}, []string{"一二三 口日目 黑墨屏"}}
	case "display":
		return specimen{"23:59", []string{"86%", "+12.8", "-04"}, []string{"86% +12 -04"}, []string{"OK RUN"}}
	}
	return specimen{"Slate UI 0123456789 ABC abc", []string{"The quick brown fox jumps.", "A0 O0 I1 l1 []{} <> /\\", "23:59 100% +12.8 -04"}, []string{"0123456789 23:59"}, []string{"A0 O0 I1 l1 mwMW"}}
}

func sampleText(kind string, sp specimen) string {
	switch kind {
	case "icon":
		return fontCatalog.IconSample
	case "display":
		return "23:59 86% +12 -04 OK RUN"
	}
	all := append(append(append([]string{sp.hero}, sp.body...), sp.metrics...), sp.glyphs...)
	return strings.Join(all, " ")
}

func missingGlyphs(f *Font, text string) int {
	seen := map[rune]bool{}
	for _, ch := range text {
		if ch != ' ' && ch != '\n' && !f.Has(ch) {
			seen[ch] = true
		}
	}
	return len(seen)
}

func lineGapFor(f *Font) int {
	switch {
	case f.LineHeight <= 8:
		return 5
	case f.LineHeight <= 12:
		return 6
	case f.LineHeight <= 16:
		return 8
	case f.LineHeight <= 24:
		return 10
	}
	return 12
}

func (d drawer) readingSpecimen(f *Font, e *FontEntry, sp specimen, fg uint8, missing int) {
	x, maxW, bottom := contentLeft, contentWidth, frameH-8
	gap := lineGapFor(f)
	y := statusBarH + 9
	y += d.text(f, e.Label, x, y, textOpts{maxWidth: maxW, ellipsis: true, color: fg})
	y += max(4, gap/2)
	y += d.text(f, fmt.Sprintf("%dpx line %d glyphs %d", e.SizePx, f.LineHeight, len(f.Glyphs)), x, y, textOpts{maxWidth: maxW, ellipsis: true, color: fg})
	y += gap
	d.rule(x, y, maxW, true, fg)
	y += gap + 2
	maxLines := 1
	if f.LineHeight <= 12 {
		maxLines = 2
	}
	for _, line := range readingLines(e, sp, f, missing) {
		if y+f.LineHeight > bottom {
			break
		}
		y += d.text(f, line, x, y, textOpts{maxWidth: maxW, maxLines: maxLines, ellipsis: true, lineGap: 1, color: fg}) + gap
	}
}

func readingLines(e *FontEntry, sp specimen, f *Font, missing int) []string {
	var footer []string
	if missing > 0 {
		footer = []string{fmt.Sprintf("missing %d", missing)}
	}
	if e.Kind == "cjk" {
		if e.ID == "ark_pixel_16" {
			lines := append([]string{sp.hero}, sp.body...)
			lines = append(append(lines, sp.metrics...), sp.glyphs...)
			return append(append(lines, "ABCDEF abcdef 0123456789", "A0 O0 I1 l1 []{}<>"), footer...)
		}
		return append([]string{sp.hero, "中文测试 墨水屏 点阵字体", "今天多云 23°C 风力 2级", "黑白像素 横竖撇捺 点线面",
			"简繁中文 标点，。！？；：～", "一二三四五六七八九十 口日目田回", "0123456789 23:59 100% +12.8", "ABC abc A0 O0 I1 l1 []{}<>"}, footer...)
	}
	dense := f.LineHeight <= 12
	fox := "The quick brown fox jumps."
	if dense {
		fox = "The quick brown fox jumps over the lazy dog."
	}
	lines := []string{sp.hero, "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz", "0123456789 23:59 100% +12.8 -04",
		"A0 O0 I1 l1 mwMW []{} <> /\\", fox, "Slate UI e-paper bitmap font", "!@#$%^&*()_+-=;:,.?"}
	if dense {
		lines = append(lines, "0123456789 ABCDEF abcdef", "render align width baseline", "pixel density row spacing test", "left center right edge sample")
	}
	return append(lines, footer...)
}

func (d drawer) displaySpecimen(f *Font, e *FontEntry, sp specimen, fg uint8, missing int) {
	opts := textOpts{align: alignCenter, maxWidth: contentWidth, ellipsis: true, color: fg}
	if f.LineHeight >= 56 {
		third := "+12 -04"
		if missing > 0 {
			third = fmt.Sprintf("missing %d", missing)
		}
		for i, text := range []string{sp.hero, "86%", third} {
			d.text(f, text, 200, []int{36, 128, 220}[i], opts)
		}
		return
	}
	d.text(f, sp.hero, 200, 34, opts)
	d.rule(72, 92, 256, true, fg)
	values := []string{"86%", "+12", "-04"}
	colW := contentWidth / len(values)
	for i, v := range values {
		d.text(f, v, contentLeft+i*colW+colW/2, 112, textOpts{align: alignCenter, maxWidth: colW - 10, ellipsis: true, color: fg})
	}
	d.rule(72, 176, 256, true, fg)
	d.text(f, or(firstOr(sp.glyphs), "OK RUN"), 200, 190, opts)
	footer := e.Label
	if missing > 0 {
		footer = fmt.Sprintf("%s missing %d", e.Label, missing)
	}
	d.text(f, footer, 200, 246, opts)
}

func (d drawer) iconSpecimen(f *Font, fg uint8) {
	var icons []string
	for _, ch := range fontCatalog.IconSample {
		if ch != ' ' {
			icons = append(icons, string(ch))
		}
	}
	large := f.LineHeight >= 24
	cols, extra := 12, 10
	if large {
		cols, extra = 8, 18
	}
	startY := statusBarH + 12
	cellW := contentWidth / cols
	cellH := f.LineHeight + extra
	rows := max(1, (frameH-startY-10)/cellH)
	for i, icon := range firstN(icons, cols*rows) {
		x := contentLeft + i%cols*cellW
		y := startY + i/cols*cellH
		d.text(f, icon, x+cellW/2, y, textOpts{align: alignCenter, maxWidth: cellW, color: fg})
	}
	if !large {
		d.rule(contentLeft, 244, contentWidth, true, fg)
		d.text(f, "      ", 200, 256, textOpts{align: alignCenter, maxWidth: contentWidth, ellipsis: true, color: fg})
	}
}

func firstOr(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}
