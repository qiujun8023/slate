// Package fetch is the HTTP client for third-party data sources.
package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DesktopUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
	DefaultTimeout = 5 * time.Second
	maxBody        = 8 << 20
)

type Options struct {
	Method  string
	Headers map[string]string
	Body    any
	Timeout time.Duration
	// NoUserAgent sends no User-Agent instead of a desktop browser one.
	NoUserAgent bool
}

var client = &http.Client{
	Transport: &http.Transport{
		DialContext:           publicDialer,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
	},
}

// publicDialer refuses loopback and private addresses, so no source URL or redirect can
// reach services on the host network.
func publicDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	if ip, ok := conn.RemoteAddr().(*net.TCPAddr); ok && private(ip.IP) {
		conn.Close()
		return nil, fmt.Errorf("private network address %s is not allowed", ip.IP)
	}
	return conn, nil
}

func private(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() ||
		(ip.To4() != nil && (ip.To4()[0] == 100 && ip.To4()[1]&0xc0 == 64 || ip.To4()[0] >= 240))
}

// StatusError is a non-2xx response.
type StatusError struct {
	Status int
	URL    string
	Body   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d for %s: %s", e.Status, e.URL, e.Body)
}

// Bytes performs the request and returns the body of a 2xx response.
func Bytes(ctx context.Context, rawURL string, o Options) ([]byte, error) {
	b, _, err := Do(ctx, rawURL, o)
	return b, err
}

// Do is Bytes that also returns the response headers.
func Do(ctx context.Context, rawURL string, o Options) ([]byte, http.Header, error) {
	timeout := o.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body io.Reader
	method := o.Method
	if method == "" {
		method = http.MethodGet
	}
	contentType := ""
	switch b := o.Body.(type) {
	case nil:
	case string:
		body = strings.NewReader(b)
	case []byte:
		body = bytes.NewReader(b)
	case url.Values:
		body, contentType = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return nil, nil, err
		}
		body, contentType = bytes.NewReader(raw), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, nil, err
	}
	if !o.NoUserAgent {
		req.Header.Set("User-Agent", DesktopUA)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range o.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet := strings.Join(strings.Fields(string(data)), " ")
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return nil, resp.Header, &StatusError{resp.StatusCode, redact(rawURL), snippet}
	}
	return data, resp.Header, nil
}

func Text(ctx context.Context, rawURL string, o Options) (string, error) {
	b, err := Bytes(ctx, rawURL, o)
	return string(b), err
}

func JSON(ctx context.Context, rawURL string, o Options, v any) error {
	b, err := Bytes(ctx, rawURL, o)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("invalid JSON from %s: %w", redact(rawURL), err)
	}
	return nil
}

func redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid-url>"
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return u.String()
}
