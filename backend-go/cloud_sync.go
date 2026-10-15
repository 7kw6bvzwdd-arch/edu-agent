package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Explicit maintenance mode: never starts task workers or modifies accounts/quotas.
func (s *Server) syncCloud(ctx context.Context) error {
	if s.cloud.objects == nil || s.cfg.MilvusURL == "" || s.cfg.EmbeddingModel == "" {
		return errors.New("cloud storage, Milvus and embedding configuration required")
	}
	files := 0
	if err := filepath.WalkDir(s.cfg.Storage, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("storage contains symlink; refusing to upload outside workspace")
		}
		if !d.IsDir() {
			if err := s.syncFile(ctx, path); err != nil {
				return err
			}
			files++
		}
		return nil
	}); err != nil {
		return err
	}
	log.Printf("Cloud storage synchronized: %d files", files)
	admin, err := pgxpool.New(ctx, s.cfg.DatabaseURL)
	if err != nil {
		return errors.New("maintenance database connection failed")
	}
	defer admin.Close()
	rows, err := admin.Query(ctx, `SELECT c.id,c.tenant_id,c.document_id,c.content,COALESCE(c.embedding::text,''),d.member_id FROM chunks c JOIN documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id WHERE d.status='ready' ORDER BY c.tenant_id,c.document_id,c.seq`)
	if err != nil {
		return err
	}
	type chunk struct {
		id, doc, text, vector string
		tenant, member        int64
	}
	chunks := []chunk{}
	for rows.Next() {
		var c chunk
		if err := rows.Scan(&c.id, &c.tenant, &c.doc, &c.text, &c.vector, &c.member); err != nil {
			rows.Close()
			return err
		}
		chunks = append(chunks, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i, c := range chunks {
		if c.vector == "" {
			c.vector, err = s.embed(ctx, Actor{TenantID: c.tenant, MemberID: c.member}, c.text)
			if err != nil {
				return errors.New("embedding backfill failed; progress retained, command can be retried")
			}
		}
		if err := s.upsertVector(ctx, c.tenant, c.doc, c.id, c.vector); err != nil {
			return err
		}
		tx, err := s.tx(ctx, c.tenant)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE chunks SET embedding=$1::vector WHERE id=$2", c.vector, c.id)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		if (i+1)%25 == 0 || i+1 == len(chunks) {
			log.Printf("Cloud vectors synchronized: %d/%d", i+1, len(chunks))
		}
	}
	return nil
}
