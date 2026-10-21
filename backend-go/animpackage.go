package main

// Single-file offline HTML package: player, validated scene data and audio are
// inlined; CSP forbids every network request (docs/animation-system-design.md §7.1).

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"html"
	"regexp"
	"strings"
)

const animRendererVersion = "direct-html-v2"
const animDirectBaseCSS = "html,body{margin:0;width:1280px;height:720px;overflow:hidden}*,*::before,*::after{box-sizing:border-box;animation:none!important;transition:none!important}"

//go:embed animplayer/direct.js
var animDirectJS string

//go:embed animplayer/direct-boot.js
var animDirectBoot string

//go:embed animplayer/drawing.js
var animDrawingJS string

//go:embed animplayer/functions.js
var teachingFunctionsJS string

//go:embed animplayer/player.js
var animPlayerJS string

//go:embed animplayer/player.css
var animPlayerCSS string

// Voice sources. Only "tts" is real narration; the others are labelled on screen.
const (
	animVoiceTTS       = "tts"
	animVoiceEstimated = "estimated" // sample mode: no audio, durations estimated
	animVoiceTone      = "tone"      // test tones for player verification, not speech
)

type animPackageInput struct {
	Science  map[string]scienceRender
	Picture  *TeachingPicture
	Script   AnimScript
	Timeline AnimTimeline
	Audio    map[string][]byte // sentence ID -> audio bytes
	Format   string            // mp3 or wav
	Voice    string
	Fixture  bool // fixed sample script, not model output
}

func animLabels(in animPackageInput) []map[string]any {
	var labels []map[string]any
	if in.Fixture {
		labels = append(labels, map[string]any{"text": "固定样例脚本，非AI生成结果", "warn": true})
	} else {
		labels = append(labels, map[string]any{"text": "AI生成内容"})
	}
	switch in.Voice {
	case animVoiceTTS:
		labels = append(labels, map[string]any{"text": "AI合成语音"})
	case animVoiceEstimated:
		label := "预览模式：无真实语音，时长为估算"
		if in.Fixture {
			label = "样例模式：无真实语音，时长为估算"
		}
		labels = append(labels, map[string]any{"text": label, "warn": true})
	case animVoiceTone:
		labels = append(labels, map[string]any{"text": "测试音：非真实语音", "warn": true})
	}
	return labels
}

func cspHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return "'sha256-" + base64.StdEncoding.EncodeToString(h[:]) + "'"
}

// The HTML parser normalizes CRLF to LF inside inline elements, so hashes must be
// computed over LF text or a CRLF checkout would silently block the player.
var animPlayerJSLF, animPlayerCSSLF = strings.ReplaceAll(strings.ReplaceAll(teachingFunctionsJS, "export default TeachingFunctions;", "")+"\n"+animDrawingJS+"\n"+animDirectJS+"\n"+animPlayerJS, "\r\n", "\n"), strings.ReplaceAll(animPlayerCSS, "\r\n", "\n")

func buildAnimPackage(in animPackageInput) []byte {
	mime := map[string]string{"mp3": "audio/mpeg", "wav": "audio/wav"}[in.Format]
	audio := map[string]string{}
	for id, b := range in.Audio {
		audio[id] = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
	}
	labels := animLabels(in)
	// json.Marshal escapes <, > and &, so the data cannot close its script element.
	boot := strings.ReplaceAll(animDirectBoot, "\r\n", "\n")
	data := string(asJSON(map[string]any{"script": in.Script, "timeline": in.Timeline, "audio": audio, "voice": in.Voice, "labels": labels, "renderer": animRendererVersion, "picture": in.Picture, "science": in.Science, "direct_boot": boot, "direct_base_css": animDirectBaseCSS}))
	csp := "default-src 'none'; script-src " + cspHash(animPlayerJSLF) + "; style-src " + cspHash(animPlayerCSSLF) + "; img-src data:; media-src data:; font-src data:; base-uri 'none'; form-action 'none';"
	if in.Script.Archetype == "html.direct@1" {
		scripts, styles := cspHash(animPlayerJSLF)+" "+cspHash(boot), cspHash(animPlayerCSSLF)+" "+cspHash(animDirectBaseCSS)
		for _, scene := range in.Script.Scenes {
			for _, m := range regexp.MustCompile("(?is)<script[^>]*>(.*?)</script\\s*>").FindAllStringSubmatch(strings.ReplaceAll(scene.HTML, "\r\n", "\n"), -1) {
				scripts += " " + cspHash(m[1])
			}
			for _, m := range regexp.MustCompile("(?is)<style[^>]*>(.*?)</style\\s*>").FindAllStringSubmatch(strings.ReplaceAll(scene.HTML, "\r\n", "\n"), -1) {
				styles += " " + cspHash(m[1])
			}
		}
		csp = "default-src 'none'; script-src " + scripts + "; style-src " + styles + "; style-src-attr 'unsafe-inline'; frame-src data:; img-src data:; media-src data:; font-src data:; base-uri 'none'; form-action 'none';"
	}
	var tags strings.Builder
	for _, l := range labels {
		class := "tag"
		if l["warn"] == true {
			class += " warn"
		}
		tags.WriteString(`<span class="` + class + `">` + html.EscapeString(l["text"].(string)) + `</span>`)
	}
	title := html.EscapeString(in.Script.Title)
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">`)
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="` + csp + `">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<meta name="generator" content="edu-agent ` + animRendererVersion + `">`)
	// Implicit AIGC marking; whether this meets each export format's labelling rules is still to be verified (§9).
	if !in.Fixture || in.Voice == animVoiceTTS {
		b.WriteString(`<meta name="aigc" content="` + map[bool]string{true: "AI合成语音", false: "AI生成内容"}[in.Fixture] + `">`)
	}
	b.WriteString(`<meta name="aigc-voice" content="` + in.Voice + `">`)
	b.WriteString(`<title>` + title + `</title><style>` + animPlayerCSSLF + `</style></head><body>`)
	b.WriteString(`<header><h1>` + title + `</h1>` + tags.String() + `</header>`)
	b.WriteString(`<main><div class="frame"><svg id="stage" viewBox="0 0 1280 720" role="img" aria-label="` + title + ` 动画画面"></svg><div id="subtitle" role="region" aria-label="字幕" aria-live="off"></div></div></main>`)
	b.WriteString(`<nav aria-label="播放控制"><button id="play" type="button" aria-pressed="false">播放</button><button id="continue" type="button" hidden>继续</button>`)
	b.WriteString(`<button id="prev" type="button">上一幕</button><button id="next" type="button">下一幕</button><button id="replay" type="button">回放</button>`)
	b.WriteString(`<input id="seek" type="range" min="0" step="10" value="0" aria-label="播放进度"><span id="time"></span><span id="scene" class="tag"></span>`)
	b.WriteString(`<button id="mode" type="button" aria-pressed="false">分步播放</button><button id="subs" type="button" aria-pressed="true">字幕</button></nav>`)
	b.WriteString(`<script type="application/json" id="anim-data">` + data + `</script>`)
	b.WriteString(`<script>` + animPlayerJSLF + `</script></body></html>`)
	return []byte(b.String())
}
