package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type videoRoundTrip func(*http.Request) (*http.Response, error)

func TestArkPollingRetriesReadsOnly(t *testing.T) {
	reads, writes := 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			writes++
			w.WriteHeader(503)
			return
		}
		reads++
		if reads < 3 {
			w.WriteHeader(503)
			return
		}
		respond(w, 200, map[string]string{"id": "remote-retry", "status": "running"})
	}))
	defer provider.Close()
	s := &Server{cfg: Config{VideoURL: provider.URL, VideoKey: "not-a-real-key"}, http: provider.Client()}
	result, err := s.arkVideoRequest(context.Background(), http.MethodGet, "remote-retry", nil)
	if err != nil || result.Status != "running" || reads != 3 {
		t.Fatal("transient polling failure was not recovered", reads, err)
	}
	if _, err = s.arkVideoRequest(context.Background(), http.MethodPost, "", map[string]string{"model": "mock"}); err == nil || writes != 1 {
		t.Fatal("a paid creation was retried", writes, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.arkVideoRequest(ctx, http.MethodGet, "remote-retry", nil); err == nil {
		t.Fatal("cancelled request succeeded")
	}
}

func TestCloudVideoRejectsCorruptFrames(t *testing.T) {
	if os.Getenv("TEST_VIDEO_SANDBOX") != "1" {
		t.Skip("set TEST_VIDEO_SANDBOX=1 to use the installed local Docker image")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "none", "--user", mediaSandboxUser(), "--mount", "type=bind,src="+dir+",dst=/media", "edu-manim:local", "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10", "-t", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "/media/lesson.mp4")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("local fixture: %v %s", err, b)
	}
	s := &Server{cfg: Config{AnimationImage: "edu-manim:local"}}
	if err := s.inspectCloudVideo(ctx, dir, 2); err != nil {
		t.Fatalf("valid video rejected: %v", err)
	}
	path := filepath.Join(dir, "lesson.mp4")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(b, []byte("mdat"))
	if start < 0 {
		t.Fatal("fixture lacks media payload")
	}
	clear(b[start+4:]) // Retain valid moov/codec/duration metadata, destroy frames.
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.inspectCloudVideo(ctx, dir, 2); err == nil {
		t.Fatal("corrupt video passed delivery validation and could be charged")
	}
}

func (f videoRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Start from real HTTP reservation, then switch the tool boundary to a cloud video.
// Vendor-contract tests do not need Docker or a real renderer to release that reservation.
func reservedVideoFixture(t *testing.T, s *Server, token string, a Actor, in TaskInput) string {
	request := in
	request.Type = "courseware"
	request.AnimationEngine = ""
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, request, 202))["id"].(string)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET type='animation',input=$1,status='rendering',stage='rendering' WHERE id=$2", asJSON(in), id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestVideoDownloadBoundariesAndMediaCheck(t *testing.T) {
	b := make([]byte, 1100)
	copy(b[4:8], "ftyp")
	client := &http.Client{Transport: videoRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("provider key leaked to video URL")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header), Request: r}, nil
	})}
	if _, err := downloadVideo(context.Background(), client, "https://media.example/video.mp4"); err != nil {
		t.Fatal(err)
	}
	b[4] = '!'
	if _, err := downloadVideo(context.Background(), client, "https://media.example/video.mp4"); err == nil {
		t.Fatal("accepted non-MP4")
	}
	public := publicHTTPClient(time.Second)
	defer public.CloseIdleConnections()
	for _, raw := range []string{"http://127.0.0.1/file.mp4", "http://169.254.169.254/file.mp4", "file:///tmp/video", "https://user:pass@example.com/video"} {
		if _, err := downloadVideo(context.Background(), public, raw); err == nil {
			t.Fatal("unsafe URL accepted", raw)
		}
	}
	good := []byte(`{"streams":[{"codec_type":"video","codec_name":"h264","width":1280,"height":720}],"format":{"duration":"5.1"}}`)
	if err := validateCloudVideoMetadata(good, 5); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.ReplaceAll(string(good), "5.1", "NaN"), strings.ReplaceAll(string(good), "5.1", "40"), strings.ReplaceAll(string(good), "h264", "unknown"), strings.ReplaceAll(string(good), "video", "audio")} {
		if validateCloudVideoMetadata([]byte(bad), 5) == nil {
			t.Fatal("invalid media accepted")
		}
	}
}

func TestArkVideoContractFailureAndNoDuplicatePaidSubmit(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	posts, gets := 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer video-test-key" {
			t.Error("missing video key")
		}
		switch r.Method {
		case "POST":
			posts++
			if r.URL.Path != "/api/v3/contents/generations/tasks" {
				t.Error("wrong create endpoint")
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["model"] != "test-video" || body["duration"] != float64(5) || body["generate_audio"] != false {
				t.Error("wrong video payload", body)
			}
			respond(w, 200, map[string]string{"id": "remote-video-1"})
		case "GET":
			gets++
			respond(w, 200, map[string]any{"id": "remote-video-1", "status": "failed", "usage": map[string]int{"total_tokens": 120, "completion_tokens": 100}})
		default:
			w.WriteHeader(204)
		}
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.LLMKey = "text-key"
	s.cfg.LLMModel = "text-model"
	s.cfg.VideoURL = provider.URL + "/api/v3"
	s.cfg.VideoKey = "video-test-key"
	s.cfg.VideoModel = "test-video"
	s.cfg.VideoDuration = "5"
	input := TaskInput{Type: "animation", Topic: "浮力", UseKB: true, Pages: 8, Theme: "forest", AnimationEngine: "ark", IdempotencyKey: newID()}
	id := reservedVideoFixture(t, s, token, actor, input)
	var err error
	for i := 0; i < 2; i++ {
		if err = s.renderArkVideo(context.Background(), actor, id, sampleDeck(input)); err == nil {
			t.Fatal("vendor failure not propagated")
		}
	}
	if posts != 1 || gets != 1 {
		t.Fatal("duplicate paid submit", posts, gets)
	}
	if err = s.finish(context.Background(), actor.TenantID, id, "failed", "test upstream failed", "", nil); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if task["charge_state"] != "released" {
		t.Fatal("failed video charged business credits")
	}
}

func TestArkVideoCancellationStopsPolling(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	id := ""
	deleted := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			respond(w, 200, map[string]string{"id": "remote-cancel"})
		case "GET":
			worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
			respond(w, 200, map[string]string{"id": "remote-cancel", "status": "running"})
		case "DELETE":
			deleted++
			w.WriteHeader(204)
		}
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.LLMKey = "text-key"
	s.cfg.LLMModel = "text"
	s.cfg.VideoURL = provider.URL
	s.cfg.VideoKey = "test-key"
	s.cfg.VideoModel = "test-video"
	s.cfg.VideoDuration = "5"
	input := TaskInput{Type: "animation", Topic: "浮力", UseKB: true, Pages: 8, Theme: "forest", AnimationEngine: "ark", IdempotencyKey: newID()}
	id = reservedVideoFixture(t, s, token, actor, input)
	var err error
	if err = s.renderArkVideo(context.Background(), actor, id, sampleDeck(input)); err == nil {
		t.Fatal("cancelled video reported success")
	}
	if deleted != 1 {
		t.Fatal("no best-effort remote cancellation")
	}
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if task["status"] != "cancelled" || task["charge_state"] != "released" {
		t.Fatal("cancellation overwritten")
	}
}

func TestCloudVideoHTTPDeliveryAndRejectedArtifacts(t *testing.T) {
	for _, provider := range []string{"ark", "minimax"} {
		t.Run(provider, func(t *testing.T) { cloudVideoHTTPDelivery(t, provider) })
	}
}

func cloudVideoHTTPDelivery(t *testing.T, protocol string) {
	if os.Getenv("TEST_VIDEO_SANDBOX") != "1" {
		t.Skip("requires the local media inspection image")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	// Fund the isolated fixture through the simulated billing API; do not depend
	// on seed balance or another test having purchased a package first.
	order := worksObject(t, worksCall(t, s, "POST", "/api/billing/orders", token, map[string]string{"sku": "topup"}, 201))
	worksCall(t, s, "POST", "/api/billing/orders/"+order["id"].(string)+"/pay", token, nil, 200)
	other := worksLogin(t, s, "other")
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	// Match the test process: it owns the private directory and must also be
	// able to modify the generated fixture for corrupted-frame checks.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "none", "--user", mediaSandboxUser(), "--mount", "type=bind,src="+dir+",dst=/media", "edu-manim:local", "ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10", "-f", "lavfi", "-i", "sine=frequency=440", "-t", "5", "-c:a", "aac", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "/media/lesson.mp4")
	if protocol == "ark" {
		cmd.Args = append(cmd.Args[:len(cmd.Args)-1], "-an", "/media/lesson.mp4")
	}
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, b)
	}
	media, err := os.ReadFile(filepath.Join(dir, "lesson.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	variant, id, gets, posts := "", "", 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/chat/completions":
			d := sampleDeck(TaskInput{Type: "animation", AnimationEngine: "ark", Topic: "浮力", Pages: 8, Theme: "forest"})
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
		case r.Method == "POST":
			posts++
			if protocol == "dashscope" {
				tx, err := s.tx(ctx, 1)
				if err != nil {
					t.Error(err)
					return
				}
				var remaining float64
				err = tx.QueryRow(ctx, "SELECT extract(epoch FROM lease_until-now())::float8 FROM tasks WHERE id=$1", id).Scan(&remaining)
				tx.Rollback(ctx)
				if err != nil || remaining < 32*60 {
					t.Error("Wan rendering lease expires before long cloud job", remaining, err)
				}
				respond(w, 200, map[string]any{"output": map[string]string{"task_id": "remote-e2e", "task_status": "PENDING"}})
				return
			}
			if protocol == "minimax" {
				respond(w, 200, map[string]string{"task_id": "remote-e2e"})
			} else {
				respond(w, 200, map[string]string{"id": "remote-e2e"})
			}
		case r.Method == "GET":
			gets++
			if protocol == "dashscope" {
				status := "SUCCEEDED"
				if gets == 1 {
					status = "RUNNING"
				}
				if variant == "vendor-failure" {
					status = "FAILED"
				}
				respond(w, 200, map[string]any{"output": map[string]string{"task_id": "remote-e2e", "task_status": status, "video_url": "https://media.example/lesson.mp4"}, "usage": map[string]int{"output_video_duration": 5}})
				return
			}
			if gets == 1 {
				if protocol == "minimax" {
					respond(w, 200, map[string]any{"task": map[string]string{"id": "remote-e2e", "status": "running"}})
				} else {
					respond(w, 200, map[string]string{"id": "remote-e2e", "status": "running"})
				}
				return
			}
			result := map[string]any{"id": "remote-e2e", "status": "succeeded", "model": "resolved-video-model", "content": map[string]string{"video_url": "https://media.example/lesson.mp4"}, "usage": map[string]int{"completion_tokens": 123}}
			if protocol == "minimax" {
				result["content"] = map[string]string{"url": "https://media.example/lesson.mp4"}
				respond(w, 200, map[string]any{"task": result})
			} else {
				respond(w, 200, result)
			}
		default:
			w.WriteHeader(204)
		}
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.LLMURL = provider.URL
	s.cfg.LLMKey = "mock-text"
	s.cfg.LLMModel = "mock-text"
	s.cfg.VideoURL = provider.URL
	s.cfg.VideoKey = "mock-video"
	s.cfg.VideoModel = "mock-video"
	s.cfg.VideoDuration = "5"
	s.cfg.VideoProvider = protocol
	if protocol == "minimax" {
		s.cfg.VideoModel = "MiniMax-H3"
	}
	if protocol == "dashscope" {
		s.cfg.VideoModel = "wan2.7-t2v"
	}
	s.cfg.AnimationImage = "edu-manim:local"
	s.mediaHTTP = &http.Client{Transport: videoRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Error("provider credential leaked to media")
		}
		body := media
		if variant == "corrupt" {
			body = bytes.Clone(media)
			start := bytes.Index(body, []byte("mdat"))
			if start < 0 {
				t.Fatal("missing mdat")
			}
			clear(body[start+4:])
		}
		if variant == "cancel" {
			worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
	})}
	variants := []string{"success", "corrupt", "cancel"}
	if protocol == "dashscope" {
		variants = append(variants, "vendor-failure")
	}
	for _, kind := range variants {
		t.Run(kind, func(t *testing.T) {
			variant, gets, posts = kind, 0, 0
			before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"].(float64)
			in := TaskInput{Type: "animation", AnimationEngine: "ark", Topic: "浮力", Grade: "八年级", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
			id = worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			if err = s.workOne(ctx); err != nil {
				t.Fatal(err)
			}
			v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
			if v["status"] != "awaiting_confirmation" {
				t.Fatal("missing teacher gate", v["error"])
			}
			if posts != 0 {
				t.Fatal("video created before teacher confirmation")
			}
			confirm := map[string]any{"result": v["result"], "revision": v["revision"], "accept_review": true}
			worksCall(t, s, "POST", "/api/tasks/"+id+"/confirm", token, confirm, 202)
			if err = s.workOne(ctx); err != nil {
				t.Fatal(err)
			}
			v = worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
			expectedGets := 2
			if kind == "vendor-failure" {
				expectedGets = 1
			}
			if posts != 1 || gets != expectedGets {
				t.Fatal("unexpected vendor calls", posts, gets)
			}
			billing := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			if kind == "success" {
				if v["status"] != "awaiting_acceptance" || v["charge_state"] != "reserved" || billing["balance"] != before {
					t.Fatal("cloud video bypassed acceptance", v["status"], v["error"])
				}
				worksCall(t, s, "POST", "/api/tasks/"+id+"/accept-animation", token, map[string]any{"revision": v["revision"], "accept": true}, 200)
				v = worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
				billing = worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
				if v["status"] != "succeeded" || v["charge_state"] != "settled" || billing["balance"] != before-40 {
					t.Fatal("success not delivered/settled", v["error"], billing)
				}
				file := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/mp4", token, nil, 200)
				if bytes.Equal(file.Body.Bytes(), media) || file.Body.Len() < 1000 {
					t.Fatal("download differs from validated video")
				}
				worksCall(t, s, "GET", "/api/tasks/"+id+"/download/mp4", other, nil, 404)
				worksCall(t, s, "POST", "/api/tasks/"+id+"/confirm", token, confirm, 409)
			} else {
				expected := "failed"
				if kind == "cancel" {
					expected = "cancelled"
				}
				if v["status"] != expected || v["charge_state"] != "released" || billing["balance"] != before {
					t.Fatal("invalid video charged or cancellation lost", v["status"], v["error"])
				}
				worksCall(t, s, "GET", "/api/tasks/"+id+"/download/mp4", token, nil, 404)
			}
			ledger := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
			worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
			worksCall(t, s, "GET", "/api/tasks/"+id+"/download/mp4", token, nil, 404)
			if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != ledger {
				t.Fatal("deletion changed billing")
			}
		})
	}
}
