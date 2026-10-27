package render

import (
	"math"
	"strings"
	"unicode"
)

type align int

const (
	alignLeft align = iota
	alignCenter
	alignRight
)

// textOpts mirrors the layout knobs every renderer uses: x is the left edge, center or
// right edge depending on align.
type textOpts struct {
	align    align
	maxWidth int
	maxLines int
	ellipsis bool
	lineGap  int
	color    uint8
}

func (o textOpts) withDefaults() textOpts {
	if o.maxWidth == 0 {
		o.maxWidth = frameW
	}
	if o.maxLines == 0 {
		o.maxLines = 1
	}
	if o.lineGap == 0 {
		o.lineGap = 3
	}
	return o
}

// drawer pairs a canvas with the font set so renderers can lay out text with fallbacks.
type drawer struct {
	c     *Canvas
	fonts *Fonts
}

func widthFallback(f, fb *Font, text string) int {
	w := 0
	for _, ch := range text {
		g, ok := f.Glyphs[ch]
		if !ok && fb != nil {
			g, ok = fb.Glyphs[ch]
		}
		if ok {
			w += g.Advance()
		}
	}
	return w
}

func drawable(f, fb *Font, text string) string {
	var b strings.Builder
	for _, ch := range text {
		if f.Has(ch) || (fb != nil && fb.Has(ch)) || ch == ' ' {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func ellipsize(f, fb *Font, text string, maxWidth int) string {
	ell := "."
	if f.Has('…') || (fb != nil && fb.Has('…')) {
		ell = "…"
	}
	chars := []rune(text)
	lo, hi := 0, len(chars)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if widthFallback(f, fb, string(chars[:mid])+ell) <= maxWidth {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(chars[:lo]) + ell
}

func normalizeSpace(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var b strings.Builder
	space := false
	for _, ch := range text {
		if ch != '\n' && unicode.IsSpace(ch) {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(ch)
	}
	if space {
		b.WriteByte(' ')
	}
	return strings.TrimSpace(b.String())
}

// wrapText breaks by character, keeps at most maxLines lines and ellipsizes the last
// one when text was cut.
func wrapText(f, fb *Font, text string, maxWidth, maxLines int, ellipsis bool) []string {
	source := normalizeSpace(text)
	if source == "" {
		return nil
	}
	var lines []string
	cur := ""
	truncated := false
	for _, ch := range source {
		if ch == '\n' {
			if cur != "" {
				lines = append(lines, cur)
			}
			cur = ""
			if len(lines) >= maxLines {
				truncated = true
				break
			}
			continue
		}
		next := cur + string(ch)
		if widthFallback(f, fb, next) > maxWidth && cur != "" {
			lines = append(lines, cur)
			cur = string(ch)
			if len(lines) >= maxLines {
				truncated = true
				break
			}
		} else {
			cur = next
		}
	}
	if len(lines) < maxLines && cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i, l := range lines {
		lines[i] = drawable(f, fb, l)
	}
	if ellipsis && len(lines) > 0 {
		last := len(lines) - 1
		if (len(lines) == maxLines && widthFallback(f, fb, lines[last]) > maxWidth) || truncated {
			lines[last] = ellipsize(f, fb, lines[last], maxWidth)
		}
	}
	return lines
}

func faceFor(f, fb *Font, ch rune) *Font {
	if f.Has(ch) {
		return f
	}
	if fb != nil && fb.Has(ch) {
		return fb
	}
	return nil
}

type bounds struct{ left, right, top, bottom int }

// pixelBounds measures the inked box of text drawn at origin 0,0 (top of the line box).
func pixelBounds(f, fb *Font, text string) (bounds, bool) {
	pen := 0
	b := bounds{math.MaxInt, math.MinInt, math.MaxInt, math.MinInt}
	for _, ch := range text {
		face := faceFor(f, fb, ch)
		if face == nil {
			pen += max(1, round(float64(f.LineHeight)/2))
			continue
		}
		g := face.Glyphs[ch]
		top := face.LineHeight - face.BaseLine - g.OfsY - g.BoxH
		left := pen + g.OfsX
		b.left, b.right = min(b.left, left), max(b.right, left+g.BoxW)
		b.top, b.bottom = min(b.top, top), max(b.bottom, top+g.BoxH)
		pen += g.Advance()
	}
	return b, b.left != math.MaxInt
}

// visualBounds is the vertical inked extent of text, ignoring advance.
func visualBounds(f, fb *Font, text string) (top, bottom int, ok bool) {
	b, ok := pixelBounds(f, fb, text)
	return b.top, b.bottom, ok
}

func (d drawer) line(f, fb *Font, text string, x, y int, color uint8) int {
	pen := x
	for _, ch := range text {
		face := faceFor(f, fb, ch)
		if face == nil {
			pen += max(1, round(float64(f.LineHeight)/2))
			continue
		}
		pen += d.c.Glyph(face, ch, pen, y+face.LineHeight-face.BaseLine, color)
	}
	return pen - x
}

func alignedX(x, width int, a align) int {
	switch a {
	case alignCenter:
		return round(float64(x) - float64(width)/2)
	case alignRight:
		return x - width
	}
	return x
}

// text draws wrapped text and returns the height used.
func (d drawer) text(f *Font, s string, x, y int, o textOpts) int {
	return d.textStroke(f, s, x, y, o, false)
}

// strongText fakes bold by drawing every line twice, one pixel apart.
func (d drawer) strongText(f *Font, s string, x, y int, o textOpts) int {
	return d.textStroke(f, s, x, y, o, true)
}

func (d drawer) textStroke(f *Font, s string, x, y int, o textOpts, bold bool) int {
	o = o.withDefaults()
	fb := d.fonts.fallback(f)
	lines := wrapText(f, fb, s, o.maxWidth, o.maxLines, o.ellipsis)
	cy := y
	for _, l := range lines {
		dx := alignedX(x, widthFallback(f, fb, l), o.align)
		d.line(f, fb, l, dx, cy, o.color)
		if bold {
			d.line(f, fb, l, dx+1, cy, o.color)
		}
		cy += f.LineHeight + o.lineGap
	}
	if len(lines) == 0 {
		return 0
	}
	return len(lines)*f.LineHeight + (len(lines)-1)*o.lineGap
}

// textInBox centers the inked bounds of one line inside the box.
func (d drawer) textInBox(f *Font, s string, x, y, w, h int, color uint8) {
	fb := d.fonts.fallback(f)
	l := drawable(f, fb, strings.TrimSpace(s))
	if l == "" {
		return
	}
	b, ok := pixelBounds(f, fb, l)
	if !ok {
		return
	}
	dx := round(float64(x) + float64(w-(b.right-b.left))/2 - float64(b.left))
	dy := round(float64(y) + float64(h-(b.bottom-b.top))/2 - float64(b.top))
	d.line(f, fb, l, dx, dy, color)
}

// textCenteredY draws one line whose inked box is vertically centered on centerY.
func (d drawer) textCenteredY(f *Font, s string, x int, centerY float64, o textOpts) {
	o = o.withDefaults()
	fb := d.fonts.fallback(f)
	lines := wrapText(f, fb, s, o.maxWidth, 1, o.ellipsis)
	if len(lines) == 0 {
		return
	}
	b, ok := pixelBounds(f, fb, lines[0])
	if !ok {
		return
	}
	dx := alignedX(x, widthFallback(f, fb, lines[0]), o.align)
	dy := round(centerY - float64(b.bottom-b.top)/2 - float64(b.top))
	d.line(f, fb, lines[0], dx, dy, o.color)
}

func (d drawer) width(f *Font, s string) int { return widthFallback(f, d.fonts.fallback(f), s) }

func (d drawer) rule(x, y, w int, dashed bool, color uint8) {
	if !dashed {
		d.c.HLine(x, y, w, color)
		return
	}
	for xx := 0; xx < w; xx += 8 {
		d.c.HLine(x+xx, y, min(4, w-xx), color)
	}
}

func (d drawer) vrule(x, y, h int, dashed bool) {
	if !dashed {
		d.c.VLine(x, y, h, Black)
		return
	}
	for yy := 0; yy < h; yy += 8 {
		d.c.VLine(x, y+yy, min(4, h-yy), Black)
	}
}

func (d drawer) sparkline(x, y, w, h int, values []float64) {
	if len(values) < 2 || w <= 1 || h <= 1 {
		return
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	// Halve before subtracting so values near ±MaxFloat64 cannot overflow to Inf.
	half := hi/2 - lo/2
	level := func(v float64) int {
		t := 0.0
		if half > 0 {
			t = (v/2 - lo/2) / half
		}
		if !(t >= 0) { // NaN or negative
			t = 0
		}
		return y + h - round(math.Min(t, 1)*float64(h))
	}
	px, py := x, level(values[0])
	for i := 1; i < len(values); i++ {
		xx := x + round(float64(w*i)/float64(len(values)-1))
		yy := level(values[i])
		d.c.Line(px, py, xx, yy, Black)
		px, py = xx, yy
	}
}

func (d drawer) badge(label string, x, y, w, h int, filled bool) {
	color := Black
	if filled {
		d.c.FillRect(x, y, w, h, Black)
		color = White
	} else {
		d.c.StrokeRect(x, y, w, h, Black)
	}
	f := d.fonts.Metric12
	switch {
	case d.fonts.Sans16.Width(label) <= w-4:
		f = d.fonts.Sans16
	case d.fonts.Sans12.Width(label) <= w-4:
		f = d.fonts.Sans12
	}
	d.textInBox(f, label, x, y, w, h, color)
}
