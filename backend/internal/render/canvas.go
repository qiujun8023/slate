// Package render draws dynamic frames with bitmap fonts directly into 1bpp canvases.
package render

import (
	"math"

	"slate/internal/api"
)

const (
	Black uint8 = 0
	White uint8 = 1
)

type Canvas struct {
	W, H int
	px   []uint8
}

func NewCanvas(w, h int) *Canvas {
	c := &Canvas{W: w, H: h, px: make([]uint8, w*h)}
	c.Clear(White)
	return c
}

func (c *Canvas) Clear(color uint8) {
	for i := range c.px {
		c.px[i] = color
	}
}

func (c *Canvas) Set(x, y int, color uint8) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	c.px[y*c.W+x] = color
}

func (c *Canvas) HLine(x, y, w int, color uint8) {
	if y < 0 || y >= c.H || w <= 0 {
		return
	}
	for xx := max(0, x); xx < min(c.W, x+w); xx++ {
		c.px[y*c.W+xx] = color
	}
}

func (c *Canvas) VLine(x, y, h int, color uint8) {
	if x < 0 || x >= c.W || h <= 0 {
		return
	}
	for yy := max(0, y); yy < min(c.H, y+h); yy++ {
		c.px[yy*c.W+x] = color
	}
}

func (c *Canvas) FillRect(x, y, w, h int, color uint8) {
	for yy := max(0, y); yy < min(c.H, y+h); yy++ {
		for xx := max(0, x); xx < min(c.W, x+w); xx++ {
			c.px[yy*c.W+xx] = color
		}
	}
}

func (c *Canvas) StrokeRect(x, y, w, h int, color uint8) {
	c.HLine(x, y, w, color)
	c.HLine(x, y+h-1, w, color)
	c.VLine(x, y, h, color)
	c.VLine(x+w-1, y, h, color)
}

// Line draws with Bresenham's algorithm.
func (c *Canvas) Line(x0, y0, x1, y1 int, color uint8) {
	dx := abs(x1 - x0)
	sx := 1
	if x0 >= x1 {
		sx = -1
	}
	dy := -abs(y1 - y0)
	sy := 1
	if y0 >= y1 {
		sy = -1
	}
	e := dx + dy
	for {
		c.Set(x0, y0, color)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x0 += sx
		}
		if e2 <= dx {
			e += dx
			y0 += sy
		}
	}
}

// Glyph draws one code point with its baseline at baselineY and returns the advance.
func (c *Canvas) Glyph(f *Font, cp rune, x, baselineY int, color uint8) int {
	g, ok := f.Glyphs[cp]
	if !ok {
		return 0
	}
	startX := x + g.OfsX
	startY := baselineY - g.OfsY - g.BoxH
	bit := g.BitmapIndex * 8
	for yy := 0; yy < g.BoxH; yy++ {
		for xx := 0; xx < g.BoxW; xx++ {
			if b := bit >> 3; b < len(f.Bitmap) && f.Bitmap[b]&(0x80>>(bit&7)) != 0 {
				c.Set(startX+xx, startY+yy, color)
			}
			bit++
		}
	}
	return g.Advance()
}

// Mask draws the set pixels of m with its top-left corner at x, y.
func (c *Canvas) Mask(m *Mask, x, y int, color uint8) {
	for yy := 0; yy < m.H; yy++ {
		for xx := 0; xx < m.W; xx++ {
			if m.Px[yy*m.W+xx] {
				c.Set(x+xx, y+yy, color)
			}
		}
	}
}

// Packed returns the frame layout: MSB first, bit 1 white.
func (c *Canvas) Packed() []byte {
	out := make([]byte, (c.W*c.H+7)/8)
	for i, v := range c.px {
		if v != 0 {
			out[i>>3] |= 0x80 >> (i & 7)
		}
	}
	return out
}

type Mask struct {
	W, H int
	Px   []bool
}

// round matches JavaScript Math.round, which rounds halves toward +Inf.
func round(v float64) int { return int(math.Floor(v + 0.5)) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

const (
	frameW       = api.FrameWidth
	frameH       = api.FrameHeight
	statusBarH   = api.StatusBarHeight
	safeTop      = statusBarH
	safeBottom   = frameH
	contentLeft  = 20
	contentRight = frameW - 20
	contentWidth = contentRight - contentLeft
	fallbackText = "暂无数据"
)
