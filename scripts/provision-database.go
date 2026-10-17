// Run from backend-go. Credentials are supplied only through the environment.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	if err := provision(); err != nil {
		fmt.Fprintln(os.Stderr, "Database provisioning failed: "+err.Error()+"; credentials and connection details suppressed")
		os.Exit(1)
	}
	fmt.Println("Database runtime role provisioned without superuser or RLS bypass")
}
func provision() error {
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("RUNTIME_DATABASE_URL") == "" {
		return fmt.Errorf("explicit credentials required")
	}
	admin, err := pgx.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("invalid admin configuration")
	}
	runtime, err := pgx.ParseConfig(os.Getenv("RUNTIME_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("invalid runtime configuration")
	}
	if runtime.User != "edu_app" || len(runtime.Password) < 24 || admin.User == runtime.User || runtime.Host != admin.Host || runtime.Port != admin.Port || runtime.Database != admin.Database {
		return fmt.Errorf("matching admin and edu_app configuration required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, admin)
	if err != nil {
		return fmt.Errorf("admin connection")
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin role transaction")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='edu_app') THEN CREATE ROLE edu_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; END IF; END $$`); err != nil {
		return fmt.Errorf("create runtime role")
	}
	literal := "'" + strings.ReplaceAll(runtime.Password, "'", "''") + "'"
	if _, err = tx.Exec(ctx, "SET LOCAL standard_conforming_strings=on"); err != nil {
		return fmt.Errorf("set literal policy")
	}
	if _, err = tx.Exec(ctx, "ALTER ROLE edu_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD "+literal); err != nil {
		return fmt.Errorf("configure runtime role")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit runtime role")
	}
	return nil
}
