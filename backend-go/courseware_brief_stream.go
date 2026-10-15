package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type briefStreamKey struct{}

// Reply is the first JSON field. Decode only complete escape sequences, so the
// teacher sees text as the upstream model produces it, never raw JSON fragments.
var briefReplyStart = regexp.MustCompile(`^\s*\{\s*"reply"\s*:\s*"`)

func briefReplyPrefix(raw string) string {
	loc := briefReplyStart.FindStringIndex(raw)
	if loc == nil {
		return ""
	}
	value := raw[loc[1]:]
	end := 0
	for end < len(value) {
		if value[end] == '"' {
			break
		}
		if value[end] == '\\' {
			length := 2
			if end+1 < len(value) && value[end+1] == 'u' {
				length = 6
			}
			if end+length > len(value) {
				break
			}
			if length == 6 {
				code, _ := strconv.ParseUint(value[end+2:end+6], 16, 16)
				if utf16.IsSurrogate(rune(code)) && code < 0xdc00 {
					if end+12 > len(value) {
						break
					}
					length = 12
				}
			}
			end += length
		} else {
			if !utf8.FullRuneInString(value[end:]) {
				break
			}
			_, size := utf8.DecodeRuneInString(value[end:])
			end += size
		}
	}
	var text string
	if json.Unmarshal([]byte(`"`+value[:end]+`"`), &text) != nil {
		return ""
	}
	return text
}

func (s *Server) postBriefStream(ctx context.Context, client *http.Client, endpoint string, payload any, key string, emit func(string) error) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(asJSON(payload)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := client.Do(request)
	if err != nil {
		return nil, providerTransportError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, providerCallError{Status: response.StatusCode, Temporary: response.StatusCode == 429 || response.StatusCode >= 500}
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		return nil, fail(502, "对话服务没有返回流式内容，请重试；需求已保留")
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 1<<20))
	scanner.Buffer(make([]byte, 4096), 128<<10)
	var content strings.Builder
	var usage json.RawMessage
	finish, completed, emitted := "", false, ""
	var data []string
	consume := func() error {
		if len(data) == 0 {
			return nil
		}
		line := strings.Join(data, "\n")
		data = nil
		if line == "[DONE]" {
			completed = true
			return nil
		}
		var frame struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage json.RawMessage `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(line), &frame) != nil || (len(frame.Error) > 0 && string(frame.Error) != "null") {
			return fail(502, "对话服务返回异常，请重试；需求已保留")
		}
		if len(frame.Usage) > 0 && string(frame.Usage) != "null" {
			usage = frame.Usage
		}
		for _, choice := range frame.Choices {
			content.WriteString(choice.Delta.Content)
			if content.Len() > 24000 {
				return fail(422, "需求回复过长，请重试；需求已保留")
			}
			if choice.Finish != "" {
				finish = choice.Finish
			}
		}
		text := briefReplyPrefix(content.String())
		if len([]rune(text)) > 1600 {
			return fail(422, "需求回复过长，请重试；需求已保留")
		}
		if strings.HasPrefix(text, emitted) && len(text) > len(emitted) {
			if err := emit(strings.TrimPrefix(text, emitted)); err != nil {
				return err
			}
			emitted = text
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := consume(); err != nil {
				return nil, err
			}
			if completed {
				break
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, providerTransportError(ctx, err)
	}
	if !completed || finish == "" {
		return nil, fail(502, "对话连接中断，请重试；已填写的需求保留")
	}
	if finish != "stop" && finish != "length" {
		return nil, fail(422, "本次需求回复未完成，请调整描述后重试")
	}
	if len(usage) == 0 {
		usage = json.RawMessage(`{}`)
	}
	return asJSON(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": content.String()}}}, "usage": usage}), nil
}

func writeBriefEvent(w http.ResponseWriter, event string, data any) error {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, asJSON(data)); err != nil {
		return err
	}
	return http.NewResponseController(w).Flush()
}
