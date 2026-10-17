// Run from backend-go with the existing minio-go dependency.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type item struct {
	Key    string
	File   string
	SHA256 string
	Size   int64
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "object backup/restore failed (details suppressed)")
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: backup-objects backup|restore-drill directory")
	}
	mode, dir := os.Args[1], os.Args[2]
	if mode != "backup" && mode != "restore-drill" {
		return fmt.Errorf("invalid mode")
	}
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		return fmt.Errorf("MINIO_ENDPOINT required")
	}
	c, err := minio.New(strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://"), &minio.Options{Creds: credentials.NewStaticV4(os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY"), ""), Secure: strings.HasPrefix(endpoint, "https://")})
	if err != nil {
		return err
	}
	bucket, prefix := os.Getenv("MINIO_BUCKET"), os.Getenv("MINIO_PREFIX")
	if bucket == "" || prefix == "" {
		return fmt.Errorf("explicit bucket and prefix required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	manifest := filepath.Join(dir, "objects.json")
	if mode == "backup" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		list := []item{}
		for info := range c.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if info.Err != nil {
				return info.Err
			}
			name := sha256.Sum256([]byte(info.Key))
			file := hex.EncodeToString(name[:])
			obj, e := c.GetObject(ctx, bucket, info.Key, minio.GetObjectOptions{})
			if e != nil {
				return e
			}
			dst, e := os.OpenFile(filepath.Join(dir, file), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				obj.Close()
				return e
			}
			h := sha256.New()
			n, e := io.Copy(io.MultiWriter(dst, h), obj)
			ce := dst.Close()
			obj.Close()
			if e != nil {
				return e
			}
			if ce != nil {
				return ce
			}
			if n != info.Size {
				return fmt.Errorf("object changed during backup")
			}
			list = append(list, item{info.Key, file, hex.EncodeToString(h.Sum(nil)), n})
		}
		b, e := json.MarshalIndent(list, "", "  ")
		if e != nil {
			return e
		}
		if e = os.WriteFile(manifest, b, 0600); e != nil {
			return e
		}
		fmt.Printf("objects backed up: %d\n", len(list))
		return nil
	}
	b, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	var list []item
	if err = json.Unmarshal(b, &list); err != nil {
		return err
	}
	drill := prefix + "restore-drill/" + fmt.Sprint(time.Now().UnixNano()) + "/"
	keys := []string{}
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		for _, key := range keys {
			if e := c.RemoveObject(clean, bucket, key, minio.RemoveObjectOptions{}); e != nil {
				fmt.Fprintln(os.Stderr, "restore drill cleanup failed")
				os.Exit(1)
			}
		}
	}()
	for _, v := range list {
		if len(v.File) != 64 || strings.ContainsAny(v.File, "/\\") {
			return fmt.Errorf("invalid manifest path")
		}
		file, e := os.Open(filepath.Join(dir, v.File))
		if e != nil {
			return e
		}
		h := sha256.New()
		n, e := io.Copy(h, file)
		file.Close()
		if e != nil {
			return e
		}
		if n != v.Size || hex.EncodeToString(h.Sum(nil)) != v.SHA256 {
			return fmt.Errorf("object checksum mismatch")
		}
		key := drill + v.File
		keys = append(keys, key)
		if _, e = c.FPutObject(ctx, bucket, key, filepath.Join(dir, v.File), minio.PutObjectOptions{}); e != nil {
			return e
		}
		obj, e := c.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
		if e != nil {
			return e
		}
		h = sha256.New()
		n, e = io.Copy(h, obj)
		obj.Close()
		if e != nil {
			return e
		}
		if n != v.Size || hex.EncodeToString(h.Sum(nil)) != v.SHA256 {
			return fmt.Errorf("restored object mismatch")
		}
	}
	fmt.Printf("objects restored and verified in temporary prefix: %d\n", len(list))
	return nil
}
