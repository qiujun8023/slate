package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"slate/internal/api"
	"slate/internal/blob"
	"slate/internal/config"
	"slate/internal/dynamic"
	"slate/internal/media"
	"slate/internal/store"
	"slate/internal/testdb"
)

type harness struct {
	t   *testing.T
	srv *httptest.Server
	st  *store.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	cfg := config.Config{
		JWTSecret:     "integration-test-secret-0123456789abcdefghijklmnopqrstuvwxyz",
		JWTExpiration: time.Hour,
		BlobDir:       t.TempDir(),
		PublicDir:     t.TempDir(),
	}
	st, err := store.Open(ctx, testdb.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Pool.Close)
	blobs, err := blob.Open(cfg.BlobDir)
	if err != nil {
		t.Fatal(err)
	}
	m := media.New()
	engine, err := dynamic.New(cfg, st, blobs, m)
	if err != nil {
		t.Fatal(err)
	}
	a := &API{Config: cfg, Store: st, Blobs: blobs, Dynamic: engine, Media: m}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return &harness{t, srv, st}
}

// call sends a request and decodes the envelope's data into out (when non-nil).
func (h *harness) call(method, path, token string, body any, out any) int {
	h.t.Helper()
	var r io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case multipartBody:
		r, contentType = b.buf, b.contentType
	default:
		raw, _ := json.Marshal(b)
		r, contentType = bytes.NewReader(raw), "application/json"
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, r)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if out != nil && res.StatusCode < 300 {
		if b, ok := out.(*[]byte); ok {
			*b = raw
			return res.StatusCode
		}
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			h.t.Fatalf("%s %s: %v: %s", method, path, err, raw)
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			h.t.Fatalf("%s %s: %v: %s", method, path, err, env.Data)
		}
	}
	return res.StatusCode
}

func (h *harness) must(want int, got int, what string) {
	h.t.Helper()
	if got != want {
		h.t.Fatalf("%s: status %d, want %d", what, got, want)
	}
}

type multipartBody struct {
	buf         *bytes.Buffer
	contentType string
}

func frameUpload(frame []byte, frameName string) multipartBody {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	part, _ := w.CreateFormFile("frame", "frame.bin")
	_, _ = part.Write(frame)
	_ = w.WriteField("frameName", frameName)
	_ = w.Close()
	return multipartBody{buf, w.FormDataContentType()}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *harness) user(name string) string {
	h.t.Helper()
	var s api.Session
	h.must(201, h.call("POST", "/api/users", "", map[string]string{"email": name + "@example.com", "username": name, "password": "integration-pass-1"}, &s), "register "+name)
	return s.Token
}

func (h *harness) group(token, name string) string {
	h.t.Helper()
	var g struct{ ID string }
	h.must(201, h.call("POST", "/api/groups", token, map[string]string{"name": name}, &g), "create group")
	return g.ID
}

func (h *harness) device(mac string) api.DeviceRegisterResponse {
	h.t.Helper()
	var d api.DeviceRegisterResponse
	h.must(200, h.call("POST", "/api/device/register", "", api.DeviceRegisterRequest{Protocol: api.DeviceProtocol, MAC: mac}, &d), "register device")
	return d
}

func TestDeviceProtocol(t *testing.T) {
	h := newHarness(t)
	token := h.user("alice")
	gid := h.group(token, "相册")
	frame := make([]byte, api.FrameBytes)
	_, _ = rand.Read(frame)
	h.must(201, h.call("POST", "/api/groups/"+gid+"/contents", token, frameUpload(frame, "第一帧"), nil), "upload frame")

	dev := h.device("AA:BB:CC:DD:EE:01")
	var state api.DeviceState
	h.must(200, h.call("POST", "/api/device/sync", dev.Secret, api.DeviceSyncRequest{Protocol: api.DeviceProtocol}, &state), "unbound sync")
	if state.Device.Bound || state.Group != nil || state.Manifest != nil {
		t.Fatalf("unbound device got group or manifest: %+v", state)
	}

	h.must(200, h.call("POST", "/api/devices/claims", token, map[string]string{"pairCode": dev.PairCode}, nil), "claim")
	h.must(200, h.call("PATCH", "/api/devices/"+dev.DeviceID, token, map[string]any{"groupId": gid}, nil), "select group")

	h.must(200, h.call("POST", "/api/device/sync", dev.Secret, api.DeviceSyncRequest{Protocol: api.DeviceProtocol}, &state), "bound sync")
	if state.Manifest == nil || len(state.Manifest.Frames) != 1 || state.Manifest.Version != state.Group.ManifestVersion {
		t.Fatalf("bound sync manifest: %+v", state)
	}
	f := state.Manifest.Frames[0]
	if f.Image.Hash != blob.Hash(frame) || f.Image.Size != api.FrameBytes || f.StatusBarText == "" || f.RefreshAt != nil {
		t.Fatalf("frame: %+v", f)
	}

	h.must(200, h.call("POST", "/api/device/sync", dev.Secret, api.DeviceSyncRequest{Protocol: api.DeviceProtocol, ManifestVersion: state.Manifest.Version}, &state), "cached sync")
	if state.Manifest != nil {
		t.Fatal("manifest resent for an up-to-date version")
	}

	var got []byte
	h.must(200, h.call("GET", "/api/device/blobs/"+f.Image.Hash, dev.Secret, nil, &got), "download blob")
	if !bytes.Equal(got, frame) {
		t.Fatal("blob bytes differ from the upload")
	}

	h.must(426, h.call("POST", "/api/device/sync", dev.Secret, api.DeviceSyncRequest{Protocol: api.DeviceProtocol - 1}, nil), "old protocol")
	h.must(401, h.call("POST", "/api/device/sync", randomHex(32), api.DeviceSyncRequest{Protocol: api.DeviceProtocol}, nil), "unknown secret")
}

func TestPatchDeviceRequiresOwner(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.user("alice"), h.user("bob")
	dev := h.device("AA:BB:CC:DD:EE:02")
	h.must(200, h.call("POST", "/api/devices/claims", alice, map[string]string{"pairCode": dev.PairCode}, nil), "claim")

	h.must(404, h.call("PATCH", "/api/devices/"+dev.DeviceID, bob, map[string]any{"name": "bob's"}, nil), "foreign patch")

	// Renames racing an unbind either land before it or see the device as gone; none
	// may rename a device that alice no longer owns.
	var wg sync.WaitGroup
	statuses := make(chan int, 8)
	for i := range 8 {
		wg.Go(func() {
			statuses <- h.call("PATCH", "/api/devices/"+dev.DeviceID, alice, map[string]any{"name": "rename-" + strconv.Itoa(i)}, nil)
		})
	}
	h.must(200, h.call("DELETE", "/api/devices/"+dev.DeviceID+"/binding", alice, nil, nil), "unbind")
	wg.Wait()
	close(statuses)
	for s := range statuses {
		if s != 200 && s != 404 {
			t.Fatalf("concurrent patch status %d", s)
		}
	}
	var name *string
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT name FROM devices WHERE id = $1", dev.DeviceID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	h.must(404, h.call("PATCH", "/api/devices/"+dev.DeviceID, alice, map[string]any{"name": "after"}, nil), "patch after unbind")
	if name != nil && *name == "after" {
		t.Fatal("unbound device renamed")
	}
}

func TestDynamicPatchFailureKeepsContent(t *testing.T) {
	h := newHarness(t)
	token := h.user("alice")
	gid := h.group(token, "天气")
	ctx := context.Background()

	// Seed a weather frame directly: creating one through the API would need QWeather.
	hash := blob.Hash(make([]byte, api.FrameBytes))
	id := "c" + randomHex(8)
	cfg := `{"type":"weather","tz":"Asia/Shanghai","locationId":"101010100","locationLabel":"北京","refreshIntervalSec":1800}`
	data := `{"updatedAt":"` + time.Now().UTC().Format(time.RFC3339) + `"}`
	_, err := h.st.Pool.Exec(ctx, `INSERT INTO contents (id, group_id, sort_order, kind, frame_name, image_hash) VALUES ($1, $2, 0, 'dynamic', '北京天气', $3)`, id, gid, hash)
	if err == nil {
		_, err = h.st.Pool.Exec(ctx, `INSERT INTO content_dynamic (content_id, type, config, data, rendered_at) VALUES ($1, 'weather', $2, $3, now())`, id, cfg, data)
	}
	if err != nil {
		t.Fatal(err)
	}

	// Switching city must fetch fresh data; without a QWeather key that fails, and the
	// old city's config, frame name and data must stay as they were.
	patch := map[string]any{"config": map[string]any{"type": "weather", "tz": "Asia/Shanghai", "locationId": "101020100", "locationLabel": "上海", "refreshIntervalSec": 1800}}
	if s := h.call("PATCH", "/api/contents/"+id, token, patch, nil); s < 400 {
		t.Fatalf("patch status %d, want failure", s)
	}
	var gotCfg map[string]any
	var gotName string
	if err := h.st.Pool.QueryRow(ctx, `SELECT d.config, c.frame_name FROM content_dynamic d JOIN contents c ON c.id = d.content_id WHERE d.content_id = $1`, id).Scan(&gotCfg, &gotName); err != nil {
		t.Fatal(err)
	}
	if gotCfg["locationId"] != "101010100" || gotName != "北京天气" {
		t.Fatalf("failed patch was persisted: config=%v frameName=%q", gotCfg, gotName)
	}

	// Preview follows the same rule: the old city's data must not stand in for the new one.
	if s := h.call("POST", "/api/contents/"+id+"/preview", token, map[string]any{"config": patch["config"]}, nil); s < 400 {
		t.Fatalf("preview status %d, want failure", s)
	}
}

func TestReplacingImageKeepsAudio(t *testing.T) {
	h := newHarness(t)
	token := h.user("alice")
	gid := h.group(token, "相册")
	var c api.Content
	h.must(201, h.call("POST", "/api/groups/"+gid+"/contents", token, frameUpload(make([]byte, api.FrameBytes), "a"), &c), "upload")
	if _, err := h.st.Pool.Exec(context.Background(), `INSERT INTO content_audio (content_id, source, status, voice, text) VALUES ($1, 'tts', 'pending', '冰糖', '你好')`, c.ID); err != nil {
		t.Fatal(err)
	}
	next := make([]byte, api.FrameBytes)
	next[0] = 0x0f
	h.must(200, h.call("PATCH", "/api/contents/"+c.ID, token, frameUpload(next, "a"), &c), "replace image")
	if c.Audio == nil || c.Audio.Text == nil || *c.Audio.Text != "你好" {
		t.Fatalf("audio dropped on image replace: %+v", c.Audio)
	}
}

// Browsers send flac/opus/wma with an empty or generic type; the upload must still work.
func TestUploadAudioWithGenericType(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	h := newHarness(t)
	token := h.user("alice")
	gid := h.group(token, "相册")
	audio, err := os.ReadFile("../media/testdata/tone.flac")
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	part, _ := w.CreateFormFile("frame", "frame.bin")
	_, _ = part.Write(make([]byte, api.FrameBytes))
	part, _ = w.CreateFormFile("audio", "tone.flac") // application/octet-stream
	_, _ = part.Write(audio)
	_ = w.Close()
	var c api.Content
	h.must(201, h.call("POST", "/api/groups/"+gid+"/contents", token, multipartBody{buf, w.FormDataContentType()}, &c), "upload with audio")
	if c.Audio == nil || c.Audio.Hash == nil {
		t.Fatalf("audio not stored: %+v", c.Audio)
	}
}
