package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema embed.FS

type Config struct {
	AllowSimulatedPayment                                                   bool
	CoursewareReviewPolicy                                                  string
	CoursewareContentReviewDisabled                                         bool
	MinIOEndpoint, MinIOAccessKey, MinIOSecretKey, MinIOBucket, MinIOPrefix string
	RedisAddr, RedisUsername, RedisPassword, RedisPrefix                    string
	MilvusURL, MilvusToken, MilvusDatabase, MilvusCollection                string
	TTS                                                                     ttsConfig

	VisionURL, VisionKey, VisionModel                                                                                                                                                                                 string
	ReviewURL, ReviewKey, ReviewModel                                                                                                                                                                                 string
	CoursewareLLMURL, CoursewareLLMKey, CoursewareLLMModel                                                                                                                                                            string
	AnimationLLMURL, AnimationLLMKey, AnimationLLMModel                                                                                                                                                               string
	DatabaseURL, RuntimeURL, Secret, Mode, SeedPassword, Storage, RenderURL, LLMURL, LLMKey, LLMModel, EvalModel, EmbeddingModel, EmbeddingURL, EmbeddingKey, SearchKey, SearchURL, RerankURL, RerankModel, RerankKey string
}
type Server struct {
	scheduleMu       sync.Mutex
	nextTenant       int64
	nextExportTenant int64
	cloud            cloudServices

	db         *pgxpool.Pool
	cfg        Config
	mux        *http.ServeMux
	http       *http.Client
	reviewHTTP *http.Client
	mediaHTTP  *http.Client
}
type Actor struct {
	UserID      int64    `json:"user_id"`
	MemberID    int64    `json:"member_id"`
	TenantID    int64    `json:"tenant_id"`
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}
type Claims struct {
	UserID   int64  `json:"uid"`
	TenantID int64  `json:"tid"`
	Session  string `json:"sid"`
	jwt.RegisteredClaims
}
type APIError struct {
	Status  int
	Message string
}

func (e APIError) Error() string        { return e.Message }
func fail(status int, msg string) error { return APIError{status, msg} }
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func newID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func asJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 2<<20)
	if e := json.NewDecoder(r.Body).Decode(v); e != nil {
		return fail(400, "请求格式错误")
	}
	return nil
}
func api(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(withCoursewareArtifacts(r.Context()))
		if e := fn(w, r); e != nil {
			var ae APIError
			if errors.As(e, &ae) {
				respond(w, ae.Status, map[string]string{"message": ae.Message})
			} else if errors.Is(e, pgx.ErrNoRows) {
				respond(w, 404, map[string]string{"message": "记录不存在或无权访问"})
			} else {
				slog.Error("request failed", "request_id", r.Context().Value(requestIDKey{}), "error_type", fmt.Sprintf("%T", e))
				respond(w, 500, map[string]string{"message": "暂时无法完成操作，已有内容会保留，请稍后重试"})
			}
		}
	}
}
func NewServer(ctx context.Context, c Config) (*Server, error) {
	if c.CoursewareReviewPolicy != "" && c.CoursewareReviewPolicy != "key" && c.CoursewareReviewPolicy != "all" && c.CoursewareReviewPolicy != "none" {
		return nil, errors.New("COURSEWARE_REVIEW_POLICY must be key, all or none")
	}
	if c.Mode != "sample" && c.Mode != "live" {
		return nil, errors.New("APP_MODE must be sample or live")
	}
	if len(c.Secret) < 32 {
		return nil, errors.New("JWT_SECRET must contain at least 32 characters")
	}
	if c.Storage == "" {
		c.Storage = "../storage"
	}
	if c.RenderURL == "" {
		c.RenderURL = "http://127.0.0.1:8000"
	}
	p, e := pgxpool.New(ctx, c.DatabaseURL)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	sql, _ := schema.ReadFile("schema.sql")
	if _, e = p.Exec(ctx, string(sql)); e != nil {
		p.Close()
		return nil, e
	}
	// The review gateway advertises HTTP/2 but returns invalid initial frames.
	// Keep generation's transport; review JSON uses HTTP/1.1 with verified TLS.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = new(tls.Config)
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	s := &Server{db: p, cfg: c, mux: http.NewServeMux(), http: &http.Client{Timeout: 100 * time.Second}, reviewHTTP: &http.Client{Timeout: 100 * time.Second, Transport: transport}, mediaHTTP: publicHTTPClient(90 * time.Second)}
	if e = os.MkdirAll(c.Storage, 0700); e != nil {
		return nil, e
	}
	if c.SeedPassword != "" {
		if e = s.seed(ctx); e != nil {
			return nil, e
		}
	}
	if c.RuntimeURL != "" {
		runtime, e := pgxpool.New(ctx, c.RuntimeURL)
		if e != nil {
			s.Close()
			return nil, e
		}
		if e = runtime.Ping(ctx); e != nil {
			runtime.Close()
			s.Close()
			return nil, e
		}
		var privileged bool
		e = runtime.QueryRow(ctx, "SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user").Scan(&privileged)
		if e != nil || privileged {
			runtime.Close()
			s.Close()
			return nil, errors.New("runtime database role must not bypass RLS")
		}
		p.Close()
		s.db = runtime
	}
	if err := s.initCloud(ctx); err != nil {
		s.Close()
		return nil, err
	}
	s.routes()
	return s, nil
}
func (s *Server) Close() {
	s.db.Close()
	s.mediaHTTP.CloseIdleConnections()
	if s.reviewHTTP != nil {
		s.reviewHTTP.CloseIdleConnections()
	}
	if s.cloud.redis != nil {
		s.cloud.redis.Close()
	}
}
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveObserved)
}
func (s *Server) tx(ctx context.Context, tenant int64) (pgx.Tx, error) {
	t, e := s.db.Begin(ctx)
	if e != nil {
		return nil, e
	}
	if _, e = t.Exec(ctx, "SET LOCAL ROLE edu_runtime"); e == nil {
		_, e = t.Exec(ctx, "SELECT set_config('app.tenant_id',$1,true)", strconv.FormatInt(tenant, 10))
	}
	if e != nil {
		t.Rollback(ctx)
		return nil, e
	}
	return t, nil
}
func (s *Server) actor(r *http.Request) (Actor, error) {
	var a Actor
	raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	claims := &Claims{}
	token, e := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if e != nil || !token.Valid {
		return a, fail(401, "请登录")
	}
	var active bool
	e = s.db.QueryRow(r.Context(), "SELECT auth_session_active($1,$2)", claims.Session, claims.UserID).Scan(&active)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return a, fail(503, "登录验证服务暂不可用，请稍后重试")
	}
	if errors.Is(e, pgx.ErrNoRows) || !active {
		return a, fail(401, "登录已失效")
	}
	tx, e := s.tx(r.Context(), claims.TenantID)
	if e != nil {
		return a, e
	}
	defer tx.Rollback(r.Context())
	e = tx.QueryRow(r.Context(), "SELECT m.id,m.tenant_id,m.user_id,u.display_name,m.role,m.permissions FROM members m JOIN users u ON m.user_id=u.id WHERE m.user_id=$1 AND m.active", claims.UserID).Scan(&a.MemberID, &a.TenantID, &a.UserID, &a.Name, &a.Role, &a.Permissions)
	if errors.Is(e, pgx.ErrNoRows) {
		return a, fail(403, "当前企业成员已停用")
	}
	if e != nil {
		return a, fail(503, "成员验证服务暂不可用，请稍后重试")
	}
	return a, nil
}
func allowed(a Actor, p string) bool {
	if a.Role == "admin" {
		return true
	}
	for _, x := range a.Permissions {
		if p == x {
			return true
		}
	}
	return false
}
func (s *Server) auth(permission string, fn func(http.ResponseWriter, *http.Request, Actor) error) http.HandlerFunc {
	return api(func(w http.ResponseWriter, r *http.Request) error {
		a, e := s.actor(r)
		if e != nil {
			return e
		}
		// Every task route shares the same ownership and visibility check, including SSE.
		if id := r.PathValue("id"); id != "" && strings.HasPrefix(r.URL.Path, "/api/tasks/") {
			tx, err := s.tx(r.Context(), a.TenantID)
			if err != nil {
				return err
			}
			var visible string
			err = tx.QueryRow(r.Context(), "SELECT id FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", id, a.MemberID).Scan(&visible)
			tx.Rollback(r.Context())
			if err != nil {
				return err
			}
		}
		if permission != "" && !allowed(a, permission) {
			return fail(403, "没有此操作权限")
		}
		return fn(w, r.WithContext(context.WithValue(r.Context(), mediaActorKey{}, a)), a)
	})
}
func (s *Server) token(uid, tid int64, session string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{uid, tid, session, jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(12 * time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}}).SignedString([]byte(s.cfg.Secret))
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var in struct{ Username, Password string }
	if e := decode(r, &in); e != nil {
		return e
	}
	in.Username = strings.TrimSpace(in.Username)
	if len(in.Username) > 40 || len(in.Password) > 72 {
		return fail(401, "用户名或密码错误")
	}
	if err := s.authAttempt(r, "login", in.Username); err != nil {
		return err
	}
	var id int64
	sid := newID()
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	// Database proof verification and session creation hold the same user row lock.
	var authenticated *int64
	e = tx.QueryRow(r.Context(), "SELECT auth_login($1,$2,$3,$4)", in.Username, in.Password, sid, time.Now().Add(12*time.Hour)).Scan(&authenticated)
	if e != nil {
		return e
	}
	if authenticated == nil {
		return fail(401, "用户名或密码错误")
	}
	id = *authenticated
	if err := s.clearLoginAttempts(r, in.Username); err != nil {
		return err
	}
	rows, e := tx.Query(r.Context(), "SELECT id,name,role FROM auth_memberships($1)", id)
	if e != nil {
		return e
	}
	defer rows.Close()
	tenants := []map[string]any{}
	var first int64
	for rows.Next() {
		var tid int64
		var name, role string
		if e = rows.Scan(&tid, &name, &role); e != nil {
			return e
		}
		if first == 0 {
			first = tid
		}
		tenants = append(tenants, map[string]any{"id": tid, "name": name, "role": role})
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if first == 0 {
		return fail(403, "没有可用企业")
	}
	token, e := s.token(id, first, sid)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"token": token, "tenants": tenants, "active_tenant_id": first})
	return nil
}
func (s *Server) seed(ctx context.Context) error {
	h, e := bcrypt.GenerateFromPassword([]byte(s.cfg.SeedPassword), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	for _, x := range []struct {
		ID   int
		Name string
	}{{1, "启明教研"}, {2, "远山学校"}} {
		if _, e = tx.Exec(ctx, "INSERT INTO tenants(id,name) VALUES($1,$2) ON CONFLICT DO NOTHING", x.ID, x.Name); e != nil {
			return e
		}
	}
	// Seed spaces use explicit IDs 1/2; advance, never rewind, the sequence so
	// the first self-registration cannot collide with those existing spaces.
	if _, e = tx.Exec(ctx, `DO $$ BEGIN
   WHILE (SELECT last_value FROM tenants_id_seq) < 3 LOOP
     PERFORM nextval('tenants_id_seq');
   END LOOP;
 END $$`); e != nil {
		return e
	}
	for _, u := range []struct {
		Username, Name, Role string
		Tenant               int
		Balance              int
	}{{"admin", "管理员", "admin", 1, 500}, {"teacher", "林老师", "teacher", 1, 100}, {"low", "周老师", "teacher", 1, 8}, {"other", "远山老师", "teacher", 2, 100}} {
		var uid int64
		e = tx.QueryRow(ctx, "INSERT INTO users(username,password_hash,display_name) VALUES($1,$2,$3) ON CONFLICT(username) DO UPDATE SET username=excluded.username RETURNING id", u.Username, string(h), u.Name).Scan(&uid)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "INSERT INTO members(tenant_id,user_id,role,balance,permissions,plan) VALUES($1,$2,$3,$4,$5,'basic') ON CONFLICT DO NOTHING", u.Tenant, uid, u.Role, u.Balance, asJSON([]string{"courseware.generate", "animation.render", "kb.manage"}))
		if e != nil {
			return e
		}
		if u.Username == "admin" {
			_, e = tx.Exec(ctx, "INSERT INTO members(tenant_id,user_id,role,balance,plan) VALUES(2,$1,'admin',500,'basic') ON CONFLICT DO NOTHING", uid)
			if e != nil {
				return e
			}
		}
	}
	return tx.Commit(ctx)
}
func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", api(func(w http.ResponseWriter, r *http.Request) error {
		deps := s.dependencyHealth(r.Context())
		for _, state := range deps {
			if state != "ok" && state != "local" && state != "disabled" {
				respond(w, 503, map[string]any{"status": "degraded", "dependencies": deps, "release_tag": releaseTag, "release_commit": releaseCommit})
				return nil
			}
		}
		_, _, coursewareModel := s.agentModel("courseware")
		respond(w, 200, map[string]any{"status": "ok", "release_tag": releaseTag, "release_commit": releaseCommit, "courseware_brief_version": "conversation-stream-v1", "workspace_needs_version": "clarify-v1", "workspace_context_version": "rolling-summary-v1", "courseware_subjects_version": "all-subjects-v1", "workspace_brief_version": "tools-inline-v1", "courseware_plot_version": "matplotlib-v1", "courseware_canvas_version": "canvas-v3", "courseware_canvas_text_version": "inline-v1", "courseware_generation_model": coursewareModel, "courseware_speed_version": "parallel-reviews-v2", "courseware_classroom_version": "answer-steps-v2", "courseware_composition_prompt_version": "purpose-v2", "courseware_review_roles_version": "roles-v1", "courseware_generation_guard_version": "quality-guard-v1", "dependencies": deps, "teaching_features_version": "teaching-v1", "workspace_management_version": "management-v2", "courseware_quality_configured": s.coursewareQualityConfigured(), "courseware_review_policy": s.reviewPolicy(), "worker_pool_size": 2, "video_worker_pool_size": 1, "courseware_content_review_enabled": !s.cfg.CoursewareContentReviewDisabled && s.reviewPolicy() != "none", "session_refresh_version": "session-v1", "courseware_media_version": "media-v1", "courseware_studio_version": "studio-v1", "works_view_version": "views-v1", "works_resource_version": "resources-v1", "courseware_skill_version": coursewareSkillVersion, "courseware_page_repair_version": coursewarePageRepairVersion, "courseware_auto_resume_version": "auto-resume-v1", "courseware_workflow_optimization_version": "bounded-drafts-v1", "visual_review_configured": s.cfg.VisionURL != "" && s.cfg.VisionKey != "" && s.cfg.VisionModel != "", "mode": s.cfg.Mode, "generation_configured": s.agentConfigured("courseware"), "courseware_configured": s.agentConfigured("courseware"), "animation_configured": s.htmlAnimationConfigured(), "animation_html_version": "workflow-v1", "animation_service_status": map[bool]string{true: "html_ready", false: "unconfigured"}[s.htmlAnimationConfigured()], "animation_service_message": "明亮课堂HTML动画支持通用讲解、教学示意图与教师配图，支持一次、二次、反比例函数和几何专用演示", "animation_archetypes": []string{"func.params@1", "geo.transform@1", "lesson.explain@1"}, "animation_video_configured": animationVideoConfigured(), "animation_function_version": "functions-v1", "courseware_image_reference_version": "assets-v1", "courseware_function_animation_version": "functions-v1", "animation_geometry_version": "geometry-v1", "animation_tts_configured": s.cfg.TTS.configured(), "search_configured": s.cfg.SearchKey != "", "embedding_configured": s.cfg.EmbeddingModel != ""})
		return nil
	}))
	s.mux.HandleFunc("POST /api/auth/login", api(s.login))
	s.mux.HandleFunc("POST /api/auth/register", api(s.register))
	s.mux.HandleFunc("POST /api/auth/reset-password", api(s.resetPassword))
	s.mux.HandleFunc("POST /api/auth/refresh", s.auth("", s.refreshSession))
	s.mux.HandleFunc("POST /api/account/recovery-code", s.auth("", s.issueRecoveryCode))
	s.mux.HandleFunc("POST /api/media", s.auth("", s.uploadMedia))
	s.mux.HandleFunc("GET /api/media/{asset}", s.auth("", s.mediaFile))
	s.mux.HandleFunc("GET /api/media/{asset}/metadata", s.auth("", s.mediaMetadata))
	s.mux.HandleFunc("POST /api/tasks/{id}/images/search", s.auth("courseware.generate", s.coursewareImagesSearch))
	s.mux.HandleFunc("POST /api/tasks/{id}/images/confirm", s.auth("courseware.generate", s.coursewareImageConfirm))
	s.mux.HandleFunc("POST /api/courseware/brief", s.auth("courseware.generate", s.coursewareBrief))
	s.mux.HandleFunc("POST /api/workspace/brief", s.auth("", s.coursewareBrief))
	s.mux.HandleFunc("GET /api/tasks/{id}/animation", s.auth("", s.animationState))
	s.mux.HandleFunc("GET /api/tasks/{id}/animation/file", s.auth("", s.animationFile))
	s.mux.HandleFunc("POST /api/tasks/{id}/animation/{action}", s.auth("", s.animationAction))
	s.mux.HandleFunc("GET /api/courseware/materials", s.auth("courseware.generate", s.coursewareMaterials))
	s.mux.HandleFunc("POST /api/courseware/formula", s.auth("courseware.generate", s.coursewareFormula))
	s.mux.HandleFunc("POST /api/courseware/scientific", s.auth("courseware.generate", s.coursewareScientific))
	s.mux.HandleFunc("POST /api/courseware/materials/preview", s.auth("courseware.generate", s.materialPreview))
	s.mux.HandleFunc("PATCH /api/documents/{id}/curriculum", s.auth("kb.manage", s.tagCurriculum))
	s.mux.HandleFunc("GET /api/courseware/references", s.auth("courseware.generate", s.listReferences))
	s.mux.HandleFunc("POST /api/courseware/references", s.auth("courseware.generate", s.uploadReference))
	s.mux.HandleFunc("DELETE /api/courseware/references/{id}", s.auth("courseware.generate", s.deleteReference))
	s.mux.HandleFunc("GET /api/account", s.auth("", s.account))
	s.mux.HandleFunc("PATCH /api/account", s.auth("", s.updateAccount))
	s.mux.HandleFunc("POST /api/account/password", s.auth("", s.changePassword))
	s.mux.HandleFunc("POST /api/tasks/{id}/retry", s.auth("courseware.generate", s.retryCourseware))
	s.mux.HandleFunc("POST /api/tasks/{id}/draft", s.auth("courseware.generate", s.saveCoursewareDraft))
	s.mux.HandleFunc("POST /api/tasks/{id}/expand", s.auth("courseware.generate", s.confirmCoursewarePreview))
	s.mux.HandleFunc("POST /api/tasks/{id}/slides/{slide}/review", s.auth("courseware.generate", s.reviewCoursewareSlide))
	s.mux.HandleFunc("POST /api/tasks/{id}/slides/{slide}/feedback", s.auth("courseware.generate", s.feedbackCoursewareSlide))
	s.mux.HandleFunc("GET /api/tasks/{id}/slides/{slide}/preview", s.auth("", s.coursewareSlidePreview))
	s.mux.HandleFunc("GET /api/tasks/{id}/slides/{slide}/objects", s.auth("", s.coursewareObjectMap))
	s.mux.HandleFunc("POST /api/tasks/{id}/slides/{slide}/canvas-preview", s.auth("", s.coursewareCanvasPreview))
	s.mux.HandleFunc("GET /api/tasks/{id}/preview-pptx", s.auth("", s.coursewarePreviewDownload))
	s.mux.HandleFunc("GET /api/tasks/{id}/conversation", s.auth("", s.coursewareConversation))
	s.mux.HandleFunc("POST /api/tasks/{id}/conversation", s.auth("courseware.generate", s.sendCoursewareMessage))
	s.mux.HandleFunc("POST /api/tasks/{id}/conversation/{message}/feedback", s.auth("courseware.generate", s.feedbackCoursewareMessage))
	s.mux.HandleFunc("GET /api/auth/me", s.auth("", func(w http.ResponseWriter, r *http.Request, a Actor) error { respond(w, 200, a); return nil }))
	s.mux.HandleFunc("POST /api/auth/logout", s.auth("", func(w http.ResponseWriter, r *http.Request, a Actor) error {
		c := &Claims{}
		jwt.ParseWithClaims(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), c, func(t *jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil })
		_, e := s.db.Exec(r.Context(), "SELECT auth_session_revoke($1)", c.Session)
		respond(w, 200, map[string]bool{"ok": true})
		return e
	}))
	s.mux.HandleFunc("GET /api/tenants", s.auth("", func(w http.ResponseWriter, r *http.Request, a Actor) error {
		rows, e := s.db.Query(r.Context(), "SELECT id,name FROM auth_memberships($1)", a.UserID)
		if e != nil {
			return e
		}
		defer rows.Close()
		v := []map[string]any{}
		for rows.Next() {
			var id int64
			var name string
			rows.Scan(&id, &name)
			v = append(v, map[string]any{"id": id, "name": name})
		}
		respond(w, 200, v)
		return rows.Err()
	}))
	s.mux.HandleFunc("POST /api/auth/switch-tenant", s.auth("", func(w http.ResponseWriter, r *http.Request, a Actor) error {
		var in struct {
			TenantID int64 `json:"tenant_id"`
		}
		if e := decode(r, &in); e != nil {
			return e
		}
		var ok bool
		if e := s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM auth_memberships($1) WHERE id=$2)", a.UserID, in.TenantID).Scan(&ok); e != nil {
			return e
		}
		if !ok {
			return fail(403, "不属于该企业")
		}
		a.TenantID = in.TenantID
		return s.refreshSession(w, r, a)
	}))
	s.mux.HandleFunc("GET /api/billing", s.auth("", s.billing))
	s.mux.HandleFunc("POST /api/billing/orders", s.auth("", s.createOrder))
	s.mux.HandleFunc("POST /api/billing/orders/{id}/pay", s.auth("", s.payOrder))
	s.mux.HandleFunc("GET /api/tasks", s.auth("", s.listTasks))
	s.mux.HandleFunc("GET /api/tasks/cleanup-status", s.auth("", s.workCleanupStatus))
	s.mux.HandleFunc("POST /api/tasks/{id}/restore", s.auth("", s.restoreWork))
	s.mux.HandleFunc("POST /api/tasks", s.auth("", s.createTask))
	s.mux.HandleFunc("POST /api/tasks/delete", s.auth("", s.deleteWorks))
	s.mux.HandleFunc("GET /api/tasks/{id}", s.auth("", s.getTask))
	s.mux.HandleFunc("PATCH /api/tasks/{id}", s.auth("", s.renameTask))
	s.mux.HandleFunc("DELETE /api/tasks/{id}", s.auth("", s.deleteTask))
	s.mux.HandleFunc("POST /api/tasks/{id}/confirm", s.auth("", s.confirmTask))
	s.mux.HandleFunc("POST /api/tasks/{id}/cancel", s.auth("", s.cancelTask))
	s.mux.HandleFunc("GET /api/tasks/{id}/events", s.auth("", s.events))
	s.mux.HandleFunc("GET /api/tasks/{id}/download/{format}", s.auth("", s.download))
	s.mux.HandleFunc("POST /api/tasks/{id}/review", s.auth("eval.run", s.reviewAgain))
	s.mux.HandleFunc("POST /api/tasks/{id}/accept-animation", s.auth("animation.render", s.acceptAnimation))
	s.mux.HandleFunc("GET /api/tasks/{id}/frames/{frame}", s.auth("", s.animationFrame))
	s.mux.HandleFunc("POST /api/tasks/{id}/edit", s.auth("courseware.generate", s.editTask))
	s.mux.HandleFunc("POST /api/tasks/{id}/rewrite", s.auth("courseware.generate", s.rewriteTask))
	s.mux.HandleFunc("GET /api/tasks/{id}/revisions", s.auth("", s.listRevisions))
	s.mux.HandleFunc("GET /api/tasks/{id}/revisions/{version}", s.auth("", s.getRevision))
	s.mux.HandleFunc("GET /api/tasks/{id}/revisions/{version}/download/{format}", s.auth("", s.download))
	s.mux.HandleFunc("POST /api/tasks/{id}/revisions/{version}/restore", s.auth("courseware.generate", s.editTask))
	s.mux.HandleFunc("POST /api/evaluations", s.auth("eval.run", s.evaluateRetrieval))
	s.mux.HandleFunc("GET /api/documents", s.auth("", s.listDocuments))
	s.mux.HandleFunc("POST /api/documents", s.auth("kb.manage", s.uploadDocument))
	s.mux.HandleFunc("POST /api/documents/{id}/retry", s.auth("kb.manage", s.retryDocument))
	s.mux.HandleFunc("DELETE /api/documents/{id}", s.auth("kb.manage", s.deleteDocument))
	s.mux.HandleFunc("POST /api/documents/url", s.auth("kb.manage", s.importURL))
	s.mux.HandleFunc("POST /api/retrieval/test", s.auth("", s.retrievalTest))
	s.mux.HandleFunc("POST /api/admin/members", s.auth("user.manage", s.createMember))
	s.mux.HandleFunc("POST /api/admin/members/{id}/package", s.auth("billing.manage", s.assignPackage))
	s.mux.HandleFunc("GET /api/admin/members", s.auth("user.manage", s.members))
	s.mux.HandleFunc("PATCH /api/admin/members/{id}", s.auth("user.manage", s.updateMember))
	s.mux.HandleFunc("GET /api/dashboard", s.auth("", s.dashboard))
	s.mux.HandleFunc("GET /api/usage", s.auth("", s.usageRecords))
	s.mux.HandleFunc("POST /api/usage/delete", s.auth("", s.deleteUsageRecords))
	s.mux.Handle("/", http.FileServer(http.Dir(env("FRONTEND_DIST", "../frontend-vue/dist"))))
}
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	log.SetOutput(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Writer())
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		json.NewEncoder(os.Stdout).Encode(map[string]string{"release_tag": releaseTag, "release_commit": releaseCommit})
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if handled, err := runAnimCLI(ctx, os.Args[1:]); handled {
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && !(len(os.Args) == 2 && os.Args[1] == "--sync-cloud") {
		if len(os.Args) != 3 || os.Args[1] != "--reset-seed-password" {
			log.Fatal("Usage: edu-server [--sync-cloud | --reset-seed-password teacher|admin|low|other]")
		}
		if err := resetSeedPassword(ctx, os.Getenv("DATABASE_URL"), os.Args[2], os.Getenv("SEED_PASSWORD")); err != nil {
			slog.Error("password reset failed", "error_type", fmt.Sprintf("%T", err))
			os.Exit(1)
		}
		log.Printf("Password reset for %s; previous sessions revoked. No other account or business data changed.", os.Args[2])
		return
	}
	secret := os.Getenv("JWT_SECRET")
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("RUNTIME_DATABASE_URL") == "" {
		log.Fatal("Set explicit DATABASE_URL and RUNTIME_DATABASE_URL; provision credentials before starting")
	}
	if secret == "" {
		log.Fatal("Set JWT_SECRET (32+ characters); see .env.example")
	}
	c := Config{RuntimeURL: os.Getenv("RUNTIME_DATABASE_URL"), DatabaseURL: os.Getenv("DATABASE_URL"), Secret: secret, Mode: env("APP_MODE", "sample"), SeedPassword: os.Getenv("SEED_PASSWORD"), Storage: env("STORAGE_DIR", "../storage"), RenderURL: env("RENDER_SERVICE_URL", "http://127.0.0.1:8000"), LLMURL: env("LLM_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"), LLMKey: os.Getenv("LLM_API_KEY"), LLMModel: os.Getenv("LLM_MODEL"), EvalModel: os.Getenv("LLM_EVAL_MODEL"), EmbeddingModel: os.Getenv("EMBEDDING_MODEL"), EmbeddingURL: env("EMBEDDING_BASE_URL", env("LLM_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3")), EmbeddingKey: env("EMBEDDING_API_KEY", os.Getenv("LLM_API_KEY")), RerankURL: os.Getenv("RERANK_URL"), RerankModel: os.Getenv("RERANK_MODEL"), RerankKey: os.Getenv("RERANK_API_KEY"), SearchKey: os.Getenv("SEARCH_API_KEY"), SearchURL: env("SEARCH_API_URL", "https://api.tavily.com/search")}
	c.MinIOEndpoint, c.MinIOAccessKey, c.MinIOSecretKey = os.Getenv("MINIO_ENDPOINT"), os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY")
	c.MinIOBucket, c.MinIOPrefix = env("MINIO_BUCKET", "edu-agent"), env("MINIO_PREFIX", "files/")
	c.RedisAddr, c.RedisUsername, c.RedisPassword = os.Getenv("REDIS_ADDR"), os.Getenv("REDIS_USERNAME"), os.Getenv("REDIS_PASSWORD")
	c.RedisPrefix = env("REDIS_PREFIX", "edu-agent:")
	c.MilvusURL, c.MilvusToken = os.Getenv("MILVUS_URL"), os.Getenv("MILVUS_TOKEN")
	c.MilvusDatabase, c.MilvusCollection = env("MILVUS_DATABASE", "edu_agent"), env("MILVUS_COLLECTION", "edu_agent_chunks")
	c.CoursewareLLMURL, c.CoursewareLLMKey, c.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	c.AnimationLLMURL, c.AnimationLLMKey, c.AnimationLLMModel = os.Getenv("ANIMATION_LLM_BASE_URL"), os.Getenv("ANIMATION_LLM_API_KEY"), os.Getenv("ANIMATION_LLM_MODEL")
	c.ReviewURL, c.ReviewKey, c.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	c.AllowSimulatedPayment = env("ALLOW_SIMULATED_PAYMENT", "false") == "true"
	c.CoursewareReviewPolicy = env("COURSEWARE_REVIEW_POLICY", "key")
	c.CoursewareContentReviewDisabled = env("COURSEWARE_CONTENT_REVIEW_DISABLED", "false") == "true"
	c.VisionURL, c.VisionKey, c.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	c.TTS = ttsConfigFromEnv()
	s, e := NewServer(ctx, c)
	if e != nil {
		slog.Error("backend initialization failed; connection details suppressed", "error_type", fmt.Sprintf("%T", e))
		os.Exit(1)
	}
	defer s.Close()
	if len(os.Args) == 2 && os.Args[1] == "--sync-cloud" {
		if err := s.syncCloud(ctx); err != nil {
			log.Fatal(err)
		}
		return
	}
	go watchShutdownFile(ctx, os.Getenv("SHUTDOWN_REQUEST_FILE"), cancel)
	workersDone := make(chan struct{})
	go func() { defer close(workersDone); s.worker(ctx) }()
	go s.documentWorker(ctx)
	go s.deletedWorkCleaner(ctx)
	server := &http.Server{Addr: env("LISTEN_ADDR", "127.0.0.1:8080"), Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		x, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		server.Shutdown(x)
	}()
	log.Printf("education backend listening on %s (mode=%s)", server.Addr, c.Mode)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
	cancel()
	<-workersDone
}
func storagePath(s *Server, tid int64, id, ext string) string {
	return filepath.Join(s.cfg.Storage, fmt.Sprintf("%d", tid), id+ext)
}
