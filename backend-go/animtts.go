package main

// Volcengine speech: explicit unidirectional streaming or Seed Audio create protocol.
// Streaming uses X-Api-Resource-Id and JSON chunks; create returns one JSON
// object with base64 audio. Configuration selects one protocol without fallback.

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
	Provider, Protocol, Model, URL, Key, ResourceID, Speaker, Format, Unit string
	SampleRate, SpeechRate, Retries                                        int
	Delivery                                                               string `json:",omitempty"`
}

func ttsConfigFromEnv() ttsConfig {
	n := func(k string, d int) int {
		v, e := strconv.Atoi(os.Getenv(k))
		if e != nil {
			return d
		}
		return v
	}
	c := ttsConfig{Protocol: env("ANIMATION_TTS_PROTOCOL", "unidirectional"), Model: env("ANIMATION_TTS_MODEL", "seed-audio-1.0"), Provider: env("ANIMATION_TTS_PROVIDER", "volcengine"), URL: env("ANIMATION_TTS_URL", "https://openspeech.bytedance.com/api/v3/tts/unidirectional"), Key: os.Getenv("ANIMATION_TTS_API_KEY"), ResourceID: env("ANIMATION_TTS_RESOURCE_ID", "seed-tts-2.0"), Speaker: os.Getenv("ANIMATION_TTS_SPEAKER"), Format: env("ANIMATION_TTS_FORMAT", "mp3"), Unit: env("ANIMATION_TTS_UNIT", "sentence"), SampleRate: n("ANIMATION_TTS_SAMPLE_RATE", 24000), SpeechRate: n("ANIMATION_TTS_SPEECH_RATE", 0), Retries: n("ANIMATION_TTS_RETRIES", 2)}
	if c.protocol() == "create" {
		c.Delivery = "classroom-v1"
	}
	return c
}

// configured reports whether live narration may run. Only sentence-level
// synthesis is implemented; any other unit is a configuration error, not a fallback.
func (c ttsConfig) protocol() string {
	if c.Protocol == "" {
		return "unidirectional"
	}
	return c.Protocol
}

func (c ttsConfig) timeout() time.Duration {
	if c.protocol() == "create" {
		return 300 * time.Second
	}
	return 60 * time.Second
}

func (c ttsConfig) configured() bool { return len(c.missing()) == 0 }
func (c ttsConfig) missing() []string {
	checks := map[string]bool{
		"ANIMATION_TTS_PROVIDER=volcengine": c.Provider == "volcengine",
		"ANIMATION_TTS_API_KEY":             c.Key != "",
		"ANIMATION_TTS_URL":                 c.URL != "",
		"ANIMATION_TTS_UNIT=sentence":       c.Unit == "sentence",
		"ANIMATION_TTS_FORMAT=mp3|wav":      c.Format == "mp3" || c.Format == "wav",
	}
	switch c.protocol() {
	case "create":
		checks["ANIMATION_TTS_MODEL=seed-audio-1.0"] = c.Model == "seed-audio-1.0"
	case "unidirectional":
		checks["ANIMATION_TTS_RESOURCE_ID"] = c.ResourceID != ""
		checks["ANIMATION_TTS_SPEAKER"] = c.Speaker != ""
	default:
		checks["ANIMATION_TTS_PROTOCOL=unidirectional|create"] = false
	}
	var missing []string
	for key, ok := range checks {
		if !ok {
			missing = append(missing, key)
		}
	}
	return missing
}

type ttsSubtitleWord struct {
	Text      string `json:"text"`
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
}
type ttsSentence struct {
	Text      string            `json:"text"`
	StartTime int               `json:"start_time"`
	EndTime   int               `json:"end_time"`
	Words     []ttsSubtitleWord `json:"words"`
}

type ttsWord struct {
	Word      string  `json:"word"`
	StartTime float64 `json:"startTime"`
	EndTime   float64 `json:"endTime"`
}
type ttsResult struct {
	OriginalDurationMS int
	Transcript         string
	Sentences          []ttsSentence
	Audio              []byte
	DurationMS         int
	Chunks             int
	Words              []ttsWord
	LogID              string
}

// ttsError omits credentials, request headers and raw provider messages.
type ttsError struct {
	Status, Code int
	Message      string
	Retryable    bool
}

func (e ttsError) Error() string {
	return fmt.Sprintf("语音合成失败（HTTP %d，code %d）：%s", e.Status, e.Code, e.Message)
}

func synthesizeSpeech(ctx context.Context, client *http.Client, c ttsConfig, text string, subtitles bool) (ttsResult, error) {
	if !c.configured() {
		return ttsResult{}, ttsError{Message: "语音配置不完整或协议不支持"}
	}
	safeClient := *client
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client = &safeClient
	audio := map[string]any{"format": c.Format, "sample_rate": c.SampleRate, "speech_rate": c.SpeechRate}
	if subtitles {
		audio["enable_subtitle"] = true
	}
	body := asJSON(map[string]any{"user": map[string]string{"uid": "edu-agent"}, "req_params": map[string]any{"text": text, "speaker": c.Speaker, "audio_params": audio}})
	if c.protocol() == "create" {
		prompt := "安静的录音环境，没有背景音乐或音效。一位女教师（成年女性，标准普通话，清晰自然，平稳讲解）逐字读出以下台词，不增删或改写，不读说明：" + string(asJSON(text))
		if c.Delivery == "classroom-v1" {
			prompt = "安静的录音环境，没有背景音乐或音效。一位女教师（成年女性，标准普通话），以亲切、自然、有好奇心的课堂语气讲解；提问时有自然语调起伏，重点适当强调，停顿从容，不夸张。逐字读出以下台词，不增删或改写，不读说明：" + string(asJSON(text))
		}
		bodyData := map[string]any{"model": c.Model, "text_prompt": prompt, "audio_config": audio, "watermark": map[string]any{}}
		if c.Speaker != "" {
			bodyData["references"] = []map[string]string{{"speaker": c.Speaker}}
			bodyData["text_prompt"] = "使用 @Audio1 的音色。" + prompt
		}
		body = asJSON(bodyData)
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(body))
	if e != nil {
		return ttsResult{}, ttsError{Message: "语音接口地址无效"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.Key)
	if c.protocol() == "unidirectional" {
		req.Header.Set("X-Api-Resource-Id", c.ResourceID)
	}
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
		return r, ttsError{Status: resp.StatusCode, Code: m.Code, Message: http.StatusText(resp.StatusCode), Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500}
	}
	if c.protocol() == "create" {
		return decodeSeedAudio(resp.Body, c.Format, r)
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
			return r, ttsError{Status: 200, Message: "无法解析音频流", Retryable: true}
		}
		if m.Code != 0 && m.Code != ttsDoneCode {
			return r, ttsError{Status: 200, Code: m.Code, Message: "供应商拒绝语音请求，请根据状态码核对权限与参数"}
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

// Seed Audio create returns one JSON object; its URL is never fetched.
// Word times are milliseconds here, unlike the streaming API's seconds.
func decodeSeedAudio(reader io.Reader, format string, r ttsResult) (ttsResult, error) {
	var reply struct {
		Code             int     `json:"code"`
		Audio            string  `json:"audio"`
		OriginalDuration float64 `json:"original_duration"`
		Subtitle         struct {
			Text      string        `json:"text"`
			Sentences []ttsSentence `json:"sentences"`
		} `json:"subtitle"`
	}
	if err := json.NewDecoder(io.LimitReader(reader, 32<<20)).Decode(&reply); err != nil {
		return r, ttsError{Status: 200, Message: "无法解析音频生成响应"}
	}
	if reply.Code != 0 {
		return r, ttsError{Status: 200, Code: reply.Code, Message: "供应商拒绝音频生成请求，请根据状态码核对权限与参数"}
	}
	if reply.Audio == "" {
		return r, ttsError{Status: 200, Message: "未返回内嵌音频，不使用临时下载链接替代"}
	}
	audio, err := base64.StdEncoding.DecodeString(reply.Audio)
	if err != nil {
		return r, ttsError{Status: 200, Message: "音频不是有效base64"}
	}
	ms, err := audioDurationMS(format, audio)
	if err != nil {
		return r, ttsError{Status: 200, Message: "无法解析生成音频的实际时长"}
	}
	r.Audio, r.DurationMS, r.Chunks = audio, ms, 1
	r.OriginalDurationMS = int(reply.OriginalDuration * 1000)
	r.Transcript, r.Sentences = reply.Subtitle.Text, reply.Subtitle.Sentences
	for _, sentence := range r.Sentences {
		for _, word := range sentence.Words {
			r.Words = append(r.Words, ttsWord{Word: word.Text, StartTime: float64(word.StartTime) / 1000, EndTime: float64(word.EndTime) / 1000})
		}
	}
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
