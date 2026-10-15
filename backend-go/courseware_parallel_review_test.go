package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCoursewareIndependentReviewsOverlapHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "并行双审核", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	visionStarted := make(chan struct{})
	var once sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string
			Messages []struct{ Content any }
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model == "review" {
			select {
			case <-visionStarted:
			case <-time.After(2 * time.Second):
				t.Error("visual review did not overlap content review")
			}
		} else {
			encoded := string(asJSON(req.Messages))
			if strings.Contains(encoded, "PRIVATE_ANSWER") {
				t.Error("visual review received hidden answer")
			}
			once.Do(func() { close(visionStarted) })
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"issues":[]}`}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "fixture", "review"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "fixture", "vision"
	d := Deck{Slides: []Slide{{Title: "浮力方向", Layout: "concept", Bullets: []string{"浮力方向竖直向上"}, Notes: "PRIVATE_ANSWER 教师备注"}}}
	ctx := withCoursewareArtifacts(context.Background())
	pptx := []byte("parallel-review-fixture")
	artifactCachePut(ctx, fmt.Sprintf("office:%x:0", sha256.Sum256(pptx)), asJSON(map[string][]byte{"slide-1.png": []byte("scheduling-only-image")}))
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	check, err := s.inspectGeneratedPage(ctx, a, id, TaskInput{}, d, pptx)
	if err != nil || !pageCheckPassed(check, false) {
		t.Fatal("independent reviews incomplete", check, err)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("review changed business accounting")
	}
}
