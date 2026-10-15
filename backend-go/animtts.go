package main

// Volcengine Doubao TTS, V3 HTTP Chunked unidirectional streaming.
// POST {url} with X-Api-Key / X-Api-Resource-Id; the body streams JSON objects
// carrying base64 audio in "data" until code 20000000.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

const ttsDoneCode = 20000000

type ttsConfig struct {
	Provider, URL, Key, ResourceID, Speaker, Format, Unit string
	SampleRate, SpeechRate, Retries                       int
}

func ttsConfigFromEnv() ttsConfig {
	n := func(k string, d int) int {
		v, e := strconv.Atoi(os.Getenv(k))
		if e != nil {
			return d
		}
		return v
	}
	return ttsConfig{Provider: env("ANIMATION_TTS_PROVIDER", "volcengine"), URL: env("ANIMATION_TTS_URL", "https://openspeech.bytedance.com/api/v3/tts/unidirectional"), Key: os.Getenv("ANIMATION_TTS_API_KEY"), ResourceID: env("ANIMATION_TTS_RESOURCE_ID", "seed-tts-2.0"), Speaker: os.Getenv("ANIMATION_TTS_SPEAKER"), Format: env("ANIMATION_TTS_FORMAT", "mp3"), Unit: env("ANIMATION_TTS_UNIT", "sentence"), SampleRate: n("ANIMATION_TTS_SAMPLE_RATE", 24000), SpeechRate: n("ANIMATION_TTS_SPEECH_RATE", 0), Retries: n("ANIMATION_TTS_RETRIES", 2)}
}

// configured reports whether live narration may run. Only sentence-level
// synthesis is implemented; any other unit is a configuration error, not a fallback.
func (c ttsConfig) configured() bool {
	return c.Provider == "volcengine" && c.URL != "" && c.Key != "" && c.ResourceID != "" && c.Speaker != "" && c.Unit == "sentence" && (c.Format == "mp3" || c.Format == "wav")
}
func (c ttsConfig) missing() []string {
	var m []string
	for k, ok := range map[string]bool{"ANIMATION_TTS_API_KEY": c.Key != "", "ANIMATION_TTS_SPEAKER": c.Speaker != "", "ANIMATION_TTS_URL": c.URL != "", "ANIMATION_TTS_RESOURCE_ID": c.ResourceID != "", "ANIMATION_TTS_UNIT=sentence": c.Unit == "sentence", "ANIMATION_TTS_FORMAT=mp3|wav": c.Format == "mp3" || c.Format == "wav", "ANIMATION_TTS_PROVIDER=volcengine": c.Provider == "volcengine"} {
		if !ok {
			m = append(m, k)
		}
	}
	return m
}

type ttsWord struct {
	Word      string  `json:"word"`
	StartTime float64 `json:"startTime"`
	EndTime   float64 `json:"endTime"`
}
type ttsResult struct {
	Audio      []byte
	DurationMS int
	Chunks     int
	Words      []ttsWord
	LogID      string
}

// ttsError never includes request headers, so the key cannot leak into logs.
type ttsError struct {
	Status, Code int
	Message      string
	Retryable    bool
}

func (e ttsError) Error() string {
	return fmt.Sprintf("语音合成失败（HTTP %d，code %d）：%s", e.Status, e.Code, e.Message)
}

func synthesizeSpeech(ctx context.Context, client *http.Client, c ttsConfig, text string, subtitles bool) (ttsResult, error) {
	audio := map[string]any{"format": c.Format, "sample_rate": c.SampleRate, "speech_rate": c.SpeechRate}
	if subtitles {
		audio["enable_subtitle"] = true
	}
	body := asJSON(map[string]any{"user": map[string]string{"uid": "edu-agent"}, "req_params": map[string]any{"text": text, "speaker": c.Speaker, "audio_params": audio}})
	req, e := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(body))
	if e != nil {
		return ttsResult{}, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.Key)
	req.Header.Set("X-Api-Resource-Id", c.ResourceID)
	req.Header.Set("X-Api-Request-Id", newID())
	resp, e := client.Do(req)
	if e != nil {
		return ttsResult{}, ttsError{Message: providerTransportError(ctx, e).Error(), Retryable: ctx.Err() == nil}
	}
	defer resp.Body.Close()
	r := ttsResult{LogID: resp.Header.Get("X-Tt-Logid")}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		var m struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		json.Unmarshal(b, &m)
		if m.Message == "" {
			m.Message = http.StatusText(resp.StatusCode)
		}
		return r, ttsError{Status: resp.StatusCode, Code: m.Code, Message: m.Message, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500}
	}
	d := json.NewDecoder(io.LimitReader(resp.Body, 32<<20))
	for {
		var m struct {
			Code     int    `json:"code"`
			Message  string `json:"message"`
			Data     string `json:"data"`
			Sentence *struct {
				Words []ttsWord `json:"words"`
			} `json:"sentence"`
		}
		if e := d.Decode(&m); e != nil {
			if errors.Is(e, io.EOF) {
				return r, ttsError{Status: 200, Message: "音频流在结束标记前中断", Retryable: true}
			}
			return r, ttsError{Status: 200, Message: "无法解析音频流：" + e.Error(), Retryable: true}
		}
		if m.Code != 0 && m.Code != ttsDoneCode {
			return r, ttsError{Status: 200, Code: m.Code, Message: m.Message}
		}
		if m.Data != "" {
			b, e := base64.StdEncoding.DecodeString(m.Data)
			if e != nil {
				return r, ttsError{Status: 200, Message: "音频分片不是有效base64", Retryable: true}
			}
			r.Audio, r.Chunks = append(r.Audio, b...), r.Chunks+1
		}
		if m.Sentence != nil {
			r.Words = append(r.Words, m.Sentence.Words...)
		}
		if m.Code == ttsDoneCode {
			break
		}
	}
	if len(r.Audio) == 0 {
		return r, ttsError{Status: 200, Message: "未返回音频", Retryable: true}
	}
	ms, e := audioDurationMS(c.Format, r.Audio)
	if e != nil {
		return r, ttsError{Status: 200, Message: e.Error(), Retryable: true}
	}
	r.DurationMS = ms
	return r, nil
}

// synthesizeWithRetry is the only failure path: bounded retries of the same
// unit and strategy, then an explicit error. It never switches strategy.
func synthesizeWithRetry(ctx context.Context, client *http.Client, c ttsConfig, text string) (ttsResult, int, error) {
	var last error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ttsResult{}, attempt, ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		r, e := synthesizeSpeech(ctx, client, c, text, false)
		if e == nil {
			return r, attempt + 1, nil
		}
		last = e
		var te ttsError
		if !errors.As(e, &te) || !te.Retryable {
			return r, attempt + 1, e
		}
	}
	return ttsResult{}, c.Retries + 1, last
}

// audioDurationMS decodes the actual duration; the timeline never trusts estimates for live audio.
func audioDurationMS(format string, b []byte) (int, error) {
	switch format {
	case "mp3":
		return mp3DurationMS(b)
	case "wav":
		if len(b) < 44 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
			return 0, errors.New("WAV头无效")
		}
		byteRate := 0
		for i := 12; i+8 <= len(b); {
			id, size := string(b[i:i+4]), int(binary.LittleEndian.Uint32(b[i+4:i+8]))
			if id == "fmt " && i+20 <= len(b) {
				byteRate = int(binary.LittleEndian.Uint32(b[i+16 : i+20]))
			}
			if id == "data" && byteRate > 0 {
				return int(int64(min(size, len(b)-i-8)) * 1000 / int64(byteRate)), nil
			}
			i += 8 + size + size%2
		}
		return 0, errors.New("WAV缺少fmt或data块")
	}
	return 0, fmt.Errorf("不支持的音频格式%s", format)
}

var (
	mp3Bitrates = [2][16]int{
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}, // MPEG-1 Layer III
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},     // MPEG-2/2.5 Layer III
	}
	mp3Rates = map[byte][3]int{3: {44100, 48000, 32000}, 2: {22050, 24000, 16000}, 0: {11025, 12000, 8000}}
)

// mp3DurationMS walks Layer III frame headers, skipping ID3v2 and the Xing/Info frame.
func mp3DurationMS(b []byte) (int, error) {
	i := 0
	if len(b) >= 10 && string(b[:3]) == "ID3" {
		i = 10 + (int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f))
	}
	var samples, frames int64
	rate := 0
	for i+4 <= len(b) {
		h := b[i : i+4]
		ver, layer := (h[1]>>3)&3, (h[1]>>1)&3
		rates, ok := mp3Rates[ver]
		if h[0] != 0xff || h[1]&0xe0 != 0xe0 || layer != 1 || !ok || h[2]>>2&3 == 3 || h[2]>>4 == 0 || h[2]>>4 == 15 {
			if frames == 0 && rate == 0 {
				i++ // still searching for the first frame
				continue
			}
			break
		}
		table, perFrame := 0, int64(1152)
		if ver != 3 {
			table, perFrame = 1, 576
		}
		kbps, sr, pad := mp3Bitrates[table][h[2]>>4], rates[h[2]>>2&3], int(h[2]>>1&1)
		size := int(perFrame/8)*kbps*1000/sr + pad
		frame := b[i:min(i+size, len(b))]
		first := frames == 0 && rate == 0
		rate, i = sr, i+size
		if first && (bytes.Contains(frame, []byte("Xing")) || bytes.Contains(frame, []byte("Info"))) {
			continue // VBR/encoder header frame carries no audio
		}
		samples, frames = samples+perFrame, frames+1
	}
	if frames == 0 || rate == 0 {
		return 0, errors.New("无法解析MP3音频帧")
	}
	return int(samples * 1000 / int64(rate)), nil
}
