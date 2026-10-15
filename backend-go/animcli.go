package main

// Developer entry points for the animation engine before it is wired into tasks.
// Scripts given here are never model output, so packages are always labelled as fixed samples.
// They read no database and write only to the given local paths.

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const animCLIUsage = "edu-server --anim-package <script.json> <out.html> [estimated|tone|tts] | --tts-probe [out-dir] [text]"

// runAnimCLI handles animation subcommands; it returns false for other arguments.
func runAnimCLI(ctx context.Context, args []string) (bool, error) {
	if len(args) < 1 {
		return false, nil
	}
	switch args[0] {
	case "--anim-package":
		if len(args) < 3 || len(args) > 4 {
			return true, fmt.Errorf("usage: %s", animCLIUsage)
		}
		voice := animVoiceEstimated
		if len(args) == 4 {
			voice = args[3]
		}
		return true, animPackageCommand(ctx, args[1], args[2], voice)
	case "--tts-probe":
		dir, text := "../work/tts-probe", "一次函数 y 等于 k x 加 b 中，k 等于负 3 时，直线从左到右下降，而且比 k 等于 1 时更陡。"
		if len(args) > 1 {
			dir = args[1]
		}
		if len(args) > 2 {
			text = strings.Join(args[2:], " ")
		}
		return true, ttsProbeCommand(ctx, dir, text)
	}
	return false, nil
}

func animPackageCommand(ctx context.Context, scriptPath, out, voice string) error {
	raw, e := os.ReadFile(scriptPath)
	if e != nil {
		return e
	}
	sc, problems := decodeAnimScript(raw)
	if len(problems) > 0 {
		return fmt.Errorf("作者数据未通过校验：\n- %s", strings.Join(problems, "\n- "))
	}
	in := animPackageInput{Script: sc, Format: "wav", Voice: voice, Fixture: true, Audio: map[string][]byte{}}
	ms := map[string]int{}
	cfg := ttsConfigFromEnv()
	if voice == animVoiceTTS {
		if !cfg.configured() {
			return fmt.Errorf("语音未配置，缺少：%s", strings.Join(cfg.missing(), ", "))
		}
		in.Format = cfg.Format
	}
	client := &http.Client{Timeout: 60 * time.Second}
	for _, s := range sc.Scenes {
		for _, n := range s.Narration {
			switch voice {
			case animVoiceEstimated:
				ms[n.ID] = estimateSpeechMS(n.Speech)
			case animVoiceTone:
				ms[n.ID] = estimateSpeechMS(n.Speech)
				in.Audio[n.ID] = toneWAV(ms[n.ID], 330+float64(len(ms)%5)*40)
			case animVoiceTTS:
				r, attempts, e := synthesizeWithRetry(ctx, client, cfg, n.Speech)
				if e != nil {
					return fmt.Errorf("幕%s句子%s语音合成失败（尝试%d次）：%w", s.ID, n.ID, attempts, e)
				}
				ms[n.ID], in.Audio[n.ID] = r.DurationMS, r.Audio
				fmt.Printf("  %s/%s %d ms, %d chunks, attempts %d\n", s.ID, n.ID, r.DurationMS, r.Chunks, attempts)
			default:
				return fmt.Errorf("voice须为estimated、tone或tts")
			}
		}
	}
	tl, problems := buildAnimTimeline(sc, ms)
	if len(problems) > 0 {
		return fmt.Errorf("时间轴未通过校验：\n- %s", strings.Join(problems, "\n- "))
	}
	in.Timeline = tl
	if e := os.MkdirAll(filepath.Dir(out), 0o755); e != nil {
		return e
	}
	if e := os.WriteFile(out, buildAnimPackage(in), 0o644); e != nil {
		return e
	}
	fmt.Printf("wrote %s: %d scenes, %.1f s (auto play), voice=%s\n", out, len(tl.Scenes), float64(tl.TotalMS)/1000, voice)
	for _, w := range tl.Warnings {
		fmt.Println("warning:", w)
	}
	return nil
}

// toneWAV is a quiet sine test tone for verifying audio-driven timing; it is never narration.
func toneWAV(ms int, hz float64) []byte {
	const rate = 16000
	n := rate * ms / 1000
	b := make([]byte, 44+2*n)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+2*n))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], rate)
	binary.LittleEndian.PutUint32(b[28:], rate*2)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(2*n))
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint16(b[44+2*i:], uint16(int16(2000*math.Sin(2*math.Pi*hz*float64(i)/rate))))
	}
	return b
}

// ttsProbeCommand records real-account capability evidence (§5.3) without printing credentials.
func ttsProbeCommand(ctx context.Context, dir, text string) error {
	cfg := ttsConfigFromEnv()
	if !cfg.configured() {
		return fmt.Errorf("语音未配置，缺少：%s（填写.env后重试）", strings.Join(cfg.missing(), ", "))
	}
	if e := os.MkdirAll(dir, 0o755); e != nil {
		return e
	}
	start := time.Now()
	r, e := synthesizeSpeech(ctx, &http.Client{Timeout: 60 * time.Second}, cfg, text, true)
	if e != nil {
		return e
	}
	stamp := time.Now().Format("20060102-150405")
	audioPath := filepath.Join(dir, "probe-"+stamp+"."+cfg.Format)
	if e := os.WriteFile(audioPath, r.Audio, 0o644); e != nil {
		return e
	}
	lastEnd := 0.0
	for _, w := range r.Words {
		lastEnd = math.Max(lastEnd, w.EndTime)
	}
	report := map[string]any{"date": time.Now().Format(time.RFC3339), "provider": cfg.Provider, "endpoint": cfg.URL, "resource_id": cfg.ResourceID, "speaker": cfg.Speaker, "format": cfg.Format, "sample_rate": cfg.SampleRate, "speech_rate": cfg.SpeechRate, "text": text, "audio_file": filepath.Base(audioPath), "audio_bytes": len(r.Audio), "decoded_ms": r.DurationMS, "chunks": r.Chunks, "elapsed_ms": time.Since(start).Milliseconds(), "word_timestamps": len(r.Words), "last_word_end_ms": int(lastEnd * 1000), "words": r.Words, "log_id": r.LogID}
	reportPath := filepath.Join(dir, "probe-"+stamp+".json")
	if e := os.WriteFile(reportPath, asJSON(report), 0o644); e != nil {
		return e
	}
	fmt.Printf("audio %s (%d ms decoded, %d chunks, %d word timestamps)\nreport %s\n", audioPath, r.DurationMS, r.Chunks, len(r.Words), reportPath)
	return nil
}
