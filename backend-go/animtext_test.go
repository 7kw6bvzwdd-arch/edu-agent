package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAnimTextSentenceBindings(t *testing.T) {
	raw, err := os.ReadFile("../fixtures/animation-v2-lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		cues    []string
		problem string
	}{
		{"valid", []string{"intro-n0", "intro-n1"}, ""},
		{"missing-item", []string{"intro-n0"}, "item_sentences"},
		{"foreign-sentence", []string{"intro-n0", "evaporation-n0"}, "本幕"},
		{"backwards", []string{"intro-n1", "intro-n0"}, "顺序"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatal(err)
			}
			object := data["scenes"].([]any)[0].(map[string]any)["objects"].([]any)[0].(map[string]any)
			object["items"] = []string{"观察现象", "提出问题"}
			object["item_sentences"] = tc.cues
			_, problems := decodeAnimScript(asJSON(data))
			got := strings.Join(problems, ";")
			if tc.problem == "" && len(problems) > 0 || tc.problem != "" && !strings.Contains(got, tc.problem) {
				t.Fatalf("problems=%s", got)
			}
		})
	}
}

func TestAnimTextGeneratedBindings(t *testing.T) {
	raw, err := os.ReadFile("../fixtures/animation-v2-lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	var script AnimScript
	if err = json.Unmarshal(raw, &script); err != nil {
		t.Fatal(err)
	}
	if p := animTextCueProblems(script, AnimScript{}); len(p) == 0 {
		t.Fatal("new model output without text cues must request repair")
	}
	if p := animTextCueProblems(script, script); len(p) != 0 {
		t.Fatal("unchanged historical objects must remain usable", p)
	}
	var changed AnimScript
	if err = json.Unmarshal(raw, &changed); err != nil {
		t.Fatal(err)
	}
	changed.Scenes[0].Objects[0].Text = "新关键词"
	if p := animTextCueProblems(changed, script); len(p) != 1 {
		t.Fatal("changed object must bind cues, other historical scenes remain intact", p)
	}
	if err = json.Unmarshal(raw, &changed); err != nil {
		t.Fatal(err)
	}
	changed.Scenes[0].Narration[0].Display = "重新讲解"
	if p := animTextCueProblems(changed, script); len(p) == 0 {
		t.Fatal("changed narration must request new bindings")
	}

	for i := range script.Scenes {
		s := &script.Scenes[i]
		for j := range s.Objects {
			o := &s.Objects[j]
			for k := range o.Items {
				o.ItemSentences = append(o.ItemSentences, s.Narration[k*len(s.Narration)/len(o.Items)].ID)
			}
		}
	}
	if p := animTextCueProblems(script, AnimScript{}); len(p) != 0 {
		t.Fatal(p)
	}
}

func TestAnimTextParallelPanels(t *testing.T) {
	raw, err := os.ReadFile("../fixtures/animation-v2-lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	var sc AnimScript
	if err = json.Unmarshal(raw, &sc); err != nil {
		t.Fatal(err)
	}
	scene := &sc.Scenes[0]
	scene.Objects = []AnimObject{
		{ID: "examples", Type: "comparison", Slot: "main", Text: "新例句的判断", Items: []string{"弯弯的月亮像小船", "圆圆的月亮像玉盘"}, ItemSentences: []string{scene.Narration[0].ID, scene.Narration[1].ID}},
		{ID: "tips", Type: "card", Slot: "main", Text: "思考方向", Items: []string{"先找本体和喻体"}, ItemSentences: []string{scene.Narration[1].ID}},
	}
	// A comparison and a short prompt are legitimate when explicitly cued.
	// Composition quality belongs to guidance/visual review, not object counts.
	for i := range sc.Scenes {
		for j := range sc.Scenes[i].Objects {
			o := &sc.Scenes[i].Objects[j]
			if len(o.ItemSentences) == 0 {
				for range o.Items {
					o.ItemSentences = append(o.ItemSentences, sc.Scenes[i].Narration[0].ID)
				}
			}
		}
	}
	if p := animTextCueProblems(sc, AnimScript{}); len(p) != 0 {
		t.Fatal("valid cued comparison must not be rejected by a universal panel count", p)
	}
	if p := animTextCueProblems(sc, sc); len(p) != 0 {
		t.Fatal("historical plans remain playable", p)
	}
}

// This optional preview reuses original audio and validates the author contract.
func TestAnimVisualCompositionPreview(t *testing.T) {
	path := os.Getenv("ANIM_VISUAL_PREVIEW")
	if path == "" {
		t.Skip("requires explicit local graphic composition preview")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), "id=\"anim-data\">", 2)
	if len(parts) != 2 {
		t.Fatal("preview data missing")
	}
	var stored map[string]json.RawMessage
	if err = json.Unmarshal([]byte(strings.SplitN(parts[1], "</script>", 2)[0]), &stored); err != nil {
		t.Fatal(err)
	}
	var sc AnimScript
	var tl AnimTimeline
	if err = json.Unmarshal(stored["script"], &sc); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(stored["timeline"], &tl); err != nil {
		t.Fatal(err)
	}
	if p := validateAnimScript(sc); len(p) > 0 {
		t.Fatal(p)
	}
	if p := animVisualProblems(sc, TaskInput{Topic: sc.Title}); len(p) > 0 {
		t.Fatal(p)
	}
	if p := animTextCueProblems(sc, AnimScript{}); len(p) > 0 {
		t.Fatal(p)
	}
	durations := map[string]int{}
	for _, scene := range tl.Scenes {
		for _, n := range scene.Sentences {
			durations[n.ID] = n.End - n.Start
		}
	}
	computed, problems := buildAnimTimeline(sc, durations)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	for i := range computed.Scenes {
		computed.Scenes[i].Subtitles = tl.Scenes[i].Subtitles
	}
	if !reflect.DeepEqual(computed, tl) {
		t.Fatalf("preview must use program-derived timeline: computed=%s stored=%s", asJSON(computed), asJSON(tl))
	}
}

// This package is a controlled layout/test-tone sample, never labelled AI output.
func TestAnimTextPresentationPackage(t *testing.T) {
	script := AnimScript{Version: animScriptVersion, Title: "文字布局逐句呈现样例", Archetype: "lesson.explain@1", Representative: "timeline"}
	durations := map[string]int{}
	audio := map[string][]byte{}
	for _, kind := range []string{"card", "comparison", "timeline", "relationship", "illustration"} {
		scene := AnimScene{ID: kind, Title: "从结构观察性质", Intent: "验证逐句呈现与布局", Holds: []AnimHold{}}
		obj := AnimObject{ID: "ideas", Type: kind, Slot: "main", Text: "结构不同，性质不同", Items: []string{"金刚石：空间网状结构", "石墨：层状结构", "碳原子的排列方式不同"}}
		if kind == "illustration" {
			scene.Title = "观察水的三种状态"
			obj.Text = "三态结构示意"
			obj.Kind = "water_states"
			obj.Items = []string{"固态：排列有序", "液态：能够流动", "气态：间隔较大"}
		}
		for i, suffix := range []string{"a", "b", "c"} {
			id := kind + "-" + suffix
			obj.ItemSentences = append(obj.ItemSentences, id)
			scene.Narration = append(scene.Narration, AnimSentence{ID: id, Display: obj.Items[i], Speech: []string{"先观察第一项。", "再观察第二项。", "最后联系起来比较。"}[i]})
			durations[id] = 2400
			audio[id] = toneWAV(2400, 330)
		}
		scene.Objects = []AnimObject{obj}
		scene.Actions = []AnimAction{{ID: "reveal", Op: "show", Object: obj.ID, At: AnimTrigger{Sentence: obj.ItemSentences[0]}, DurationMS: 200}}
		script.Scenes = append(script.Scenes, scene)
	}
	if p := validateAnimScript(script); len(p) > 0 {
		t.Fatal(p)
	}
	timeline, p := buildAnimTimeline(script, durations)
	if len(p) > 0 {
		t.Fatal(p)
	}
	out := os.Getenv("ANIM_TEXT_PREVIEW")
	if out == "" {
		out = filepath.Join(t.TempDir(), "text-layout.html")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, buildAnimPackage(animPackageInput{Script: script, Timeline: timeline, Voice: animVoiceTone, Format: "wav", Fixture: true, Audio: audio}), 0600); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ANIM_TEXT_PREVIEW") != "" {
		for _, name := range []string{"lesson-layout", "illustrated-layout"} {
			raw, err := os.ReadFile("../fixtures/animation-v2-" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var stress AnimScript
			if err = json.Unmarshal(raw, &stress); err != nil {
				t.Fatal(err)
			}
			if name == "lesson-layout" {
				for _, kind := range []string{"comparison", "timeline", "relationship"} {
					scene := stress.Scenes[0]
					scene.ID = "stress-" + kind
					scene.Title = "多列长文本布局"
					scene.Objects = append([]AnimObject(nil), scene.Objects...)
					for j := range scene.Objects {
						scene.Objects[j].Type = kind
						scene.Objects[j].ItemSentences = nil
					}
					stress.Scenes = append(stress.Scenes, scene)
				}
			}
			ms := map[string]int{}
			audio := map[string][]byte{}
			for i := range stress.Scenes {
				scene := &stress.Scenes[i]
				for _, n := range scene.Narration {
					ms[n.ID] = 3000
					audio[n.ID] = toneWAV(3000, 330)
				}
				for j := range scene.Objects {
					o := &scene.Objects[j]
					for k := range o.Items {
						o.ItemSentences = append(o.ItemSentences, scene.Narration[k*len(scene.Narration)/len(o.Items)].ID)
					}
				}
			}
			tl, p := buildAnimTimeline(stress, ms)
			if len(p) > 0 {
				t.Fatal(p)
			}
			if err = os.WriteFile(filepath.Join(filepath.Dir(out), name+".html"), buildAnimPackage(animPackageInput{Script: stress, Timeline: tl, Voice: animVoiceTone, Format: "wav", Fixture: true, Audio: audio}), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}

}
