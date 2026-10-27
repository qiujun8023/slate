package hotlist

import (
	"os"
	"testing"
)

func TestEverySourceIsImplemented(t *testing.T) {
	for _, s := range Catalog() {
		if sources[s.ID] == nil {
			t.Errorf("catalog source %s has no fetcher", s.ID)
		}
	}
	if len(sources) != len(Catalog()) {
		t.Errorf("%d fetchers for %d catalog entries", len(sources), len(Catalog()))
	}
}

func TestAtWalksJSON(t *testing.T) {
	v := map[string]any{"data": map[string]any{"list": []any{map[string]any{"id": 12.0, "title": "x"}}}}
	if str(v, "data.list.0.id") != "12" || str(v, "data.list.0.title") != "x" || at(v, "data.list.1") != nil {
		t.Fatal("path lookup failed")
	}
}

// TestLiveSources reads real sites; run with SLATE_TEST_LIVE_HOTLIST=1.
func TestLiveSources(t *testing.T) {
	if os.Getenv("SLATE_TEST_LIVE_HOTLIST") == "" {
		t.Skip("set SLATE_TEST_LIVE_HOTLIST=1 to fetch real sources")
	}
	for _, s := range Catalog() {
		t.Run(s.ID, func(t *testing.T) {
			items, err := Fetch(t.Context(), s.ID)
			if err != nil || len(items) == 0 {
				t.Errorf("%d items, err %v", len(items), err)
			}
		})
	}
}
