package dynamic

import (
	"context"
	"testing"

	"slate/internal/blob"
	"slate/internal/config"
	"slate/internal/media"
	"slate/internal/store"
	"slate/internal/testdb"
)

// A worker that dies during the last allowed attempt must not leave the job generating
// forever.
func TestRunTTSFailsAbandonedLastAttempt(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Pool.Close()
	blobs, err := blob.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(config.Config{}, st, blobs, media.New())
	if err != nil {
		t.Fatal(err)
	}
	seed := `
INSERT INTO users (id, email, password_hash) VALUES ('u', 'u@example.com', 'x');
INSERT INTO groups (id, owner_id, name, sort_order) VALUES ('g', 'u', 'g', 0);
INSERT INTO contents (id, group_id, sort_order, kind, image_hash)
VALUES ('abandoned', 'g', 0, 'image', repeat('0', 64)), ('live', 'g', 1, 'image', repeat('0', 64));
INSERT INTO content_audio (content_id, source, status, voice, text, lease_until, attempts) VALUES
  ('abandoned', 'tts', 'generating', '冰糖', '你好', now() - interval '1 minute', 3),
  ('live', 'tts', 'generating', '冰糖', '你好', now() + interval '1 minute', 3);`
	if _, err := st.Pool.Exec(ctx, seed); err != nil {
		t.Fatal(err)
	}

	e.runTTS(ctx)

	for id, want := range map[string]string{"abandoned": "failed", "live": "generating"} {
		var status string
		if err := st.Pool.QueryRow(ctx, "SELECT status FROM content_audio WHERE content_id = $1", id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != want {
			t.Errorf("%s: status %s, want %s", id, status, want)
		}
	}
}
