package blob

import (
	"os"
	"testing"
	"time"
)

func TestGCKeepsBlobReusedByPut(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("frame")
	hash, err := s.Put(data)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(s.path(hash), old, old); err != nil {
		t.Fatal(err)
	}

	// A request re-uses the unreferenced blob before committing its row.
	if _, err := s.Put(data); err != nil {
		t.Fatal(err)
	}
	removed, err := s.GC(func(string) bool { return false }, time.Hour)
	if err != nil || removed != 0 || !s.Exists(hash) {
		t.Fatalf("re-used blob collected: removed=%d err=%v", removed, err)
	}

	if err := os.Chtimes(s.path(hash), old, old); err != nil {
		t.Fatal(err)
	}
	if removed, _ := s.GC(func(string) bool { return false }, time.Hour); removed != 1 || s.Exists(hash) {
		t.Fatalf("stale blob kept: removed=%d", removed)
	}
}
