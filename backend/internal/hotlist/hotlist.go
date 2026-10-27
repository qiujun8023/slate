package hotlist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

	"slate/internal/fetch"
	"slate/internal/render"
)

const (
	maxItems      = 30
	sourceTimeout = 15 * time.Second
	mobileUA      = "Mozilla/5.0 (Linux; Android 10; Mi 10) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Mobile Safari/537.36"
)

// item is what the frame needs: the title, plus a link kept for the stored data.
type item struct{ title, url string }

type source func(ctx context.Context) ([]item, error)

// Fetch returns up to 30 ranked items from a source.
func Fetch(ctx context.Context, id string) ([]render.HotListItem, error) {
	src, ok := sources[id]
	if !ok {
		return nil, fmt.Errorf("未知热榜数据源: %s", id)
	}
	ctx, cancel := context.WithTimeout(ctx, sourceTimeout)
	defer cancel()
	raw, err := src(ctx)
	if err != nil {
		return nil, err
	}
	out := []render.HotListItem{}
	for _, it := range raw {
		title := fetch.StripHTML(it.title)
		if title == "" {
			continue
		}
		out = append(out, render.HotListItem{Rank: len(out) + 1, Title: title, URL: safeURL(it.url)})
		if len(out) == maxItems {
			break
		}
	}
	return out, nil
}

func safeURL(v string) string {
	u, err := url.Parse(strings.TrimSpace(v))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

func absURL(base, href string) string {
	if href = strings.TrimSpace(href); href == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	h, err := url.Parse(fetch.StripHTML(href))
	if err != nil {
		return ""
	}
	return b.ResolveReference(h).String()
}

// headers turns "Key", "Value" pairs into request options.
func headers(kv ...string) fetch.Options {
	o := fetch.Options{Headers: map[string]string{}}
	for i := 0; i+1 < len(kv); i += 2 {
		o.Headers[kv[i]] = kv[i+1]
	}
	return o
}

func getJSON(ctx context.Context, u string, o fetch.Options) (any, error) {
	var v any
	err := fetch.JSON(ctx, u, o, &v)
	return v, err
}

func getText(ctx context.Context, u string, o fetch.Options) (string, error) {
	return fetch.Text(ctx, u, o)
}

func getGBK(ctx context.Context, u string, o fetch.Options) (string, error) {
	b, err := fetch.Bytes(ctx, u, o)
	if err != nil {
		return "", err
	}
	out, err := io.ReadAll(simplifiedchinese.GBK.NewDecoder().Reader(strings.NewReader(string(b))))
	return string(out), err
}

// at walks a decoded JSON value by a dot path; numeric parts index arrays.
func at(v any, path string) any {
	if path == "" {
		return v
	}
	for _, part := range strings.Split(path, ".") {
		switch t := v.(type) {
		case map[string]any:
			v = t[part]
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(t) {
				return nil
			}
			v = t[i]
		default:
			return nil
		}
	}
	return v
}

func list(v any, path string) []any {
	l, _ := at(v, path).([]any)
	return l
}

// str returns the value at path as text; numbers print without exponent.
func str(v any, path string) string {
	switch t := at(v, path).(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	}
	return ""
}

func num(v any, path string) float64 {
	f, _ := at(v, path).(float64)
	return f
}

// firstStr returns the first non-empty value among paths.
func firstStr(v any, paths ...string) string {
	for _, p := range paths {
		if s := str(v, p); s != "" {
			return s
		}
	}
	return ""
}

// mapItems maps each element of the list at path.
func mapItems(v any, path string, f func(any) item) []item {
	var out []item
	for _, e := range list(v, path) {
		out = append(out, f(e))
	}
	return out
}

func ifSet(v, format string) string {
	if v == "" {
		return ""
	}
	return fmt.Sprintf(format, v)
}

func re(p string) *regexp.Regexp { return regexp.MustCompile(p) }

// first returns the first capture of pattern, entity-decoded and trimmed.
func first(text string, pattern *regexp.Regexp) string {
	m := pattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return fetch.StripHTML(m[1])
}

func blocks(text string, pattern *regexp.Regexp) []string {
	var out []string
	for _, m := range pattern.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

var (
	rssItem  = re(`(?is)<item\b[^>]*>(.*?)</item>`)
	rssTitle = re(`(?is)<title\b[^>]*>(.*?)</title>`)
	rssLink  = re(`(?is)<link\b[^>]*>(.*?)</link>`)
)

func rss(u string, gbk bool, o fetch.Options) source {
	return func(ctx context.Context) ([]item, error) {
		var xml string
		var err error
		if gbk {
			xml, err = getGBK(ctx, u, o)
		} else {
			xml, err = getText(ctx, u, o)
		}
		if err != nil {
			return nil, err
		}
		var out []item
		for _, b := range blocks(xml, rssItem) {
			out = append(out, item{first(b, rssTitle), first(b, rssLink)})
		}
		return out, nil
	}
}

// htmlList extracts one item per block matched by blockPattern.
func htmlList(u string, o fetch.Options, blockPattern *regexp.Regexp, f func(block string) item) source {
	return func(ctx context.Context) ([]item, error) {
		html, err := getText(ctx, u, o)
		if err != nil {
			return nil, err
		}
		var out []item
		for _, b := range blocks(html, blockPattern) {
			out = append(out, f(b))
		}
		return out, nil
	}
}

// jsonList fetches JSON and maps the list at path.
func jsonList(u string, o fetch.Options, path string, f func(any) item) source {
	return func(ctx context.Context) ([]item, error) {
		v, err := getJSON(ctx, u, o)
		if err != nil {
			return nil, err
		}
		return mapItems(v, path, f), nil
	}
}

// jsonFromHTML parses the first capture of pattern after HTML entity decoding.
func jsonFromHTML(html string, pattern *regexp.Regexp) any {
	m := pattern.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil
	}
	var v any
	if json.Unmarshal([]byte(fetch.StripHTML(m[1])), &v) != nil {
		return nil
	}
	return v
}

// balancedObject returns the first {...} in text, honoring JSON strings.
func balancedObject(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}
	depth, inString, escaped := 0, false, false
	for i := start; i < len(text); i++ {
		ch := text[i]
		switch {
		case inString && escaped:
			escaped = false
		case inString && ch == '\\':
			escaped = true
		case inString && ch == '"':
			inString = false
		case inString:
		case ch == '"':
			inString = true
		case ch == '{':
			depth++
		case ch == '}':
			if depth--; depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}
