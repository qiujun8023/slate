// Command genfont converts bitmap fonts into the 1bpp JSON format embedded by
// internal/render: an LVGL C font from lv_font_conv, or a BDF font.
//
//	go run ./cmd/genfont lvgl <font.c> <out.json>
//	go run ./cmd/genfont bdf <font.bdf> <out.json>
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type glyph struct {
	BitmapIndex int `json:"bitmap_index"`
	AdvW        int `json:"adv_w"`
	BoxW        int `json:"box_w"`
	BoxH        int `json:"box_h"`
	OfsX        int `json:"ofs_x"`
	OfsY        int `json:"ofs_y"`
}

type font struct {
	Name         string           `json:"name"`
	LineHeight   int              `json:"lineHeight"`
	BaseLine     int              `json:"baseLine"`
	Glyphs       map[string]glyph `json:"glyphs"`
	BitmapBase64 string           `json:"bitmapBase64"`
}

func main() {
	if len(os.Args) != 4 || (os.Args[1] != "lvgl" && os.Args[1] != "bdf") {
		fmt.Fprintln(os.Stderr, "usage: genfont lvgl|bdf <input> <out.json>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Args[3]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(format, input, output string) error {
	src, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	var f font
	if format == "lvgl" {
		f, err = parseLVGL(string(src))
	} else {
		f, err = parseBDF(string(src))
	}
	if err != nil {
		return err
	}
	out, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(output, append(out, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("extracted %d glyphs -> %s\n", len(f.Glyphs), output)
	return nil
}

// bitWriter appends MSB-first bits; each glyph starts on a byte boundary.
type bitWriter struct {
	buf []byte
	bit int
}

func (w *bitWriter) align() int {
	w.bit = (w.bit + 7) &^ 7
	return w.bit >> 3
}

func (w *bitWriter) put(on bool) {
	for len(w.buf) <= w.bit>>3 {
		w.buf = append(w.buf, 0)
	}
	if on {
		w.buf[w.bit>>3] |= 0x80 >> (w.bit & 7)
	}
	w.bit++
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func match(src string, re *regexp.Regexp, what string) ([]string, error) {
	m := re.FindStringSubmatch(src)
	if m == nil {
		return nil, errors.New(what + " not found")
	}
	return m, nil
}

var (
	bdfFont     = regexp.MustCompile(`(?m)^FONT\s+(.+)$`)
	bdfBBox     = regexp.MustCompile(`(?m)^FONTBOUNDINGBOX\s+-?\d+\s+(-?\d+)\s+-?\d+\s+-?\d+$`)
	bdfChar     = regexp.MustCompile(`(?ms)^STARTCHAR.*?^ENDCHAR$`)
	bdfEncoding = regexp.MustCompile(`(?m)^ENCODING\s+(-?\d+)$`)
	bdfDWidth   = regexp.MustCompile(`(?m)^DWIDTH\s+(-?\d+)\s+-?\d+$`)
	bdfBBX      = regexp.MustCompile(`(?m)^BBX\s+(-?\d+)\s+(-?\d+)\s+(-?\d+)\s+(-?\d+)$`)
	bdfBitmap   = regexp.MustCompile(`(?ms)^BITMAP\n(.*?)\nENDCHAR$`)
)

func bdfProperty(src, key string, fallback int) int {
	m := regexp.MustCompile(`(?m)^` + key + `\s+(-?\d+)`).FindStringSubmatch(src)
	if m == nil {
		return fallback
	}
	return atoi(m[1])
}

func parseBDF(src string) (font, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	bbox, err := match(src, bdfBBox, "FONTBOUNDINGBOX")
	if err != nil {
		return font{}, err
	}
	ascent := bdfProperty(src, "FONT_ASCENT", atoi(bbox[1]))
	descent := bdfProperty(src, "FONT_DESCENT", 0)
	f := font{Name: "bdf_font", LineHeight: ascent + descent, BaseLine: descent, Glyphs: map[string]glyph{}}
	if m := bdfFont.FindStringSubmatch(src); m != nil {
		f.Name = strings.TrimSpace(m[1])
	}
	var w bitWriter
	for _, block := range bdfChar.FindAllString(src, -1) {
		enc, err := match(block, bdfEncoding, "ENCODING")
		if err != nil {
			return font{}, err
		}
		if atoi(enc[1]) < 0 {
			continue
		}
		dw, err := match(block, bdfDWidth, "DWIDTH")
		if err != nil {
			return font{}, err
		}
		box, err := match(block, bdfBBX, "BBX")
		if err != nil {
			return font{}, err
		}
		bm, err := match(block, bdfBitmap, "BITMAP")
		if err != nil {
			return font{}, err
		}
		g := glyph{BitmapIndex: w.align(), AdvW: atoi(dw[1]) * 16, BoxW: atoi(box[1]), BoxH: atoi(box[2]), OfsX: atoi(box[3]), OfsY: atoi(box[4])}
		rows := strings.Fields(bm[1])
		for _, row := range rows[:min(len(rows), g.BoxH)] {
			v, _ := strconv.ParseUint(row, 16, 64)
			bits := len(row) * 4
			for x := 0; x < g.BoxW; x++ {
				w.put(v>>(bits-1-x)&1 == 1)
			}
		}
		f.Glyphs[enc[1]] = g
	}
	f.BitmapBase64 = base64.StdEncoding.EncodeToString(w.buf)
	return f, nil
}

var (
	lvComment    = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lvNumber     = regexp.MustCompile(`0x[0-9a-fA-F]+|-?\d+`)
	lvBpp        = regexp.MustCompile(`\*\s*Bpp:\s*(\d+)`)
	lvBitmap     = regexp.MustCompile(`static\s+LV_ATTRIBUTE_LARGE_CONST\s+const\s+uint8_t\s+glyph_bitmap\[\]\s*=\s*\{([\s\S]*?)\};`)
	lvGlyphDsc   = regexp.MustCompile(`static\s+const\s+lv_font_fmt_txt_glyph_dsc_t\s+glyph_dsc\[\]\s*=\s*\{([\s\S]*?)\};`)
	lvEntry      = regexp.MustCompile(`\{[^}]*\}`)
	lvLineHeight = regexp.MustCompile(`\.line_height\s*=\s*(\d+)`)
	lvBaseLine   = regexp.MustCompile(`\.base_line\s*=\s*(\d+)`)
	lvName       = regexp.MustCompile(`(?:const\s+)?lv_font_t\s+([A-Za-z0-9_]+)\s*=`)
	lvLists      = regexp.MustCompile(`static\s+const\s+uint(?:8|16|32)_t\s+([A-Za-z0-9_]+)\[\]\s*=\s*\{([\s\S]*?)\};`)
	lvCmaps      = regexp.MustCompile(`static\s+const\s+lv_font_fmt_txt_cmap_t\s+cmaps\[\]\s*=\s*\{([\s\S]*?)\};`)
)

func numbers(body string) []int {
	var out []int
	for _, s := range lvNumber.FindAllString(lvComment.ReplaceAllString(body, " "), -1) {
		n, _ := strconv.ParseInt(s, 0, 64)
		out = append(out, int(n))
	}
	return out
}

func field(entry, key string) string {
	m := regexp.MustCompile(`\.` + key + `\s*=\s*([^,}]+)`).FindStringSubmatch(entry)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func numField(entry, key string) (int, error) {
	raw := field(entry, key)
	n, err := strconv.ParseInt(raw, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("%s missing or invalid: %s", key, entry)
	}
	return int(n), nil
}

func ptrList(entry, key string, lists map[string][]int) ([]int, error) {
	name := strings.Trim(field(entry, key), "() ")
	if name == "" || name == "NULL" {
		return nil, fmt.Errorf("%s missing: %s", key, entry)
	}
	list, ok := lists[name]
	if !ok {
		return nil, errors.New(name + " not found")
	}
	return list, nil
}

func parseLVGL(src string) (font, error) {
	bpp := 1
	if m := lvBpp.FindStringSubmatch(src); m != nil {
		bpp = atoi(m[1])
	}
	if bpp != 1 && bpp != 4 {
		return font{}, fmt.Errorf("unsupported bpp: %d", bpp)
	}
	bm, err := match(src, lvBitmap, "glyph_bitmap")
	if err != nil {
		return font{}, err
	}
	bitmap := numbers(bm[1])
	dscBlock, err := match(src, lvGlyphDsc, "glyph_dsc")
	if err != nil {
		return font{}, err
	}
	var dsc []glyph
	for _, entry := range lvEntry.FindAllString(dscBlock[1], -1) {
		var g glyph
		for _, p := range []struct {
			key string
			dst *int
		}{{"bitmap_index", &g.BitmapIndex}, {"adv_w", &g.AdvW}, {"box_w", &g.BoxW}, {"box_h", &g.BoxH}, {"ofs_x", &g.OfsX}, {"ofs_y", &g.OfsY}} {
			if *p.dst, err = numField(entry, p.key); err != nil {
				return font{}, err
			}
		}
		dsc = append(dsc, g)
	}
	charmap, err := lvglCharmap(src)
	if err != nil {
		return font{}, err
	}
	lh, err := match(src, lvLineHeight, "line_height")
	if err != nil {
		return font{}, err
	}
	bl, err := match(src, lvBaseLine, "base_line")
	if err != nil {
		return font{}, err
	}
	f := font{Name: "lvgl_font", LineHeight: atoi(lh[1]), BaseLine: atoi(bl[1]), Glyphs: map[string]glyph{}}
	if m := lvName.FindStringSubmatch(src); m != nil {
		f.Name = m[1]
	}
	codepoints := make([]int, 0, len(charmap))
	for cp, id := range charmap {
		if id < len(dsc) {
			codepoints = append(codepoints, cp)
		}
	}
	sort.Ints(codepoints)
	if bpp == 1 {
		for _, cp := range codepoints {
			f.Glyphs[strconv.Itoa(cp)] = dsc[charmap[cp]]
		}
		f.BitmapBase64 = base64.StdEncoding.EncodeToString(intsToBytes(bitmap))
		return f, nil
	}
	// 4bpp sources are thresholded at half alpha and repacked glyph by glyph.
	var w bitWriter
	for _, cp := range codepoints {
		g := dsc[charmap[cp]]
		old := g.BitmapIndex * 8
		g.BitmapIndex = w.align()
		for i := 0; i < g.BoxW*g.BoxH; i++ {
			bit := old + i*4
			b := 0
			if bit>>3 < len(bitmap) {
				b = bitmap[bit>>3]
			}
			alpha := b & 0xf
			if bit&4 == 0 {
				alpha = b >> 4 & 0xf
			}
			w.put(alpha >= 8)
		}
		f.Glyphs[strconv.Itoa(cp)] = g
	}
	f.BitmapBase64 = base64.StdEncoding.EncodeToString(w.buf)
	return f, nil
}

func lvglCharmap(src string) (map[int]int, error) {
	block, err := match(src, lvCmaps, "cmaps")
	if err != nil {
		return nil, err
	}
	lists := map[string][]int{}
	for _, m := range lvLists.FindAllStringSubmatch(src, -1) {
		lists[m[1]] = numbers(m[2])
	}
	out := map[int]int{}
	for _, entry := range regexp.MustCompile(`(?s)\{.*?\}`).FindAllString(block[1], -1) {
		typ := field(entry, "type")
		var start, length, glyphStart, listLength int
		for _, p := range []struct {
			key string
			dst *int
		}{{"range_start", &start}, {"range_length", &length}, {"glyph_id_start", &glyphStart}, {"list_length", &listLength}} {
			if *p.dst, err = numField(entry, p.key); err != nil {
				return nil, err
			}
		}
		switch {
		case strings.Contains(typ, "FORMAT0_TINY"):
			for i := 0; i < length; i++ {
				out[start+i] = glyphStart + i
			}
		case strings.Contains(typ, "SPARSE_TINY"):
			list, err := ptrList(entry, "unicode_list", lists)
			if err != nil {
				return nil, err
			}
			for i := 0; i < min(listLength, len(list)); i++ {
				out[start+list[i]] = glyphStart + i
			}
		case strings.Contains(typ, "FORMAT0_FULL"):
			ofs, err := ptrList(entry, "glyph_id_ofs_list", lists)
			if err != nil {
				return nil, err
			}
			for i := 0; i < min(listLength, len(ofs), length); i++ {
				if ofs[i] == 0 && i != 0 {
					continue
				}
				out[start+i] = glyphStart + ofs[i]
			}
		default:
			return nil, fmt.Errorf("unsupported cmap type: %s", typ)
		}
	}
	return out, nil
}

func intsToBytes(v []int) []byte {
	out := make([]byte, len(v))
	for i, n := range v {
		out[i] = byte(n)
	}
	return out
}
