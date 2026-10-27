package render

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"sync"
)

//go:embed assets/fonts/*.json assets/icons/*.png
var assets embed.FS

type Glyph struct {
	BitmapIndex int `json:"bitmap_index"`
	AdvW        int `json:"adv_w"`
	BoxW        int `json:"box_w"`
	BoxH        int `json:"box_h"`
	OfsX        int `json:"ofs_x"`
	OfsY        int `json:"ofs_y"`
}

// Advance is the pen advance in pixels; adv_w is stored in 1/16 px.
func (g Glyph) Advance() int { return round(float64(g.AdvW) / 16) }

type Font struct {
	Name       string
	LineHeight int
	BaseLine   int
	Glyphs     map[rune]Glyph
	Bitmap     []byte
}

func (f *Font) Has(cp rune) bool { _, ok := f.Glyphs[cp]; return ok }

// Width sums glyph advances; missing glyphs add nothing.
func (f *Font) Width(text string) int {
	w := 0
	for _, ch := range text {
		if g, ok := f.Glyphs[ch]; ok {
			w += g.Advance()
		}
	}
	return w
}

var (
	fontMu    sync.Mutex
	fontCache = map[string]*Font{}
)

// LoadFont reads an embedded font such as "spleen-6x12.json"; fonts are cached.
func LoadFont(file string) (*Font, error) {
	fontMu.Lock()
	defer fontMu.Unlock()
	if f, ok := fontCache[file]; ok {
		return f, nil
	}
	raw, err := assets.ReadFile("assets/fonts/" + file)
	if err != nil {
		return nil, err
	}
	var s struct {
		Name         string           `json:"name"`
		LineHeight   int              `json:"lineHeight"`
		BaseLine     int              `json:"baseLine"`
		Glyphs       map[string]Glyph `json:"glyphs"`
		BitmapBase64 string           `json:"bitmapBase64"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("font %s: %w", file, err)
	}
	bitmap, err := base64.StdEncoding.DecodeString(s.BitmapBase64)
	if err != nil {
		return nil, fmt.Errorf("font %s: %w", file, err)
	}
	f := &Font{Name: s.Name, LineHeight: s.LineHeight, BaseLine: s.BaseLine, Glyphs: make(map[rune]Glyph, len(s.Glyphs)), Bitmap: bitmap}
	for k, g := range s.Glyphs {
		cp, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("font %s: bad code point %q", file, k)
		}
		f.Glyphs[rune(cp)] = g
	}
	fontCache[file] = f
	return f, nil
}

// Fonts are the faces the frame layouts use.
type Fonts struct {
	Sans16, Sans12, CalendarSub10, Metric12, Fallback16, DisplayLarge *Font
}

func LoadFonts() (*Fonts, error) {
	load := func(file string) *Font {
		f, err := LoadFont(file)
		if err != nil {
			panic(err)
		}
		return f
	}
	var fs Fonts
	err := func() (err error) {
		defer func() {
			if v := recover(); v != nil {
				err = fmt.Errorf("%v", v)
			}
		}()
		fusion10 := load("fusion-pixel-10.json")
		fs = Fonts{
			Sans16:        load("source-han-sans-16-slim.json"),
			Sans12:        fusion10,
			CalendarSub10: fusion10,
			Metric12:      load("spleen-6x12.json"),
			Fallback16:    load("unifont-16.json"),
			DisplayLarge:  load("spleen-32x64.json"),
		}
		return nil
	}()
	return &fs, err
}

// fallback is the face used for glyphs missing from f. Only the 16px sans face has one:
// a fallback with another line height would misalign rows.
func (fs *Fonts) fallback(f *Font) *Font {
	if f == fs.Sans16 {
		return fs.Fallback16
	}
	return nil
}

var (
	iconMu    sync.Mutex
	iconCache = map[string]*Mask{}
)

// WeatherIcon returns a QWeather icon mask of 30 or 70 px; unknown codes use 999.
func WeatherIcon(code, size int) *Mask {
	iconMu.Lock()
	defer iconMu.Unlock()
	key := fmt.Sprintf("%d-%d", code, size)
	if m, ok := iconCache[key]; ok {
		return m
	}
	f, err := assets.Open("assets/icons/" + key + ".png")
	if err != nil {
		if code != 999 {
			iconMu.Unlock()
			m := WeatherIcon(999, size)
			iconMu.Lock()
			return m
		}
		return nil
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil
	}
	m := maskFromImage(img)
	iconCache[key] = m
	return m
}

func maskFromImage(img image.Image) *Mask {
	b := img.Bounds()
	m := &Mask{W: b.Dx(), H: b.Dy(), Px: make([]bool, b.Dx()*b.Dy())}
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			r, _, _, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			m.Px[y*m.W+x] = r < 0x8000
		}
	}
	return m
}
