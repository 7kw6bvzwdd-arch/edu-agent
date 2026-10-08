package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema embed.FS

type Config struct {
	VisionURL, VisionKey, VisionModel                                                                                                                                                                                                 string
	ReviewURL, ReviewKey, ReviewModel                                                                                                                                                                                                 string
	VideoProvider                                                                                                                                                                                                                     string
	CoursewareLLMURL, CoursewareLLMKey, CoursewareLLMModel                                                                                                                                                                            string
	AnimationLLMURL, AnimationLLMKey, AnimationLLMModel                                                                                                                                                                               string
	VideoURL, VideoKey, VideoModel, VideoDuration                                                                                                                                                                                     string
	DatabaseURL, RuntimeURL, Secret, Mode, SeedPassword, Storage, RenderURL, LLMURL, LLMKey, LLMModel, EvalModel, EmbeddingModel, EmbeddingURL, EmbeddingKey, SearchKey, SearchURL, RerankURL, RerankModel, RerankKey, AnimationImage string
}
type Server struct {
	db        *pgxpool.Pool
	cfg       Config
	mux       *http.ServeMux
	http      *http.Client
	mediaHTTP *http.Client
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
		if e := fn(w, r); e != nil {
			var ae APIError
			if errors.As(e, &ae) {
				respond(w, ae.Status, map[string]string{"message": ae.Message})
			} else if errors.Is(e, pgx.ErrNoRows) {
				respond(w, 404, map[string]string{"message": "记录不存在或无权访问"})
			} else {
				log.Printf("request %s %s: %v", r.Method, r.URL.Path, e)
				respond(w, 500, map[string]string{"message": "操作失败，请查看服务端日志"})
			}
		}
	}
}
func NewServer(ctx context.Context, c Config) (*Server, error) {
	if c.VideoProvider == "" {
		c.VideoProvider = "ark"
	}
	if c.VideoURL == "" && c.VideoProvider == "minimax" {
		c.VideoURL = "https://api.minimax.cn"
	}
	if c.VideoURL == "" && c.VideoProvider == "ark" {
		c.VideoURL = c.LLMURL
	}
	if c.VideoKey == "" && c.VideoProvider == "ark" {
		c.VideoKey = c.LLMKey
	}
	if c.VideoDuration == "" {
		c.VideoDuration = "5"
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
	s := &Server{db: p, cfg: c, mux: http.NewServeMux(), http: &http.Client{Timeout: 100 * time.Second}, mediaHTTP: publicHTTPClient(90 * time.Second)}
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
	s.routes()
	return s, nil
}
func (s *Server) Close() { s.db.Close(); s.mediaHTTP.CloseIdleConnections() }
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		s.mux.ServeHTTP(w, r)
	})
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
	e = s.db.QueryRow(r.Context(), "SELECT u.active FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=$1 AND s.user_id=$2 AND s.expires_at>now()", claims.Session, claims.UserID).Scan(&active)
	if e != nil || !active {
		return a, fail(401, "登录已失效")
	}
	tx, e := s.tx(r.Context(), claims.TenantID)
	if e != nil {
		return a, e
	}
	defer tx.Rollback(r.Context())
	e = tx.QueryRow(r.Context(), "SELECT m.id,m.tenant_id,m.user_id,u.display_name,m.role,m.permissions FROM members m JOIN users u ON m.user_id=u.id WHERE m.user_id=$1 AND m.active", claims.UserID).Scan(&a.MemberID, &a.TenantID, &a.UserID, &a.Name, &a.Role, &a.Permissions)
	if e != nil {
		return a, fail(403, "当前企业成员已停用")
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
		return fn(w, r, a)
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
	var id int64
	var hash string
	var active bool
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	// Serialize credential verification/session creation with password changes.
	e = tx.QueryRow(r.Context(), "SELECT id,password_hash,active FROM users WHERE username=$1 FOR SHARE", in.Username).Scan(&id, &hash, &active)
	if e != nil || !active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		return fail(401, "用户名或密码错误")
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
	sid := newID()
	_, e = tx.Exec(r.Context(), "INSERT INTO sessions VALUES($1,$2,$3)", sid, id, time.Now().Add(12*time.Hour))
	if e != nil {
		return e
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
		e := s.db.Ping(r.Context())
		if e != nil {
			return e
		}
		duration, err := strconv.Atoi(s.cfg.VideoDuration)
		if err != nil || duration < 2 || duration > 15 {
			duration = 5
		}
		respond(w, 200, map[string]any{"status": "ok", "courseware_media_version": "media-v1", "courseware_studio_version": "studio-v1", "works_view_version": "views-v1", "animation_quality_version": "scene-v1", "visual_review_configured": s.cfg.VisionURL != "" && s.cfg.VisionKey != "" && s.cfg.VisionModel != "", "mode": s.cfg.Mode, "generation_configured": s.agentConfigured("courseware"), "courseware_configured": s.agentConfigured("courseware"), "animation_configured": s.agentConfigured("animation"), "animation_video_configured": s.videoConfigured(), "animation_video_duration": duration, "search_configured": s.cfg.SearchKey != "", "embedding_configured": s.cfg.EmbeddingModel != ""})
		return nil
	}))
	s.mux.HandleFunc("POST /api/auth/login", api(s.login))
	s.mux.HandleFunc("GET /api/account", s.auth("", s.account))
	s.mux.HandleFunc("PATCH /api/account", s.auth("", s.updateAccount))
	s.mux.HandleFunc("POST /api/account/password", s.auth("", s.changePassword))
	s.mux.HandleFunc("POST /api/tasks/{id}/retry", s.auth("courseware.generate", s.retryCourseware))
	s.mux.HandleFunc("POST /api/tasks/{id}/draft", s.auth("courseware.generate", s.saveCoursewareDraft))
	s.mux.HandleFunc("POST /api/tasks/{id}/expand", s.auth("courseware.generate", s.confirmCoursewarePreview))
	s.mux.HandleFunc("POST /api/tasks/{id}/slides/{slide}/review", s.auth("courseware.generate", s.reviewCoursewareSlide))
	s.mux.HandleFunc("POST /api/tasks/{id}/slides/{slide}/feedback", s.auth("courseware.generate", s.feedbackCoursewareSlide))
	s.mux.HandleFunc("GET /api/tasks/{id}/slides/{slide}/preview", s.auth("", s.coursewareSlidePreview))
	s.mux.HandleFunc("GET /api/tasks/{id}/preview-pptx", s.auth("", s.coursewarePreviewDownload))
	s.mux.HandleFunc("GET /api/tasks/{id}/conversation", s.auth("", s.coursewareConversation))
	s.mux.HandleFunc("POST /api/tasks/{id}/conversation", s.auth("courseware.generate", s.sendCoursewareMessage))
	s.mux.HandleFunc("POST /api/tasks/{id}/conversation/{message}/feedback", s.auth("courseware.generate", s.feedbackCoursewareMessage))
	s.mux.HandleFunc("GET /api/auth/me", s.auth("", func(w http.ResponseWriter, r *http.Request, a Actor) error { respond(w, 200, a); return nil }))
	s.mux.HandleFunc("POST /api/auth/logout", s.auth("", func(w http.ResponseWriter, r *http.Request, a Actor) error {
		c := &Claims{}
		jwt.ParseWithClaims(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), c, func(t *jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil })
		_, e := s.db.Exec(r.Context(), "DELETE FROM sessions WHERE id=$1", c.Session)
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
		c := &Claims{}
		jwt.ParseWithClaims(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), c, func(t *jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil })
		token, e := s.token(a.UserID, in.TenantID, c.Session)
		respond(w, 200, map[string]string{"token": token})
		return e
	}))
	s.mux.HandleFunc("GET /api/billing", s.auth("", s.billing))
	s.mux.HandleFunc("POST /api/billing/orders", s.auth("", s.createOrder))
	s.mux.HandleFunc("POST /api/billing/orders/{id}/pay", s.auth("", s.payOrder))
	s.mux.HandleFunc("GET /api/tasks", s.auth("", s.listTasks))
	s.mux.HandleFunc("POST /api/tasks", s.auth("", s.createTask))
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
	s.mux.Handle("/", http.FileServer(http.Dir(env("FRONTEND_DIST", "../frontend-vue/dist"))))
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if len(os.Args) > 1 {
		if len(os.Args) != 3 || os.Args[1] != "--reset-seed-password" {
			log.Fatal("Usage: edu-server [--reset-seed-password teacher|admin|low|other]")
		}
		if err := resetSeedPassword(ctx, env("DATABASE_URL", "postgres://edu:edu-local-only@127.0.0.1:55432/edu?sslmode=disable"), os.Args[2], os.Getenv("SEED_PASSWORD")); err != nil {
			log.Fatal(err)
		}
		log.Printf("Password reset for %s; previous sessions revoked. No other account or business data changed.", os.Args[2])
		return
	}
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("Set JWT_SECRET (32+ characters); see .env.example")
	}
	c := Config{RuntimeURL: env("RUNTIME_DATABASE_URL", "postgres://edu_app:edu-runtime-local-only@127.0.0.1:55432/edu?sslmode=disable"), DatabaseURL: env("DATABASE_URL", "postgres://edu:edu-local-only@127.0.0.1:55432/edu?sslmode=disable"), Secret: secret, Mode: env("APP_MODE", "sample"), SeedPassword: os.Getenv("SEED_PASSWORD"), Storage: env("STORAGE_DIR", "../storage"), RenderURL: env("RENDER_SERVICE_URL", "http://127.0.0.1:8000"), LLMURL: env("LLM_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"), LLMKey: os.Getenv("LLM_API_KEY"), LLMModel: os.Getenv("LLM_MODEL"), EvalModel: os.Getenv("LLM_EVAL_MODEL"), EmbeddingModel: os.Getenv("EMBEDDING_MODEL"), EmbeddingURL: env("EMBEDDING_BASE_URL", env("LLM_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3")), EmbeddingKey: env("EMBEDDING_API_KEY", os.Getenv("LLM_API_KEY")), RerankURL: os.Getenv("RERANK_URL"), RerankModel: os.Getenv("RERANK_MODEL"), RerankKey: os.Getenv("RERANK_API_KEY"), SearchKey: os.Getenv("SEARCH_API_KEY"), SearchURL: env("SEARCH_API_URL", "https://api.tavily.com/search"), AnimationImage: env("ANIMATION_IMAGE", "edu-manim:local")}
	c.CoursewareLLMURL, c.CoursewareLLMKey, c.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	c.AnimationLLMURL, c.AnimationLLMKey, c.AnimationLLMModel = os.Getenv("ANIMATION_LLM_BASE_URL"), os.Getenv("ANIMATION_LLM_API_KEY"), os.Getenv("ANIMATION_LLM_MODEL")
	c.ReviewURL, c.ReviewKey, c.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	c.VisionURL, c.VisionKey, c.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	c.VideoProvider = env("VIDEO_PROVIDER", "ark")
	c.VideoURL, c.VideoKey, c.VideoModel, c.VideoDuration = env("VIDEO_BASE_URL", os.Getenv("ARK_VIDEO_BASE_URL")), env("VIDEO_API_KEY", os.Getenv("ARK_VIDEO_API_KEY")), env("VIDEO_MODEL", os.Getenv("ARK_VIDEO_MODEL")), env("VIDEO_DURATION", env("ARK_VIDEO_DURATION", "5"))
	s, e := NewServer(ctx, c)
	if e != nil {
		log.Fatal(e)
	}
	defer s.Close()
	go s.worker(ctx)
	go s.documentWorker(ctx)
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
}
func storagePath(s *Server, tid int64, id, ext string) string {
	return filepath.Join(s.cfg.Storage, fmt.Sprintf("%d", tid), id+ext)
}
