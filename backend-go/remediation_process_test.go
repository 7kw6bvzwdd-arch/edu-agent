package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRemediationStartupFailureRedactsCredentials(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process := exec.Command(executable, "-test.run=^TestRemediationServiceProcessHelper$")
	for _, item := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(item, "=", 2)[0])
		if key == "PATH" || key == "SYSTEMROOT" || key == "TEMP" || key == "TMP" {
			process.Env = append(process.Env, item)
		}
	}
	secret := "never-print-this-startup-password"
	process.Env = append(process.Env, "EDU_PROCESS_FIXTURE=1", "DATABASE_URL=postgres://edu:"+secret+"@127.0.0.1:invalid/edu", "RUNTIME_DATABASE_URL=postgres://edu_app:fixture@127.0.0.1:1/edu", "JWT_SECRET=test-secret-at-least-thirty-two-bytes", "APP_MODE=sample")
	output, err := process.CombinedOutput()
	if err == nil {
		t.Fatal("invalid startup configuration succeeded")
	}
	if bytes.Contains(output, []byte(secret)) || bytes.Contains(output, []byte("postgres://")) {
		t.Fatal("startup logs disclose database credentials")
	}
}

func TestRemediationServiceProcessHelper(t *testing.T) {
	if os.Getenv("EDU_PROCESS_FIXTURE") != "1" {
		t.Skip("subprocess helper only")
	}
	os.Args = []string{"edu-server"}
	main()
}

// A real service process, HTTP task state, private fixture provider, isolated DB.
func TestRemediationProcessRecoveryHTTP(t *testing.T) {
	s := worksServer(t)
	tokens := []string{worksLogin(t, s, "teacher"), worksLogin(t, s, "other")}
	ids := []string{}
	for _, token := range tokens {
		ids = append(ids, worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "实际进程恢复", UseKB: true, IdempotencyKey: newID()}, 202))["id"].(string))
	}
	for i, id := range ids {
		tx, err := s.tx(context.Background(), int64(i+1))
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live',input=jsonb_set(input,'{use_kb}','false') WHERE id=$1", id)
		if err == nil {
			err = tx.Commit(context.Background())
		} else {
			tx.Rollback(context.Background())
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer provider.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	request := filepath.Join(t.TempDir(), "shutdown.request")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var process *exec.Cmd
	var done chan error
	start := func() {
		process = exec.Command(executable, "-test.run=^TestRemediationServiceProcessHelper$")
		keep := map[string]bool{"PATH": true, "PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "TEMP": true, "TMP": true, "USERPROFILE": true, "LOCALAPPDATA": true, "APPDATA": true}
		for _, entry := range os.Environ() {
			if keep[strings.ToUpper(strings.SplitN(entry, "=", 2)[0])] {
				process.Env = append(process.Env, entry)
			}
		}
		process.Env = append(process.Env, "EDU_PROCESS_FIXTURE=1", "DATABASE_URL="+os.Getenv("TEST_DATABASE_URL"), "RUNTIME_DATABASE_URL="+os.Getenv("TEST_RUNTIME_DATABASE_URL"), "JWT_SECRET=test-secret-at-least-thirty-two-bytes", "APP_MODE=live", "STORAGE_DIR="+s.cfg.Storage, "LISTEN_ADDR="+address, "SHUTDOWN_REQUEST_FILE="+request, "RENDER_SERVICE_URL="+s.cfg.RenderURL, "COURSEWARE_LLM_BASE_URL="+provider.URL, "COURSEWARE_LLM_API_KEY=fixture", "COURSEWARE_LLM_MODEL=fixture", "COURSEWARE_CONTENT_REVIEW_DISABLED=true")
		if err := process.Start(); err != nil {
			t.Fatal(err)
		}
		done = make(chan error, 1)
		cmd := process
		finished := done
		go func() { finished <- cmd.Wait() }()
	}
	defer func() {
		if process != nil && process.Process != nil {
			_ = process.Process.Kill()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("process did not exit")
			}
		}
	}()
	waitState := func(want string) {
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			matched := true
			for i, id := range ids {
				v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, tokens[i], nil, 200))
				if v["status"] != want || v["charge_state"] != "reserved" {
					matched = false
				}
			}
			if matched {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("process tasks did not reach %s with reservations retained", want)
	}
	start()
	waitState("researching")
	if err := os.WriteFile(request, []byte("shutdown"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("graceful process exit failed")
		}
		process = nil
	case <-time.After(30 * time.Second):
		t.Fatal("graceful process did not stop")
	}
	waitState("pending")
	start()
	waitState("researching")
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		process = nil
	case <-time.After(10 * time.Second):
		t.Fatal("forced process did not stop")
	}
	for i, id := range ids {
		tx, err := s.tx(context.Background(), int64(i+1))
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), "UPDATE tasks SET lease_until=now()-interval '1 second' WHERE id=$1", id)
		if err == nil {
			err = tx.Commit(context.Background())
		} else {
			tx.Rollback(context.Background())
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	start()
	waitState("researching")
	for i, id := range ids {
		var state struct {
			Charge string `json:"charge_state"`
			Events []struct {
				Stage string `json:"stage"`
			}
		}
		json.Unmarshal(worksCall(t, s, "GET", "/api/tasks/"+id, tokens[i], nil, 200).Body.Bytes(), &state)
		recovered := 0
		for _, event := range state.Events {
			if event.Stage == "auto_recovering" {
				recovered++
			}
		}
		if recovered < 2 {
			t.Fatal("restart and killed-process recoveries were not observable")
		}
		worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", tokens[i], nil, 200)
	}
	if err := os.WriteFile(request, []byte("shutdown"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
		process = nil
	case <-time.After(30 * time.Second):
		t.Fatal("final service shutdown failed")
	}
}
