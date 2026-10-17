package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnimationRebuildRejectsNewWorkBeforeSideEffects(t *testing.T) {
	for _, mode := range []string{"sample", "live"} {
		for _, engine := range []string{"manim", "ark"} {
			t.Run(mode+"/"+engine, func(t *testing.T) {
				// A nil database/provider catches any work or reservation before rejection.
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("disabled animation reached database/provider: %v", recovered)
					}
				}()
				s := &Server{cfg: Config{Mode: mode}}
				in := TaskInput{Type: "animation", AnimationEngine: engine, Topic: "浮力", UseKB: true, IdempotencyKey: "rebuild-check"}
				r := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(asJSON(in)))
				w := httptest.NewRecorder()
				api(func(w http.ResponseWriter, r *http.Request) error {
					return s.createTask(w, r, Actor{Role: "admin"})
				}).ServeHTTP(w, r)
				if w.Code != 503 || !bytes.Contains(w.Body.Bytes(), []byte("旧版动画生成已移除")) {
					t.Fatalf("got %d %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestAnimationRebuildWorkerDoesNotCallLegacyEngine(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("worker reached legacy animation dependencies: %v", recovered)
		}
	}()
	s := &Server{}
	for _, mode := range []string{"sample", "live"} {
		s.cfg.Mode = mode
		for _, engine := range []string{"manim", "ark"} {
			err := func() error {
				_, err := s.generate(context.Background(), Actor{}, "old-task", TaskInput{Type: "animation", AnimationEngine: engine}, nil)
				return err
			}()
			if err == nil || !bytes.Contains([]byte(err.Error()), []byte("旧版动画生成已移除")) {
				t.Fatalf("legacy engine still reachable: %v", err)
			}
		}
	}
}
