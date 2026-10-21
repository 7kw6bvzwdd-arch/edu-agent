package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real Vue, live models and public APIs against an isolated test database.
func TestLiveConversationClosureBrowserHTTP(t *testing.T) {
	if os.Getenv("TEST_LONG_CONVERSATION_LIVE") != "1" {
		t.Skip("explicit live conversation and generation opt-in required")
	}
	s := worksServer(t)
	s.cfg.Mode, s.cfg.CoursewareReviewPolicy = "live", "all"
	s.cfg.CoursewareContentReviewDisabled = false
	s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	s.cfg.SearchURL, s.cfg.SearchKey = env("SEARCH_API_URL", "https://api.tavily.com/search"), os.Getenv("SEARCH_API_KEY")
	if !s.agentConfigured("courseware") || !s.coursewareQualityConfigured() {
		t.Fatal("real model and review configuration required")
	}
	dir, err := filepath.Abs(filepath.Join("../work/acceptance-closure-20261020", env("TEST_FULL_DECK_RUN", "conversation")))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s.cfg.Storage = filepath.Join(dir, "artifacts")
	static, api := http.FileServer(http.Dir("../frontend-vue/dist")), s.Handler()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
		} else {
			static.ServeHTTP(w, r)
		}
	}))
	defer host.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 28*time.Minute)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.worker(ctx) }()
	defer func() { cancel(); <-done }()
	command := exec.CommandContext(ctx, "node", "../scripts/probe-long-conversation-live.cjs", host.URL, dir)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err = command.Run(); err != nil {
		t.Fatal("live browser acceptance failed; evidence saved:", err)
	}
}
