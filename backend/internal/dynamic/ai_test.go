package dynamic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSynthesizeNeedsStopFinish(t *testing.T) {
	audio := `data: {"choices":[{"delta":{"audio":{"data":"AAABAA=="}},"finish_reason":null}]}`
	stop := `data: {"choices":[{"delta":{"audio":null},"finish_reason":"stop"}]}`
	for name, tc := range map[string]struct {
		lines []string
		ok    bool
	}{
		"complete":  {[]string{audio, stop, `data: [DONE]`}, true},
		"truncated": {[]string{audio}, false},
		"length":    {[]string{audio, `data: {"choices":[{"delta":{},"finish_reason":"length"}]}`}, false},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for _, l := range tc.lines {
					fmt.Fprintf(w, "%s\n\n", l)
				}
			}))
			defer srv.Close()
			pcm, err := llm{"key", srv.URL, "m", srv.Client()}.synthesize(context.Background(), "你好", "冰糖")
			if tc.ok && (err != nil || len(pcm) != 4) {
				t.Fatalf("got %d bytes, %v; want 4 bytes", len(pcm), err)
			}
			if !tc.ok && err == nil {
				t.Fatal("want error")
			}
		})
	}
}
