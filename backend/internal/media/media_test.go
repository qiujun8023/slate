package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"slate/internal/api"
)

// The samples cover every extension the upload form accepts, so a trimmed ffmpeg build
// that drops a demuxer or decoder fails here. The Docker build runs this test against
// the ffmpeg it ships.
func TestTranscodeFormats(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	files, err := filepath.Glob("testdata/tone.*")
	if err != nil || len(files) == 0 {
		t.Fatalf("no samples: %v", err)
	}
	tr := New()
	for _, f := range files {
		t.Run(filepath.Ext(f), func(t *testing.T) {
			in, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			pcm, err := tr.Transcode(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			// One second of 16 kHz mono s16le, give or take codec padding.
			if want := api.AudioSampleRate * 2; len(pcm) < want*9/10 || len(pcm) > want*11/10 {
				t.Fatalf("got %d bytes, want about %d", len(pcm), want)
			}
		})
	}
}

func TestResample(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	in := make([]byte, 24000*2) // one second of 24 kHz silence, the TTS output rate
	pcm, err := New().Resample(context.Background(), in, 24000)
	if err != nil {
		t.Fatal(err)
	}
	if want := api.AudioSampleRate * 2; len(pcm) < want*9/10 || len(pcm) > want*11/10 {
		t.Fatalf("got %d bytes, want about %d", len(pcm), want)
	}
}
