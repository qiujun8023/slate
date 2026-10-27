// Package httpapi serves the web API, the device protocol and the web UI.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"slate/internal/api"
	"slate/internal/blob"
	"slate/internal/config"
	"slate/internal/dynamic"
	"slate/internal/media"
	"slate/internal/store"
)

type API struct {
	Config  config.Config
	Store   *store.Store
	Blobs   *blob.Store
	Dynamic *dynamic.Engine
	Media   *media.Transcoder

	limits *rateLimits
}

type httpError struct {
	status     int
	message    string
	retryAfter int
}

func (e *httpError) Error() string { return e.message }

func fail(status int, format string, args ...any) error {
	return &httpError{status: status, message: fmt.Sprintf(format, args...)}
}

func badRequest(format string, args ...any) error { return fail(400, format, args...) }
func notFound(what string) error                  { return fail(404, "%s不存在", what) }

type ctxKey int

const (
	userKey ctxKey = iota
	deviceKey
)

func userID(r *http.Request) string   { v, _ := r.Context().Value(userKey).(string); return v }
func deviceID(r *http.Request) string { v, _ := r.Context().Value(deviceKey).(string); return v }

type auth int

const (
	public auth = iota
	userAuth
	deviceAuth
)

type handler func(http.ResponseWriter, *http.Request) error

func (a *API) Handler() http.Handler {
	a.limits = newRateLimits()
	mux := http.NewServeMux()
	route := func(pattern string, mode auth, h handler) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			r, err := a.authenticate(r, mode)
			if err == nil {
				err = h(w, r)
			}
			if err != nil {
				writeRequestError(w, r, err)
			}
		})
	}
	route("GET /api/health", public, func(w http.ResponseWriter, r *http.Request) error { return ok(w, nil) })
	route("POST /api/users", public, a.register)
	route("POST /api/sessions", public, a.login)
	route("GET /api/users/me", userAuth, a.me)

	route("GET /api/devices", userAuth, a.listDevices)
	route("PUT /api/devices/order", userAuth, a.reorderDevices)
	route("POST /api/devices/claims", userAuth, a.claimDevice)
	route("GET /api/devices/{id}", userAuth, a.getDevice)
	route("PATCH /api/devices/{id}", userAuth, a.patchDevice)
	route("DELETE /api/devices/{id}/binding", userAuth, a.unbindDevice)

	route("GET /api/groups", userAuth, a.listGroups)
	route("POST /api/groups", userAuth, a.createGroup)
	route("PUT /api/groups/order", userAuth, a.reorderGroups)
	route("GET /api/groups/{id}", userAuth, a.getGroup)
	route("PATCH /api/groups/{id}", userAuth, a.renameGroup)
	route("DELETE /api/groups/{id}", userAuth, a.deleteGroup)
	route("GET /api/groups/{id}/contents", userAuth, a.listContents)
	route("POST /api/groups/{id}/contents", userAuth, a.createContent)
	route("PUT /api/groups/{id}/contents/order", userAuth, a.reorderContents)

	route("POST /api/contents/preview", userAuth, a.previewDynamic)
	route("GET /api/contents/{id}", userAuth, a.getContent)
	route("PATCH /api/contents/{id}", userAuth, a.patchContent)
	route("DELETE /api/contents/{id}", userAuth, a.deleteContent)
	route("POST /api/contents/{id}/preview", userAuth, a.previewDynamicContent)
	route("POST /api/contents/{id}/refresh", userAuth, a.refreshContent)
	route("DELETE /api/contents/{id}/audio", userAuth, a.deleteAudio)
	route("POST /api/contents/{id}/audio/tts", userAuth, a.generateTTS)
	route("POST /api/contents/{id}/data", public, a.ingest)
	route("GET /api/weather/cities", userAuth, a.searchCities)
	route("GET /api/blobs/{hash}", userAuth, a.getBlob)

	route("POST /api/device/register", public, a.registerDevice)
	route("POST /api/device/sync", deviceAuth, a.syncDevice)
	route("POST /api/device/group", deviceAuth, a.changeDeviceGroup)
	route("GET /api/device/blobs/{hash}", deviceAuth, a.getDeviceBlob)

	spa := a.spaHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("HTTP panic", "method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(v), "stack", string(debug.Stack()))
				writeError(rec, fail(500, "服务器内部错误"))
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				slog.Info("HTTP request", "method", r.Method, "path", r.URL.Path, "status", rec.Status(), "durationMs", time.Since(start).Milliseconds())
			}
		}()
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			spa.ServeHTTP(rec, r)
			return
		}
		if _, pattern := mux.Handler(r); pattern == "" {
			writeError(rec, fail(404, "接口不存在"))
			return
		}
		mux.ServeHTTP(rec, r)
	})
}

// spaHandler serves PUBLIC_DIR and falls back to index.html for paths without a file
// extension; missing assets stay 404 so broken <img> tags do not receive HTML.
func (a *API) spaHandler() http.Handler {
	dir := a.Config.PublicDir
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		clean := filepath.Clean("/" + r.URL.Path)
		if info, err := os.Stat(filepath.Join(dir, clean)); err == nil && !info.IsDir() {
			if strings.HasPrefix(clean, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(clean) != "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func ok(w http.ResponseWriter, data any) error {
	return writeJSON(w, 200, api.Envelope{Success: true, Message: "操作成功", Data: data})
}

func created(w http.ResponseWriter, data any) error {
	return writeJSON(w, 201, api.Envelope{Success: true, Message: "操作成功", Data: data})
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(b)
	return err
}

func writeBinary(w http.ResponseWriter, data []byte, cache string) error {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", cache)
	_, err := w.Write(data)
	return err
}

// statusError lets other packages return client-facing errors; Error() is shown to the user.
type statusError interface {
	error
	HTTPStatus() int
}

func writeRequestError(w http.ResponseWriter, r *http.Request, err error) {
	var e *httpError
	var se statusError
	switch {
	case errors.As(err, &e), errors.As(err, &se):
	case errors.Is(err, context.Canceled) && r.Context().Err() != nil:
		slog.Info("HTTP request canceled by client", "method", r.Method, "path", r.URL.Path)
		return
	default:
		slog.Error("HTTP request failed", "method", r.Method, "path", r.URL.Path, "error", err.Error())
	}
	writeError(w, err)
}

func writeError(w http.ResponseWriter, err error) {
	var e *httpError
	var se statusError
	switch {
	case errors.As(err, &e):
	case errors.As(err, &se):
		e = &httpError{status: se.HTTPStatus(), message: se.Error()}
	default:
		e = &httpError{status: 500, message: "服务器内部错误"}
	}
	if e.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(e.retryAfter))
	}
	_ = writeJSON(w, e.status, api.Envelope{Message: e.message})
}

// decodeJSON reads a JSON object body of at most limit bytes and rejects unknown fields.
func decodeJSON(r *http.Request, limit int64, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return jsonError(err, limit)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return badRequest("请求体不是合法 JSON")
		}
		return jsonError(err, limit)
	}
	if raw[0] != '{' {
		return badRequest("请求体须为 JSON 对象")
	}
	obj := json.NewDecoder(bytes.NewReader(raw))
	obj.DisallowUnknownFields()
	if err := obj.Decode(v); err != nil {
		return jsonError(err, limit)
	}
	return nil
}

func jsonError(err error, limit int64) error {
	var tooLarge *http.MaxBytesError
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooLarge):
		return fail(413, "请求体超过 %dKB", limit/1024)
	case errors.Is(err, io.EOF):
		return badRequest("请求体不能为空")
	case errors.As(err, &typeErr):
		return badRequest("参数 %s 类型错误", typeErr.Field)
	case errors.As(err, &syntax), errors.Is(err, io.ErrUnexpectedEOF):
		return badRequest("请求体不是合法 JSON")
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return badRequest("不支持参数 %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
	default:
		return badRequest("请求体无效: %s", err.Error())
	}
}

// clientIP trusts X-Forwarded-For only from loopback or private-network peers, so a
// client connecting directly cannot spoof its address to dodge rate limits.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return host
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := net.ParseIP(strings.TrimSpace(parts[i]))
			if candidate == nil {
				break
			}
			if !(candidate.IsLoopback() || candidate.IsPrivate() || candidate.IsLinkLocalUnicast()) || i == 0 {
				return candidate.String()
			}
		}
	}
	return host
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Status() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
