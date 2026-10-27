package render

import (
	"math"
	"testing"
)

func TestSparklineExtremeValuesStayInBox(t *testing.T) {
	cases := [][]float64{
		{-math.MaxFloat64, math.MaxFloat64},
		{math.MaxFloat64, -math.MaxFloat64, 0},
		{-1e308, 1e308, -1e308},
		{5, 5, 5},
	}
	for _, values := range cases {
		c := NewCanvas(frameW, frameH)
		drawer{c: c}.sparkline(10, 40, 100, 50, values)
		for y := range frameH {
			for x := range frameW {
				if c.px[y*c.W+x] == Black && (x < 10 || x > 110 || y < 40 || y > 90) {
					t.Fatalf("values %v: pixel (%d,%d) outside sparkline box", values, x, y)
				}
			}
		}
	}
}

func TestToNumberRejectsNonFinite(t *testing.T) {
	for _, v := range []any{"NaN", "nan", "Inf", "-Inf", "1e999", math.NaN(), math.Inf(1)} {
		if _, ok := toNumber(v); ok {
			t.Errorf("toNumber(%v) accepted", v)
		}
	}
	if n, ok := toNumber(" 1.5 "); !ok || n != 1.5 {
		t.Errorf("toNumber(\" 1.5 \") = %v, %v", n, ok)
	}
}
