package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func worksServer(t *testing.T) *Server {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	// Keep the explicit simulated-payment policy stable when a fixture switches
	// sample/live providers, so billing snapshots still compare the full account.
	s, err := NewServer(context.Background(), Config{DatabaseURL: isolatedTestDatabase(t), RuntimeURL: os.Getenv("TEST_RUNTIME_DATABASE_URL"), Secret: "test-secret-at-least-thirty-two-bytes", Mode: "sample", AllowSimulatedPayment: true, SeedPassword: "test-pass-123", Storage: t.TempDir(), RenderURL: os.Getenv("TEST_RENDER_URL")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func worksCall(t *testing.T, s *Server, method, path, token string, body any, want int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(asJSON(body)))
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	sum := h.Sum32()
	r.RemoteAddr = net.JoinHostPort(net.IPv4(198, 18, byte(sum>>8), byte(sum)).String(), "1234")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	return w
}

func worksObject(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func worksLogin(t *testing.T, s *Server, username string) string {
	t.Helper()
	return worksObject(t, worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": username, "password": "test-pass-123"}, 200))["token"].(string)
}

func TestWorksRenameDeleteIsolation(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	admin := worksLogin(t, s, "admin")
	other := worksLogin(t, s, "other")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	path := "/api/tasks/" + id
	for _, foreign := range []string{admin, other} {
		worksCall(t, s, "PATCH", path, foreign, map[string]string{"title": "私有课件"}, 404)
		worksCall(t, s, "DELETE", path, foreign, nil, 404)
	}
	worksCall(t, s, "PATCH", path, "", map[string]string{"title": "私有课件"}, 401)
	for _, title := range []string{"", "  ", strings.Repeat("字", 101)} {
		worksCall(t, s, "PATCH", path, token, map[string]string{"title": title}, 400)
	}
	worksCall(t, s, "PATCH", path, token, map[string]string{"title": "  第一课：浮力  "}, 200)
	v := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if v["display_title"] != "第一课：浮力" || v["input"].(map[string]any)["topic"] != "浮力" {
		t.Fatal("rename must persist independently of input", v)
	}
	worksCall(t, s, "DELETE", path, token, nil, 409)
	worksCall(t, s, "POST", path+"/cancel", token, nil, 200)
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "DELETE", path, token, nil, 200)
	for _, suffix := range []string{"", "/download/md", "/download/pptx", "/download/mp4", "/events"} {
		worksCall(t, s, "GET", path+suffix, token, nil, 404)
	}
	for _, suffix := range []string{"/cancel", "/confirm", "/edit", "/rewrite"} {
		worksCall(t, s, "POST", path+suffix, token, map[string]any{}, 404)
	}
	worksCall(t, s, "PATCH", path, token, map[string]string{"title": "恢复"}, 404)
	worksCall(t, s, "DELETE", path, token, nil, 404)
	worksCall(t, s, "POST", "/api/tasks", token, in, 409)
	if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
		t.Fatal("deletion changed billing or ledger")
	}
	if strings.Contains(worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String(), id) {
		t.Fatal("deleted task still listed")
	}
}

// Fixtures are terminal task snapshots; all visibility and mutation assertions use HTTP.
func worksFixture(t *testing.T, s *Server, token, kind, status, topic string, in TaskInput) string {
	t.Helper()
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	id := newID()
	in.Topic = topic
	in.Type = kind
	in.Pages = 8
	in.Theme = "forest"
	_, err = tx.Exec(context.Background(), `INSERT INTO tasks(id,tenant_id,member_id,type,status,stage,input,result,mode,cost,charge_state,idempotency_key)
		VALUES($1,$2,$3,$4,$5,$5,$6,$7,'sample',20,'settled',$1)`, id, a.TenantID, a.MemberID, kind, status, asJSON(in), asJSON(sampleDeck(in)))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := s.tx(context.Background(), a.TenantID)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Rollback(context.Background())
		_, err = cleanup.Exec(context.Background(), "UPDATE tasks SET status='cancelled' WHERE id=$1 AND charge_state<>'reserved'", id)
		if err == nil {
			err = cleanup.Commit(context.Background())
		}
		if err != nil {
			t.Error(err)
		}
	})
	return id
}

func TestWorksSearchPaginationAndTerminalStates(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	other := worksLogin(t, s, "other")
	marker := newID()
	for i := 0; i < 103; i++ {
		worksFixture(t, s, token, "courseware", "succeeded", fmt.Sprintf("%s %03d", marker, i), TaskInput{})
	}
	worksFixture(t, s, other, "courseware", "succeeded", marker+" foreign", TaskInput{})
	worksFixture(t, s, token, "animation", "degraded", marker+" animation", TaskInput{})
	query := "/api/tasks?q=" + marker + "&type=courseware&status=succeeded&limit=51"
	seen := map[string]bool{}
	for _, page := range []struct{ offset, count int }{{0, 51}, {50, 51}, {100, 3}} {
		var rows []map[string]any
		w := worksCall(t, s, "GET", query+fmt.Sprint("&offset=", page.offset), token, nil, 200)
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != page.count {
			t.Fatalf("offset %d: got %d, want %d", page.offset, len(rows), page.count)
		}
		for _, row := range rows[:min(50, len(rows))] {
			id := row["id"].(string)
			if seen[id] {
				t.Fatal("pagination duplicated a displayed row")
			}
			seen[id] = true
		}
	}
	if len(seen) != 103 {
		t.Fatal("works beyond 100 inaccessible")
	}
	for _, query := range []string{"limit=0", "limit=201", "limit=abc", "offset=-1"} {
		worksCall(t, s, "GET", "/api/tasks?"+query, token, nil, 400)
	}
	for _, state := range []string{"succeeded", "degraded", "failed", "cancelled"} {
		id := worksFixture(t, s, token, "courseware", state, marker+state, TaskInput{})
		worksCall(t, s, "PATCH", "/api/tasks/"+id, token, map[string]string{"title": "Literal%_" + marker}, 200)
		var rows []map[string]any
		json.Unmarshal(worksCall(t, s, "GET", "/api/tasks?q="+url.QueryEscape("Literal%_"+marker), token, nil, 200).Body.Bytes(), &rows)
		if len(rows) != 1 || rows[0]["id"] != id {
			t.Fatal("renamed title search must be literal and scoped", rows)
		}
		worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
	}
}

func TestWorksViewsCollapseRetriesHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	marker := newID()
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	in := TaskInput{Type: "courseware", Topic: marker, Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	first := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	latest := first
	rows := func(view, extra string, who string) []map[string]any {
		t.Helper()
		var result []map[string]any
		json.Unmarshal(worksCall(t, s, "GET", "/api/tasks?q="+marker+"&view="+view+extra, who, nil, 200).Body.Bytes(), &result)
		return result
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.finish(context.Background(), actor.TenantID, latest, "failed", "模拟生成中断", "", Deck{}); err != nil {
			t.Fatal(err)
		}
		if got := rows("works", "", token); len(got) != 0 {
			t.Fatalf("interrupted works clutter the default view: %d", len(got))
		}
		if got := rows("interrupted", "", token); len(got) != 1 || got[0]["id"] != latest {
			t.Fatal("retry chain should occupy one interrupted card", got)
		}
		if got := rows("history", "", token); len(got) != 1 {
			t.Fatal("continuation created extra work", got)
		}
		latest = worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+latest+"/retry", token, nil, 202))["id"].(string)
		if got := rows("works", "", token); len(got) != 1 || got[0]["id"] != latest {
			t.Fatal("latest active attempt missing", got)
		}
		if got := rows("interrupted", "", token); len(got) != 0 {
			t.Fatal("superseded failures still clutter interrupted view", got)
		}
	}
	// Existing tasks with the same subject must remain distinct; filtering precedes pagination.
	for i := 0; i < 3; i++ {
		worksFixture(t, s, token, "courseware", "succeeded", marker, TaskInput{})
	}
	cancelled := worksFixture(t, s, token, "animation", "cancelled", marker, TaskInput{})
	worksFixture(t, s, other, "courseware", "failed", marker, TaskInput{})
	worksFixture(t, s, admin, "courseware", "failed", marker, TaskInput{})
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	if got := rows("works", "&limit=2", token); len(got) != 2 {
		t.Fatal("filtering must happen before limit", got)
	}
	if got := rows("works", "&limit=2&offset=2", token); len(got) != 2 {
		t.Fatal("pagination lost distinct works", got)
	}
	if got := rows("works", "&status=succeeded&type=courseware", token); len(got) != 3 {
		t.Fatal("status/type filters or same-topic works lost", got)
	}
	if got := rows("interrupted", "&type=animation", token); len(got) != 1 || got[0]["id"] != cancelled {
		t.Fatal("cancelled work not recoverable in interrupted view", got)
	}
	if got := rows("history", "&status=failed&type=courseware", token); len(got) != 0 {
		t.Fatal("retry history filtering failed", got)
	}
	for _, who := range []string{other, admin} {
		for _, view := range []string{"works", "interrupted", "history"} {
			for _, row := range rows(view, "", who) {
				if row["id"] == first || row["id"] == latest {
					t.Fatal("private retry chain leaked")
				}
			}
		}
		worksCall(t, s, "GET", "/api/tasks/"+first, who, nil, 404)
	}
	worksCall(t, s, "GET", "/api/tasks?view=unknown", token, nil, 400)
	worksCall(t, s, "GET", "/api/tasks?view=history", "", nil, 401)
	if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
		t.Fatal("listing changed billing")
	}
	// Deleting the latest attempt must not resurrect an earlier failed card.
	worksCall(t, s, "POST", "/api/tasks/"+latest+"/cancel", token, nil, 200)
	worksCall(t, s, "DELETE", "/api/tasks/"+latest, token, nil, 200)
	if got := rows("interrupted", "&type=courseware", token); len(got) != 0 {
		t.Fatal("deleted continuation resurrected predecessor", got)
	}
	if got := rows("history", "&status=failed&type=courseware", token); len(got) != 0 {
		t.Fatal("deleted work remains visible", got)
	}
	worksCall(t, s, "GET", "/api/tasks/"+first, token, nil, 404)
}

func TestWorksDeletionStopsStreamsAndSourceReuse(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = "http://127.0.0.1:1", "fixture", "fixture"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: "http://127.0.0.1:1/tts", Key: "fixture", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
	token := worksLogin(t, s, "admin")
	other := worksLogin(t, s, "other")
	id := worksFixture(t, s, token, "courseware", "succeeded", "浮力", TaskInput{})
	revisionID := worksFixture(t, s, token, "revision", "succeeded", "浮力", TaskInput{ParentID: id})
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	// Cloud database authentication can take several round trips before the first heartbeat.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/tasks/"+id+"/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	started := time.Now()
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SSE connection established in %s", time.Since(started).Round(time.Millisecond))
	defer resp.Body.Close()
	worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
	var b bytes.Buffer
	closed := make(chan error, 1)
	go func() { _, err := b.ReadFrom(resp.Body); closed <- err }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal("existing SSE stream did not close after deletion", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("existing SSE stream did not close within five seconds of deletion")
	}
	if strings.Contains(b.String(), "data:") {
		t.Fatal("deleted task events exposed")
	}
	for _, target := range []string{id, revisionID} {
		for _, suffix := range []string{"", "/events", "/download/md"} {
			worksCall(t, s, "GET", "/api/tasks/"+target+suffix, token, nil, 404)
		}
		worksCall(t, s, "POST", "/api/tasks/"+target+"/review", token, nil, 404)
	}
	for _, who := range []string{token, other} {
		worksCall(t, s, "POST", "/api/tasks", who, TaskInput{Type: "animation", Topic: "浮力", UseKB: true, SourceTaskID: id, IdempotencyKey: newID()}, 404)
	}
}

func TestWorksActiveRevisionAndDeletedIdempotency(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	id := worksFixture(t, s, token, "courseware", "succeeded", "浮力", TaskInput{})
	path := "/api/tasks/" + id
	body := map[string]any{"instruction": "说明理由", "slide": 0, "revision": 0, "idempotency_key": newID()}
	child := worksObject(t, worksCall(t, s, "POST", path+"/rewrite", token, body, 202))["id"].(string)
	worksCall(t, s, "DELETE", path, token, nil, 409)
	worksCall(t, s, "POST", "/api/tasks/"+child+"/cancel", token, nil, 200)
	worksCall(t, s, "DELETE", "/api/tasks/"+child, token, nil, 200)
	worksCall(t, s, "POST", path+"/rewrite", token, body, 409)
	worksCall(t, s, "DELETE", path, token, nil, 200)
	worksCall(t, s, "POST", path+"/rewrite", token, body, 404)
}

func TestWorksDeleteDuringEditCannotRestoreArtifact(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	id := worksFixture(t, s, token, "courseware", "succeeded", "浮力", TaskInput{})
	entered, release := make(chan struct{}), make(chan struct{})
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			respond(w, 200, map[string]string{"courseware_media_version": "media-v1"})
			return
		}
		close(entered)
		<-release
		w.Write([]byte("rendered-test-artifact"))
	}))
	defer renderer.Close()
	s.cfg.RenderURL = renderer.URL
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		r := httptest.NewRequest("POST", "/api/tasks/"+id+"/edit", bytes.NewReader(asJSON(map[string]any{"revision": 0, "result": sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "forest"})})))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		done <- w
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("edit did not reach renderer")
	}
	w := worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
	close(release)
	w = <-done
	if w.Code != 409 && w.Code != 404 {
		t.Fatal("in-flight edit restored deleted work", w.Code)
	}
	worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 404)
}
