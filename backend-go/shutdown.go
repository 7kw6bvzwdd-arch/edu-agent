package main

import (
	"context"
	"os"
	"time"
)

// Hidden Windows services cannot receive a console Ctrl+C. Each launch has its
// own private shutdown file, recorded alongside its PID by the launcher.
func watchShutdownFile(ctx context.Context, path string, cancel context.CancelFunc) {
	if path == "" {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := os.Stat(path); err == nil {
				_ = os.Remove(path)
				cancel()
				return
			}
		}
	}
}
