package main

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var isolatedTestDatabases sync.Map

// Restarts inside one test share data; other tests get fresh accounts and work.
func isolatedTestDatabase(t *testing.T) string {
	t.Helper()
	if value, ok := isolatedTestDatabases.Load(t); ok {
		return value.(string)
	}
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	u, err := url.Parse(base)
	if err != nil || !regexp.MustCompile(`^/edu_verify_20261009_[a-f0-9]{12}$`).MatchString(u.Path) {
		t.Fatal("test database must belong to the isolated verification harness")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal("cannot connect to verification database administrator")
	}
	defer admin.Close(context.Background())
	name := "edu_verify_20261009_" + newID()[:12]
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal("cannot create per-test verification database")
	}
	t.Cleanup(func() {
		isolatedTestDatabases.Delete(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, base)
		if err == nil {
			defer conn.Close(ctx)
			_, err = conn.Exec(ctx, "DROP DATABASE "+quoted+" WITH (FORCE)")
		}
		if err != nil {
			t.Error("cannot remove per-test verification database")
		}
	})
	u.Path = "/" + name
	database := u.String()
	t.Setenv("TEST_DATABASE_URL", database)
	if runtime := os.Getenv("TEST_RUNTIME_DATABASE_URL"); runtime != "" {
		ru, err := url.Parse(runtime)
		if err != nil {
			t.Fatal("invalid test runtime database URL")
		}
		ru.Path = u.Path
		t.Setenv("TEST_RUNTIME_DATABASE_URL", ru.String())
	}
	isolatedTestDatabases.Store(t, database)
	return database
}

func TestDatabaseIsolationHTTP(t *testing.T) {
	t.Run("first", func(t *testing.T) {
		s := worksServer(t)
		token := worksLogin(t, s, "teacher")
		worksFixture(t, s, token, "courseware", "succeeded", "must stay in this test", TaskInput{})
	})
	t.Run("second", func(t *testing.T) {
		s := worksServer(t)
		token := worksLogin(t, s, "teacher")
		rows := worksCall(t, s, "GET", "/api/tasks", token, nil, 200)
		if rows.Body.String() != "[]\n" {
			t.Fatal("another test's work leaked into a fresh test", rows.Body.String())
		}
	})
}
