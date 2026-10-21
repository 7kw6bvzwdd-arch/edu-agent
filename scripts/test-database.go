// Run from backend-go so the existing pgx dependency is available.
// Only randomly named verification databases may be created or dropped.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func main() {
	if len(os.Args) != 3 || !regexp.MustCompile(`^edu_verify_20261009_[a-f0-9]{12}$`).MatchString(os.Args[2]) || (os.Args[1] != "create" && os.Args[1] != "drop") {
		fmt.Fprintln(os.Stderr, "usage: test-database create|drop edu_verify_20261009_<12 random hex digits>")
		os.Exit(1)
	}
	databaseURL := os.Getenv("TEST_DATABASE_ADMIN_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "TEST_DATABASE_ADMIN_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	check(err)
	defer conn.Close(ctx)
	name := pgx.Identifier{os.Args[2]}.Sanitize()
	if os.Args[1] == "create" {
		_, err = conn.Exec(ctx, "CREATE DATABASE "+name)
	} else {
		_, err = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	}
	check(err)
	fmt.Println(os.Args[1] + ": isolated test database ready")
}

func check(err error) {
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			fmt.Fprintf(os.Stderr, "isolated test database operation failed (PostgreSQL %s: %s)\n", pgErr.Code, pgErr.Message)
		} else {
			fmt.Fprintf(os.Stderr, "isolated test database operation failed (%T; credentials suppressed)\n", err)
		}
		os.Exit(1)
	}
}
