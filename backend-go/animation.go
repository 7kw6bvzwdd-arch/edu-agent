package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed manim_preflight.py
var manimPreflight string

type animationFailure struct{ kind, detail string }

func (e *animationFailure) Error() string { return e.message() + "：" + e.detail }
func (e *animationFailure) message() string {
	switch e.kind {
	case "preflight":
		return "动画兼容性预检未通过"
	case "code":
		return "动画脚本执行失败"
	case "timeout":
		return "动画渲染超过100秒，请减少对象和复杂转场"
	case "media":
		return "动画成品未通过解码、时长或格式检查"
	default:
		return "动画隔离环境异常，请检查Docker及动画镜像；未重试模型"
	}
}

//go:embed scene_renderer.py
var sceneRenderer string

func controlledAnimation(d Deck) string {
	// JSON is double encoded: user text cannot escape into Python source.
	return "import json\nPLAN=json.loads(" + string(asJSON(string(asJSON(d)))) + ")\n" + sceneRenderer
}

func (s *Server) renderAnimation(ctx context.Context, a Actor, id string, d Deck) error {
	if err := s.activeVideoTask(ctx, a, id); err != nil {
		return err
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	var sources []Source
	err = tx.QueryRow(ctx, "SELECT sources FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", id, a.MemberID).Scan(&sources)
	tx.Rollback(ctx)
	if err != nil {
		return err
	}
	if err = validateAnimationPlan(d, sources, 0, false); err != nil {
		return err
	}
	if err = s.videoEvent(ctx, a, id, "结构化分镜通过检查，使用受控图形组件渲染720p/30fps预览；不执行模型自由代码"); err != nil {
		return err
	}
	d.Script = ""
	path, err := s.runAnimation(ctx, a.TenantID, id, controlledAnimation(d), 0)
	if err != nil {
		return err
	}
	return s.stageAnimation(ctx, a, id, path, d)
}
func (s *Server) runAnimation(ctx context.Context, tid int64, id, script string, attempt int) (string, error) {
	if len(script) > 64<<10 {
		return "", &animationFailure{"preflight", "脚本超过64KiB，请简化"}
	}
	root, e := filepath.Abs(filepath.Join(s.cfg.Storage, "animation-jobs", id, fmt.Sprint(attempt)))
	if e != nil {
		return "", e
	}
	input := filepath.Join(root, "input")
	output := filepath.Join(root, "output")
	if e = os.MkdirAll(input, 0755); e != nil {
		return "", e
	}
	if e = os.MkdirAll(output, 0777); e != nil {
		return "", e
	}
	if e = os.WriteFile(filepath.Join(input, "lesson.py"), []byte(script), 0644); e != nil {
		return "", e
	}
	if e = os.WriteFile(filepath.Join(input, "preflight.py"), []byte(manimPreflight), 0644); e != nil {
		return "", e
	}
	name := "edu-animation-" + id + "-" + fmt.Sprint(attempt)
	runCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "docker", "run", "--rm", "--name", name, "--network", "none", "--read-only", "--user", "1000:1000", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "96", "--memory", "768m", "--cpus", "1", "--tmpfs", "/tmp:rw,noexec,nosuid,size=128m", "--tmpfs", "/output:rw,noexec,nosuid,size=96m,uid=1000,gid=1000", "--mount", "type=bind,src="+input+",dst=/input,readonly", "--env", "HOME=/tmp", s.cfg.AnimationImage, "sh", "-c", "python /input/preflight.py /input/lesson.py && manim -qm --disable_caching --media_dir /output /input/lesson.py Lesson >&2 && base64 /output/videos/lesson/720p30/Lesson.mp4")
	var encoded bytes.Buffer

	logPath := filepath.Join(root, "render.log")
	log, e := os.Create(logPath)
	if e != nil {
		return "", e
	}
	cmd.Stdout = &boundedWriter{w: &encoded, left: 42 << 20}
	cmd.Stderr = &boundedWriter{w: log, left: 1 << 20}
	e = cmd.Run()
	log.Close()
	cleanCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
	exec.CommandContext(cleanCtx, "docker", "rm", "-f", name).Run()
	c()
	if e != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if runCtx.Err() != nil {
			return "", &animationFailure{"timeout", "隔离容器已终止"}
		}
		details, _ := os.ReadFile(logPath)
		kind := "environment"
		var exit *exec.ExitError
		if errors.As(e, &exit) && exit.ExitCode() == 1 {
			kind = "code"
		}
		if bytes.Contains(details, []byte("Traceback")) {
			kind = "code"
		}
		if bytes.Contains(details, []byte("ANIMATION_PREFLIGHT:")) {
			kind = "preflight"
		}
		if len(details) > 2000 {
			details = details[len(details)-2000:]
		}
		return "", &animationFailure{kind, strings.ToValidUTF8(string(details), "")}
	}
	content, e := base64.StdEncoding.DecodeString(encoded.String())
	if e != nil || len(content) > 30<<20 {
		return "", &animationFailure{"media", "渲染输出不是有效的受限MP4数据"}
	}
	found := filepath.Join(output, "Lesson.mp4")
	if e = os.WriteFile(found, content, 0600); e != nil {
		return "", e
	}

	info, e := os.Stat(found)
	if e != nil || info.Size() < 1000 || info.Size() > 30<<20 {
		return "", &animationFailure{"media", "MP4文件大小无效"}
	}
	if e = s.inspectAnimation(ctx, output); e != nil {
		return "", e
	}
	path := storagePath(s, tid, id, ".mp4")
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return "", e
	}
	return path, os.WriteFile(path, content, 0600)
}

// Private bind-mounted media belongs to the backend process. Root with all
// capabilities dropped cannot bypass a different Linux user's 0600/0700 modes.
func mediaSandboxUser() string {
	if os.Geteuid() >= 0 {
		return fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid())
	}
	return "1000:1000" // Windows Docker Desktop maps bind-mount permissions.
}

func (s *Server) inspectAnimation(ctx context.Context, output string) error {
	// Verify media in a separate trusted container: generated scripts cannot forge this check.
	// Match the host owner; keep the mount read-only and drop all capabilities.
	// No model-generated Python is mounted or executed in this container.
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	name := "edu-animation-check-" + newID()
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
	}()
	command := "ffmpeg -v error -xerror -err_detect explode -threads 1 -i /output/Lesson.mp4 -map 0:v:0 -f null - && ffprobe -v error -show_entries stream=codec_name,codec_type,width,height:format=duration -of json /output/Lesson.mp4"
	probe := exec.CommandContext(probeCtx, "docker", "run", "--rm", "--name", name, "--network", "none", "--read-only", "--user", mediaSandboxUser(), "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--memory", "384m", "--cpus", "1", "--pids-limit", "32", "--mount", "type=bind,src="+output+",dst=/output,readonly", s.cfg.AnimationImage, "sh", "-c", command)
	b, e := probe.Output()
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(e, &exit) && exit.ExitCode() == 1 {
			// Keep bounded diagnostics; a nonzero exit may be a permissions error,
			// not damaged frames. Size is context, not proof of media integrity.
			detail := strings.TrimSpace(string(exit.Stderr))
			if detail == "" {
				detail = "MP4帧无法完整解码"
			}
			if len(detail) > 500 {
				detail = detail[len(detail)-500:]
				detail = strings.ToValidUTF8(detail, "")
			}
			if info, statErr := os.Stat(filepath.Join(output, "Lesson.mp4")); statErr == nil {
				detail = fmt.Sprintf("%s [artifact %d bytes]", detail, info.Size())
			}
			return &animationFailure{"media", detail}
		}
		return &animationFailure{"environment", "媒体检查容器未正常完成"}
	}
	return validateAnimationMetadata(b)
}

func validateAnimationMetadata(b []byte) error {
	var metadata struct {
		Streams []struct {
			Codec  string `json:"codec_name"`
			Type   string `json:"codec_type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(b, &metadata) != nil {
		return &animationFailure{"media", "视频元信息无效"}
	}
	duration, e := strconv.ParseFloat(metadata.Format.Duration, 64)
	if e != nil || !(duration >= 20 && duration <= 46) {
		return &animationFailure{"media", "时长必须为20–45秒（编码尾帧容差1秒）"}
	}
	if len(metadata.Streams) != 1 || metadata.Streams[0].Type != "video" || metadata.Streams[0].Codec != "h264" || metadata.Streams[0].Height != 720 || metadata.Streams[0].Width < 1 || metadata.Streams[0].Width > 4096 {
		return &animationFailure{"media", "需要720p静音H.264视频"}
	}
	return nil
}

type boundedWriter struct {
	w    io.Writer
	left int
}

func (b *boundedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if b.left > 0 {
		keep := min(b.left, n)
		_, e := b.w.Write(p[:keep])
		b.left -= keep
		if e != nil {
			return 0, e
		}
	}
	return n, nil
}
