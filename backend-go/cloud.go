package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
)

type cloudServices struct {
	objects *minio.Client
	redis   *redis.Client
}

func (s *Server) initCloud(ctx context.Context) error {
	if s.cfg.MinIOEndpoint != "" {
		if s.cfg.MinIOBucket == "" || s.cfg.MinIOAccessKey == "" || s.cfg.MinIOSecretKey == "" {
			return errors.New("MinIO configuration incomplete")
		}
		endpoint := strings.TrimPrefix(strings.TrimPrefix(s.cfg.MinIOEndpoint, "http://"), "https://")
		c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(s.cfg.MinIOAccessKey, s.cfg.MinIOSecretKey, ""), Secure: strings.HasPrefix(s.cfg.MinIOEndpoint, "https://")})
		if err != nil {
			return errors.New("invalid MinIO endpoint")
		}
		s.cloud.objects = c
	}
	if s.cfg.RedisAddr != "" {
		s.cloud.redis = redis.NewClient(&redis.Options{Addr: s.cfg.RedisAddr, Username: s.cfg.RedisUsername, Password: s.cfg.RedisPassword, DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second})
	}
	if s.cfg.MilvusURL != "" {
		if s.cfg.MilvusToken == "" || s.cfg.MilvusDatabase == "" || s.cfg.MilvusCollection == "" {
			return errors.New("Milvus configuration incomplete")
		}
		if err := s.ensureVectorCollection(ctx); err != nil {
			return err
		}
	}
	for name, state := range s.dependencyHealth(ctx) {
		if state != "ok" && state != "local" && state != "disabled" {
			return fmt.Errorf("%s connection failed", name)
		}
	}
	return nil
}
func (s *Server) dependencyHealth(ctx context.Context) map[string]string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	d := map[string]string{"postgresql": "ok", "minio": "local", "redis": "disabled", "milvus": "disabled"}
	if s.db.Ping(ctx) != nil {
		d["postgresql"] = "unavailable"
	}
	if s.cloud.objects != nil {
		ok, err := s.cloud.objects.BucketExists(ctx, s.cfg.MinIOBucket)
		if err != nil || !ok {
			d["minio"] = "unavailable"
		} else {
			d["minio"] = "ok"
		}
	}
	if s.cloud.redis != nil {
		if s.cloud.redis.Ping(ctx).Err() != nil {
			d["redis"] = "unavailable"
		} else {
			d["redis"] = "ok"
		}
	}
	if s.cfg.MilvusURL != "" {
		if _, err := s.milvus(ctx, "/collections/list", map[string]any{}); err != nil {
			d["milvus"] = "unavailable"
		} else {
			d["milvus"] = "ok"
		}
	}
	return d
}

// Object names are relative to the trusted storage root, never user-supplied paths.
func (s *Server) objectKey(path string) (string, error) {
	root, err := filepath.Abs(s.cfg.Storage)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("file outside storage root")
	}
	return s.cfg.MinIOPrefix + filepath.ToSlash(rel), nil
}
func (s *Server) syncFile(ctx context.Context, path string) error {
	if s.cloud.objects == nil {
		return nil
	}
	key, err := s.objectKey(path)
	if err != nil {
		return err
	}
	_, err = s.cloud.objects.FPutObject(ctx, s.cfg.MinIOBucket, key, path, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return errors.New("云端文件上传失败，未完成交付")
	}
	return nil
}
func (s *Server) writeStoredFile(ctx context.Context, path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		return err
	}
	return s.syncFile(ctx, path)
}
func (s *Server) readStoredFile(ctx context.Context, path string) ([]byte, error) {
	if s.cloud.objects == nil {
		return os.ReadFile(path)
	}
	key, err := s.objectKey(path)
	if err != nil {
		return nil, err
	}
	object, err := s.cloud.objects.GetObject(ctx, s.cfg.MinIOBucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.New("云端文件读取失败")
	}
	defer object.Close()
	return io.ReadAll(io.LimitReader(object, 256<<20))
}
func (s *Server) serveStoredFile(w http.ResponseWriter, r *http.Request, path string) error {
	if s.cloud.objects == nil {
		http.ServeFile(w, r, path)
		return nil
	}
	key, err := s.objectKey(path)
	if err != nil {
		return err
	}
	object, err := s.cloud.objects.GetObject(r.Context(), s.cfg.MinIOBucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fail(503, "云端文件读取失败")
	}
	defer object.Close()
	info, err := object.Stat()
	if err != nil {
		return fail(503, "云端文件读取失败")
	}
	http.ServeContent(w, r, filepath.Base(path), info.LastModified, object)
	return nil
}
func (s *Server) removeStoredFile(ctx context.Context, path string) error {
	if s.cloud.objects != nil {
		key, err := s.objectKey(path)
		if err != nil {
			return err
		}
		if err := s.cloud.objects.RemoveObject(ctx, s.cfg.MinIOBucket, key, minio.RemoveObjectOptions{}); err != nil {
			return errors.New("云端文件删除失败")
		}
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (s *Server) syncDirectory(ctx context.Context, dir string) error {
	if s.cloud.objects == nil {
		return nil
	}
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return s.syncFile(ctx, path)
		}
		return nil
	})
}
func (s *Server) milvus(ctx context.Context, path string, in map[string]any) (json.RawMessage, error) {
	body := map[string]any{"dbName": s.cfg.MilvusDatabase}
	for k, v := range in {
		body[k] = v
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(s.cfg.MilvusURL, "/")+"/v2/vectordb"+path, strings.NewReader(string(asJSON(body))))
	if err != nil {
		return nil, errors.New("invalid Milvus URL")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.MilvusToken)
	res, err := s.http.Do(req)
	if err != nil {
		return nil, errors.New("Milvus connection failed")
	}
	defer res.Body.Close()
	var v struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&v) != nil || res.StatusCode != 200 || v.Code != 0 {
		return nil, fmt.Errorf("Milvus request failed (HTTP %d, code %d)", res.StatusCode, v.Code)
	}
	return v.Data, nil
}
func (s *Server) ensureVectorCollection(ctx context.Context) error {
	data, err := s.milvus(ctx, "/collections/has", map[string]any{"collectionName": s.cfg.MilvusCollection})
	if err != nil {
		return err
	}
	var v struct {
		Has bool `json:"has"`
	}
	if json.Unmarshal(data, &v) != nil {
		return errors.New("invalid Milvus response")
	}
	if !v.Has {
		fields := []map[string]any{
			{"fieldName": "id", "dataType": "VarChar", "isPrimary": true, "elementTypeParams": map[string]string{"max_length": "64"}},
			{"fieldName": "tenant_id", "dataType": "Int64"},
			{"fieldName": "document_id", "dataType": "VarChar", "elementTypeParams": map[string]string{"max_length": "64"}},
			{"fieldName": "vector", "dataType": "FloatVector", "elementTypeParams": map[string]string{"dim": "1536"}},
		}
		_, err = s.milvus(ctx, "/collections/create", map[string]any{"collectionName": s.cfg.MilvusCollection, "schema": map[string]any{"autoId": false, "enableDynamicField": false, "fields": fields}, "indexParams": []map[string]any{{"fieldName": "vector", "indexName": "vector_index", "metricType": "COSINE", "params": map[string]string{"index_type": "AUTOINDEX"}}}})
		if err != nil {
			return err
		}
	}
	_, err = s.milvus(ctx, "/collections/load", map[string]any{"collectionName": s.cfg.MilvusCollection})
	return err
}
func (s *Server) upsertVector(ctx context.Context, tid int64, doc, id, raw string) error {
	if s.cfg.MilvusURL == "" || raw == "" {
		return nil
	}
	var vector []float64
	if json.Unmarshal([]byte(raw), &vector) != nil || len(vector) != 1536 {
		return errors.New("invalid vector dimension")
	}
	_, err := s.milvus(ctx, "/entities/upsert", map[string]any{"collectionName": s.cfg.MilvusCollection, "data": []map[string]any{{"id": id, "tenant_id": tid, "document_id": doc, "vector": vector}}})
	return err
}
func (s *Server) deleteVectors(ctx context.Context, tid int64, doc string) error {
	if s.cfg.MilvusURL == "" {
		return nil
	}
	_, err := s.milvus(ctx, "/entities/delete", map[string]any{"collectionName": s.cfg.MilvusCollection, "filter": "tenant_id == " + strconv.FormatInt(tid, 10) + " and document_id == " + strconv.Quote(doc)})
	return err
}
func (s *Server) vectorIDs(ctx context.Context, tid int64, raw string) ([]string, error) {
	var vector []float64
	if json.Unmarshal([]byte(raw), &vector) != nil {
		return nil, errors.New("invalid query vector")
	}
	data, err := s.milvus(ctx, "/entities/search", map[string]any{"collectionName": s.cfg.MilvusCollection, "data": [][]float64{vector}, "annsField": "vector", "filter": "tenant_id == " + strconv.FormatInt(tid, 10), "limit": 24, "outputFields": []string{"id"}, "consistencyLevel": "Strong"})
	if err != nil {
		return nil, err
	}
	var matches []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(data, &matches) != nil {
		return nil, errors.New("invalid Milvus search response")
	}
	ids := []string{}
	for _, m := range matches {
		ids = append(ids, m.ID)
	}
	return ids, nil
}
