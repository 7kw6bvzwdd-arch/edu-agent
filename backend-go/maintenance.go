package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"time"
)

// Explicit local recovery for existing demo accounts; normal startup never
// overwrites passwords. Uses the same nonempty seed-password contract as seed().
func resetSeedPassword(ctx context.Context, databaseURL, username, password string) error {
	switch username {
	case "teacher", "admin", "low", "other":
	default:
		return errors.New("only the four local demo accounts can use seed password recovery")
	}
	if len(password) == 0 || len(password) > 72 {
		return errors.New("SEED_PASSWORD must contain 1-72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("could not prepare password hash")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	db, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return errors.New("could not connect to the configured database")
	}
	defer db.Close(context.Background())
	tx, err := db.Begin(ctx)
	if err != nil {
		return errors.New("could not start password reset transaction")
	}
	defer tx.Rollback(context.Background())
	var id int64
	if err = tx.QueryRow(ctx, "SELECT id FROM users WHERE username=$1 FOR UPDATE", username).Scan(&id); err != nil {
		return errors.New("demo account not found or unavailable")
	}
	if _, err = tx.Exec(ctx, "UPDATE users SET password_hash=$1 WHERE id=$2", string(hash), id); err != nil {
		return errors.New("could not update account password")
	}
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", id); err != nil {
		return errors.New("could not revoke previous login sessions")
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("password reset commit failed; verify login before retrying")
	}
	return nil
}
