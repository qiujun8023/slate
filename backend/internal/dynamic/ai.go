package dynamic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// historyPromptVersion is part of the cache key; bump it when the prompt changes.
const historyPromptVersion = "2026-05-21.v5"

// llm talks to OpenAI-compatible /chat/completions endpoints. Base URLs come from the
// operator's environment, so private networks are allowed here.
type llm struct {
	apiKey, baseURL, model string
	client                 *http.Client
}

func (l llm) enabled() bool    { return l.apiKey != "" && l.baseURL != "" }
func (l llm) cacheKey() string { return l.model + ":" + historyPromptVersion }

func (l llm) post(ctx context.Context, body any, timeout time.Duration) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = cancelOnClose{resp.Body, cancel}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 240))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error { c.cancel(); return c.ReadCloser.Close() }

type historyEvent struct {
	Year      int           `json:"year"`
	YearLabel string        `json:"yearLabel"`
	Text      string        `json:"text"`
	Pages     []historyPage `json:"pages,omitempty"`
}

type historyPage struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Extract     string `json:"extract"`
}

type historyPick struct {
	DateLabel string `json:"dateLabel"`
	Items     []struct {
		Year    string `json:"year"`
		Display string `json:"display"`
	} `json:"items"`
}

func (l llm) optimizeHistory(ctx context.Context, dateLabel string, events []historyEvent) (historyPick, error) {
	var out historyPick
	if !l.enabled() {
		return out, errors.New("历史上的今天需要配置 AI_API_KEY 和 AI_BASE_URL")
	}
	user, _ := json.Marshal(map[string]any{
		"task": "从输入事件中按权重选出最多 5 条可展示事件，并改写为大陆简体中文墨水屏文案。",
		"editorial_policy": []string{
			"按综合权重筛选，不要把“中国大陆相关”作为绝对优先。",
			"权重最高：对世界历史、科技、战争、制度、国际组织、文化有长期影响的事件。",
			"权重较高：中国大陆历史、政治、科技、文化、社会发展、重大灾害相关事件。",
			"权重中等：对中国整体历史进程有影响，但地域较窄或解释成本较高的事件。",
			"权重较低：地方性短期新闻、地区政治细节、娱乐八卦、普通人物出生逝世。",
			"在权重接近时，优先保留时间跨度更分散、主题更多样的事件。",
			"不要为了凑满 5 条选择低价值事件；但有 5 条可读事件时应尽量填满。",
		},
		"writing_rules": []string{
			"必须使用中国大陆常用简体中文、中文标点和大陆常用译名。",
			"将繁体、台湾地区用语、港澳台译名尽量改为大陆常用表达。",
			"只基于输入事件改写，不编造输入中没有的信息。",
			"year 优先原样使用输入事件的 yearLabel，不使用负数年份。",
			"display 不包含年份，不使用“历史上的今天”等标题。",
			"display 目标长度 28-40 个汉字；不要压缩成标题，尽量保留事件的对象、动作和结果。",
			"如果原始信息不足 28 字也不要硬编；但不要故意写得过短。",
			"表述克制、准确，不煽情，不标题党。",
		},
		"dateLabel": dateLabel,
		"events":    events,
		"output_schema": map[string]any{
			"dateLabel": "string",
			"items":     []map[string]string{{"year": `string, use "1904" or "前221"`, "display": "string, simplified Chinese, no year, about 28-40 Chinese chars"}},
		},
	})
	resp, err := l.post(ctx, map[string]any{
		"model": l.model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是面向中国大陆用户的历史内容主编。只输出 JSON，不要 Markdown。内容用于 400x300 黑白墨水屏。"},
			{"role": "user", "content": string(user)},
		},
		"temperature":     0.2,
		"response_format": map[string]string{"type": "json_object"},
	}, 45*time.Second)
	if err != nil {
		return out, fmt.Errorf("AI 请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return out, fmt.Errorf("AI 响应读取失败: %w", err)
	}
	content := completionContent(body)
	if content == "" || !decodeFirstObject(content, &out) || len(out.Items) == 0 {
		slog.Warn("AI returned unusable history_today content", "promptVersion", historyPromptVersion, "body", truncate(string(body), 240))
		return out, errors.New("history_today AI 优化失败")
	}
	return out, nil
}

// completionContent accepts both plain JSON completions and SSE streams.
func completionContent(body []byte) string {
	type completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	parse := func(raw []byte) string {
		var c completion
		if json.Unmarshal(raw, &c) != nil || len(c.Choices) == 0 {
			return ""
		}
		if c.Choices[0].Message.Content != "" {
			return c.Choices[0].Message.Content
		}
		var b strings.Builder
		for _, ch := range c.Choices {
			b.WriteString(ch.Delta.Content)
		}
		return b.String()
	}
	text := bytes.TrimSpace(body)
	if s := parse(text); s != "" {
		return s
	}
	var b strings.Builder
	for _, payload := range ssePayloads(bytes.NewReader(text)) {
		b.WriteString(parse(payload))
	}
	return b.String()
}

func ssePayloads(r io.Reader) [][]byte {
	var out [][]byte
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if p, ok := strings.CutPrefix(line, "data:"); ok {
			if p = strings.TrimSpace(p); p != "" && p != "[DONE]" {
				out = append(out, []byte(p))
			}
		}
	}
	return out
}

// decodeFirstObject parses text as JSON, or else the first balanced {...} that parses.
func decodeFirstObject(text string, v any) bool {
	if json.Unmarshal([]byte(text), v) == nil {
		return true
	}
	for start := strings.IndexByte(text, '{'); start >= 0; {
		depth, inString, escaped := 0, false, false
		for i := start; i < len(text); i++ {
			ch := text[i]
			switch {
			case escaped:
				escaped = false
			case ch == '\\' && inString:
				escaped = true
			case ch == '"':
				inString = !inString
			case inString:
			case ch == '{':
				depth++
			case ch == '}':
				if depth--; depth == 0 {
					if json.Unmarshal([]byte(text[start:i+1]), v) == nil {
						return true
					}
					i = len(text)
				}
			}
		}
		next := strings.IndexByte(text[start+1:], '{')
		if next < 0 {
			break
		}
		start += next + 1
	}
	return false
}

const (
	ttsSourceRate = 24000
	maxTTSPCM     = 5 << 20
)

// synthesize streams pcm16 audio from an OpenAI-compatible endpoint; the assistant
// message is the text to read.
func (l llm) synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if !l.enabled() {
		return nil, errors.New("TTS_API_KEY 或 TTS_BASE_URL 未配置")
	}
	resp, err := l.post(ctx, map[string]any{
		"model":    l.model,
		"messages": []map[string]string{{"role": "user", "content": ""}, {"role": "assistant", "content": text}},
		"audio":    map[string]string{"format": "pcm16", "voice": voice},
		"stream":   true,
	}, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("TTS 请求失败: %w", err)
	}
	defer resp.Body.Close()
	var pcm []byte
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		payload, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "data:")
		if payload = strings.TrimSpace(payload); !ok || payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Delta struct {
					Audio struct {
						Data string `json:"data"`
					} `json:"audio"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return nil, fmt.Errorf("TTS 流解析失败: %w", err)
		}
		if chunk.Error != nil {
			return nil, fmt.Errorf("TTS 流错误: %s", truncate(chunk.Error.Message, 240))
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Audio.Data == "" {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(chunk.Choices[0].Delta.Audio.Data)
		if err != nil {
			return nil, fmt.Errorf("TTS 音频解码失败: %w", err)
		}
		if pcm = append(pcm, b...); len(pcm) > maxTTSPCM {
			return nil, errors.New("TTS 音频超过 5MB")
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("TTS 流读取失败: %w", err)
	}
	if len(pcm) == 0 || len(pcm)%2 != 0 {
		return nil, errors.New("TTS 音频为空或未按 16-bit 对齐")
	}
	return pcm, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
