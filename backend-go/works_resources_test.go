package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestSafeStoragePathBoundaries(t *testing.T) {
	root := t.TempDir()
	s := &Server{}
	s.cfg.Storage = root
	inside := filepath.Join(root, "1", "deck.pptx")
	if err := s.safeStoragePath(inside); err != nil {
		t.Fatal(err)
	}
	if err := s.safeStoragePath(filepath.Join(root, "..", "outside.pptx")); err == nil {
		t.Fatal("accepted path outside storage")
	}
	if runtime.GOOS == "windows" {
		if err := s.safeStoragePath(strings.ToUpper(inside)); err != nil {
			t.Fatal("Windows path casing must reach the storage root", err)
		}
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(t.TempDir(), link); err == nil {
		if err := s.safeStoragePath(filepath.Join(link, "deck.pptx")); err == nil {
			t.Fatal("accepted symlinked storage parent")
		}
	}
}

func TestWorksRestoreWithoutGenerationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "admin")
	id := worksFixture(t, s, token, "courseware", "cancelled", "restore-"+newID(), TaskInput{})
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	original := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	worksCall(t, s, "POST", "/api/tasks/"+id+"/restore", other, nil, 404)
	for i := 0; i < 2; i++ {
		worksCall(t, s, "POST", "/api/tasks/"+id+"/restore", token, nil, 200)
	}
	restored := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if restored["status"] != original["status"] || string(asJSON(restored["result"])) != string(asJSON(original["result"])) || restored["restored_at"] == nil {
		t.Fatal("restore must only change collection membership")
	}
	for _, view := range []string{"works", "interrupted"} {
		body := worksCall(t, s, "GET", "/api/tasks?view="+view, token, nil, 200).Body.String()
		if strings.Contains(body, id) != (view == "works") {
			t.Fatal("restored work is in wrong collection", view)
		}
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("restore changed points")
	}
	worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/restore", token, nil, 404)
}

func TestRestoredCancelledCoursewareCanBeManuallyContinued(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "取消后还原", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 409)
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "POST", "/api/tasks/"+id+"/restore", token, nil, 200)
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("restore charged points")
	}
	next := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202))["id"].(string)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/restore", token, nil, 409)
	list := worksCall(t, s, "GET", "/api/tasks?view=works", token, nil, 200).Body.String()
	if next != id || strings.Count(list, `"id":"`+id+`"`) != 1 {
		t.Fatal("continuation must occupy exactly one work")
	}
	worksCall(t, s, "POST", "/api/tasks/"+next+"/cancel", token, nil, 200)
}

func TestDeletedWorkWaitsForCancelledWorkerAndRecleansLateFiles(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "animation", "cancelled", "cancelled render", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET lease_until=now()+interval '1 minute' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := storagePath(s, a.TenantID, id, ".mp4")
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("render in flight"), 0600); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200))
	if v["cleanup_pending"] != true {
		t.Fatal("in-flight worker must defer cleanup")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("active worker file removed")
	}
	s.queueDeletedCleanup(a.TenantID, id, true)
	if ok, err := s.cleanupDeletedTask(context.Background(), a.TenantID, id); err != nil || !ok {
		t.Fatal("cleanup after worker exit failed", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("worker file retained")
	}
	if err = os.WriteFile(path, []byte("late editor result"), 0600); err != nil {
		t.Fatal(err)
	}
	s.queueDeletedCleanup(a.TenantID, id, false)
	if ok, err := s.cleanupDeletedTask(context.Background(), a.TenantID, id); err != nil || !ok {
		t.Fatal("late result cleanup failed", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("late file retained")
	}
}

func TestDeletedWorkCloudCleanupFailureRemainsRetryable(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "failed", "cloud cleanup", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	var mu sync.Mutex
	objects := map[string]bool{}
	blocked := true
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		key := strings.TrimPrefix(r.URL.Path, "/test-bucket/")
		if r.Method == "GET" && r.URL.Query().Has("list-type") {
			type item struct {
				Key          string
				Size         int
				LastModified string
			}
			result := struct {
				XMLName     xml.Name `xml:"ListBucketResult"`
				Name        string
				Prefix      string
				IsTruncated bool
				Contents    []item
			}{Name: "test-bucket", Prefix: r.URL.Query().Get("prefix")}
			for k := range objects {
				if strings.HasPrefix(k, result.Prefix) {
					result.Contents = append(result.Contents, item{Key: k, Size: 15, LastModified: time.Now().UTC().Format(time.RFC3339)})
				}
			}
			w.Header().Set("Content-Type", "application/xml")
			xml.NewEncoder(w).Encode(result)
			return
		}
		if r.Method == "DELETE" {
			if blocked {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(403)
				w.Write([]byte("<Error><Code>AccessDenied</Code><Message>fixture</Message></Error>"))
				return
			}
			delete(objects, key)
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(400)
	}))
	defer remote.Close()
	client, err := minio.New(strings.TrimPrefix(remote.URL, "http://"), &minio.Options{Creds: credentials.NewStaticV4("fixture", "fixture-secret", ""), Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	s.cloud.objects = client
	s.cfg.MinIOBucket = "test-bucket"
	s.cfg.MinIOPrefix = "test/"
	own := storagePath(s, a.TenantID, id, ".pptx")
	key, err := s.objectKey(own)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{key, key + ".slides/slide-1.png", strings.Replace(key, id, id+"-cloud-only", 1)} {
		objects[k] = true
	}
	unrelated := "test/1/" + newID() + ".pptx"
	objects[unrelated] = true
	if err = os.MkdirAll(filepath.Dir(own), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(own, []byte("local copy"), 0600); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200))
	if v["cleanup_pending"] != true {
		t.Fatal("cloud failure reported success")
	}
	pending := worksObject(t, worksCall(t, s, "GET", "/api/tasks/cleanup-status", token, nil, 200))
	if pending["pending"].(float64) < 1 {
		t.Fatal("cleanup intent was lost")
	}
	worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 404)
	mu.Lock()
	blocked = false
	mu.Unlock()
	if ok, err := s.cleanupDeletedTask(context.Background(), a.TenantID, id); err != nil || !ok {
		t.Fatal("cloud cleanup retry failed", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(objects) != 1 || !objects[unrelated] {
		t.Fatal("cloud objects leaked or unrelated object removed", len(objects))
	}
	if _, err = os.Stat(own); !os.IsNotExist(err) {
		t.Fatal("local cache retained")
	}
}

func TestWorksDeletionReclaimsOnlyUnsharedFilesHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	id := worksFixture(t, s, token, "courseware", "failed", "cleanup-"+newID(), TaskInput{})
	child := worksFixture(t, s, token, "revision", "succeeded", "child revision", TaskInput{ParentID: id})
	kept := worksFixture(t, s, token, "courseware", "succeeded", "kept", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	own := storagePath(s, a.TenantID, id, ".pptx")
	childPath := storagePath(s, a.TenantID, child, ".pptx")
	shared := storagePath(s, a.TenantID, id+"-shared", ".pptx")
	files := []string{own, filepath.Join(own+".slides", "slide-1.png"), storagePath(s, a.TenantID, id+"-orphan", ".pptx"), shared, filepath.Join(shared+".slides", "slide-1.png"), storagePath(s, a.TenantID, kept, ".pptx")}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("private-artifact"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(childPath, []byte("child-artifact"), 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET pptx_path=$1 WHERE id=$2", own, id); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET pptx_path=$1 WHERE id=$2", shared, kept); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "INSERT INTO artifact_revisions(tenant_id,task_id,revision,result,pptx_path,mp4_path) VALUES($1,$2,1,'{}',$3,'')", a.TenantID, id, shared); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "DELETE", "/api/tasks/"+id, other, nil, 404)
	for _, f := range files {
		if _, err = os.Stat(f); err != nil {
			t.Fatal("foreign delete touched file")
		}
	}
	reply := worksObject(t, worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200))
	if reply["cleanup_pending"] != true {
		t.Fatal("child cleanup must remain visible", reply)
	}
	worksCall(t, s, "GET", "/api/tasks/"+child, token, nil, 404)
	if ok, err := s.cleanupDeletedTask(context.Background(), a.TenantID, child); err != nil || !ok {
		t.Fatal("child cleanup failed", err)
	}
	if _, err = os.Stat(childPath); !os.IsNotExist(err) {
		t.Fatal("child artifact retained")
	}
	for _, f := range files[:3] {
		if _, err = os.Stat(f); !os.IsNotExist(err) {
			t.Fatal("deleted work still occupies storage", f)
		}
	}
	for _, f := range files[3:] {
		if _, err = os.Stat(f); err != nil {
			t.Fatal("shared or unrelated file deleted", f)
		}
	}
	worksCall(t, s, "GET", "/api/tasks/"+kept, token, nil, 200)
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("deletion changed accounting")
	}
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var empty bool
	if err = tx.QueryRow(context.Background(), "SELECT result='{}'::jsonb AND sources='[]'::jsonb AND pptx_path='' AND mp4_path='' FROM tasks WHERE id=$1", id).Scan(&empty); err != nil || !empty {
		t.Fatal("deleted payload retained", err)
	}
	tx.Rollback(context.Background())
	worksCall(t, s, "DELETE", "/api/tasks/"+kept, token, nil, 200)
	for _, f := range files[3:] {
		if _, err = os.Stat(f); !os.IsNotExist(err) {
			t.Fatal("last reference did not release file", f)
		}
	}
}
