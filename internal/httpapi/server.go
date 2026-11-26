package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sourcebox/sourcebox/internal/config"
	"github.com/sourcebox/sourcebox/internal/github"
	"github.com/sourcebox/sourcebox/internal/icons"
	"github.com/sourcebox/sourcebox/internal/refresh"
	"github.com/sourcebox/sourcebox/internal/source"
	"github.com/sourcebox/sourcebox/internal/store"
)

const (
	cookieName   = "sb_session"
	sessionSalt  = "sourcebox-session-v1"
	sessionAge   = 30 * 24 * 3600
	maxBodyBytes = 64 << 10
	adminPathKey = "admin_path"
	maxLoginFail = 8
	loginLockout = 15 * time.Minute
)

var (
	tintPattern      = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	adminPathPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,63}$`)
)

var reservedPaths = map[string]bool{
	"api": true, "assets": true, "health": true, "icons": true,
	"source": true, "source.json": true, "static": true, "admin": true,
}

// Server implements the HTTP surface (API, source JSON, icons, static web app).
type Server struct {
	cfg       config.Config
	db        *store.Store
	factory   refresh.Factory
	adminPath string
	limiter   *loginLimiter
}

// New builds the handler set.
func New(cfg config.Config, db *store.Store, factory refresh.Factory) *Server {
	server := &Server{cfg: cfg, db: db, factory: factory, limiter: newLoginLimiter()}
	server.resolveAdminPath()
	return server
}

// AdminURL is the private entry point of the dashboard.
func (s *Server) AdminURL() string {
	return s.cfg.BaseURL + "/" + s.adminPath + "/"
}

// SourceURL is the public endpoint handed to AltStore and SideStore clients.
func (s *Server) SourceURL() string {
	return s.sourceBase() + "/source.json"
}

// sourceBase is the public prefix shared by the dashboard and the source
// document: /<adminPath>.
func (s *Server) sourceBase() string {
	return s.cfg.BaseURL + "/" + s.adminPath
}

// refreshConfig points the icon builder at the public prefix, so cached app
// entries carry the same random segment as the source document.
func (s *Server) refreshConfig() config.Config {
	cfg := s.cfg
	cfg.BaseURL = s.sourceBase()
	return cfg
}

func (s *Server) resolveAdminPath() {
	if candidate := normalizeAdminPath(s.cfg.AdminPath); candidate != "" && !reservedPaths[candidate] {
		s.adminPath = candidate
		return
	}

	kv, err := s.db.KVAll()
	if err == nil {
		if stored := kv[adminPathKey]; adminPathPattern.MatchString(stored) && !reservedPaths[stored] {
			s.adminPath = stored
			return
		}
	}

	generated := "p-" + randomString(16)
	if err := s.db.KVSet(adminPathKey, generated); err != nil {
		log.Printf("SourceBox: could not persist the admin path: %v", err)
	}
	s.adminPath = generated
}

func normalizeAdminPath(raw string) string {
	text := strings.ToLower(strings.Trim(strings.TrimSpace(raw), "/"))
	if adminPathPattern.MatchString(text) {
		return text
	}
	return ""
}

func randomString(length int) string {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "p-fallback-path"
	}
	for index := range buffer {
		buffer[index] = alphabet[int(buffer[index])%len(alphabet)]
	}
	return string(buffer)
}

type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]loginEntry
}

type loginEntry struct {
	fails int
	until time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: map[string]loginEntry{}}
}

func (l *loginLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok {
		return false
	}
	if time.Now().After(entry.until) {
		delete(l.entries, key)
		return false
	}
	return entry.fails >= maxLoginFail
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	entry.fails++
	entry.until = time.Now().Add(loginLockout)
	l.entries[key] = entry
}

func (l *loginLimiter) clear(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Recover turns a panicking handler into a 500 instead of a dropped connection.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, recovered, debug.Stack())
				writeError(w, http.StatusInternalServerError, "服务器内部错误。")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ------------------------------------------------------------- plumbing

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"detail": message})
}

func decodeJSON(r *http.Request, out any) error {
	if contentType := strings.ToLower(r.Header.Get("Content-Type")); !strings.HasPrefix(contentType, "application/json") {
		return errors.New("请求体需要是 JSON。")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	if err := decoder.Decode(out); err != nil {
		return errors.New("请发送合法的 JSON 请求体。")
	}
	return nil
}

func sessionValue(token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(sessionSalt))
	return hex.EncodeToString(mac.Sum(nil))
}

func constantTimeEqual(left, right string) bool {
	return hmac.Equal([]byte(left), []byte(right))
}

func (s *Server) authed(r *http.Request) bool {
	if s.cfg.AdminToken == "" {
		return true
	}
	if cookie, err := r.Cookie(cookieName); err == nil {
		if constantTimeEqual(cookie.Value, sessionValue(s.cfg.AdminToken)) {
			return true
		}
	}
	header := r.Header.Get("Authorization")
	if len(header) > 7 && strings.EqualFold(header[:7], "bearer ") {
		return constantTimeEqual(strings.TrimSpace(header[7:]), s.cfg.AdminToken)
	}
	return false
}

func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			writeError(w, http.StatusUnauthorized, "请先登录再管理这个源。")
			return
		}
		next(w, r)
	}
}

func (s *Server) ttl() time.Duration {
	kv, err := s.db.KVAll()
	if err != nil {
		return refresh.TTL(s.cfg, nil)
	}
	return refresh.TTL(s.cfg, kv)
}

// --------------------------------------------------------------- routing

// ServeHTTP keeps the dashboard behind a secret path and everything else minimal.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.setSecurityHeaders(w)
	path := r.URL.Path
	prefix := "/" + s.adminPath

	if path == prefix || strings.HasPrefix(path, prefix+"/") {
		rest := strings.TrimPrefix(path, prefix)
		switch {
		case rest == "" || rest == "/":
			s.serveIndex(w, r)
		case rest == "/api" || strings.HasPrefix(rest, "/api/"):
			w.Header().Set("Cache-Control", "no-store")
			s.routeAPI(w, cloneWithPath(r, rest))
		case rest == "/runtime-config.js":
			s.handleRuntimeConfig(w, r)
		case rest == "/source.json":
			s.handleSource(w, r)
		case rest == "/source-icon.png":
			s.handleSourceIcon(w, r)
		case strings.HasPrefix(rest, "/icons/") && strings.HasSuffix(rest, ".png"):
			s.handleRepoIcon(w, cloneWithPath(r, rest))
		case strings.HasSuffix(rest, ".json"):
			s.handleNamedSource(w, cloneWithPath(r, rest))
		default:
			s.serveWebFile(w, r, rest)
		}
		return
	}

	s.routePublic(w, r)
}

func (s *Server) setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
}

func cloneWithPath(r *http.Request, path string) *http.Request {
	clone := r.Clone(r.Context())
	urlCopy := *r.URL
	urlCopy.Path = path
	clone.URL = &urlCopy
	return clone
}

// routeAPI serves the authenticated dashboard API.
func (s *Server) routeAPI(w http.ResponseWriter, r *http.Request) {
	switch path := r.URL.Path; {
	case path == "/api/me":
		s.handleMe(w, r)
	case path == "/api/login":
		s.handleLogin(w, r)
	case path == "/api/logout":
		s.handleLogout(w, r)
	case path == "/api/overview":
		s.guard(s.handleOverview)(w, r)
	case path == "/api/repos":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.guard(s.handleAddRepo)(w, r)
	case path == "/api/repos/refresh":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.guard(s.handleRefreshAll)(w, r)
	case path == "/api/repos/reorder":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.guard(s.handleReorder)(w, r)
	case strings.HasPrefix(path, "/api/repos/"):
		s.handleRepoByID(w, r)
	case path == "/api/source":
		s.guard(s.handleSourceConfig)(w, r)
	case path == "/api/preview":
		s.guard(s.handlePreview)(w, r)
	case path == "/api/qr.svg":
		s.guard(s.handleQR)(w, r)
	default:
		writeError(w, http.StatusNotFound, "页面不存在。")
	}
}

// routePublic keeps a single unauthenticated liveness probe. Everything a
// client needs lives behind the random source path.
func (s *Server) routePublic(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health":
		s.handleHealth(w, r)
	default:
		http.NotFound(w, r)
	}
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "不允许该请求方法。")
}

// ------------------------------------------------------------- public

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	repos, err := s.db.ListRepos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据库失败。")
		return
	}
	// /health stays unauthenticated, so it must not echo the random segment
	// that guards both the dashboard and the source document.
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"version":      Version,
		"repositories": len(repos),
		"apps":         statsFromRepos(repos).Apps,
		"time":         time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	})
}

func (s *Server) handleSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	s.writeSource(w)
}

func (s *Server) handleNamedSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".json")
	kv, _ := s.db.KVAll()
	if name != sourceFromDB(s.cfg, s.adminPath, kv).Slug {
		writeError(w, http.StatusNotFound, "未知的源。")
		return
	}
	s.writeSource(w)
}

func (s *Server) writeSource(w http.ResponseWriter) {
	ttl := s.ttl()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	refresh.EnsureFresh(ctx, s.refreshConfig(), s.db, ttl, false, s.factory)

	repos, err := s.db.ListRepos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据库失败。")
		return
	}
	kv, _ := s.db.KVAll()
	document := buildDocument(s.cfg, s.adminPath, kv, repos)

	maxAge := int(min(ttl.Seconds(), 600))
	if maxAge < 0 {
		maxAge = 0
	}
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge))
	writeJSON(w, http.StatusOK, document)
}

func (s *Server) handleRepoIcon(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/icons/"), ".png")
	if slug == "" || strings.Contains(slug, "/") {
		writeError(w, http.StatusNotFound, "未知图标。")
		return
	}
	repo, err := s.db.GetRepoBySlug(slug)
	if err != nil {
		writeError(w, http.StatusNotFound, "未知图标。")
		return
	}
	data, media := icons.RepoIcon(s.cfg, repo)
	if data == nil {
		writeError(w, http.StatusInternalServerError, "生成图标失败。")
		return
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleSourceIcon(w http.ResponseWriter, r *http.Request) {
	kv, _ := s.db.KVAll()
	view := sourceFromDB(s.cfg, s.adminPath, kv)
	data := icons.SourceIcon(s.cfg, view.Name, view.TintColor)
	if data == nil {
		writeError(w, http.StatusInternalServerError, "生成图标失败。")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// -------------------------------------------------------------- session

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":               Version,
		"authRequired":          s.cfg.AdminToken != "",
		"authed":                s.authed(r),
		"githubTokenConfigured": s.cfg.GitHubToken != "",
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.cfg.AdminToken == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	key := clientKey(r)
	if s.limiter.blocked(key) {
		writeError(w, http.StatusTooManyRequests, "尝试次数过多，请稍后再试。")
		return
	}
	if !constantTimeEqual(strings.TrimSpace(payload.Token), s.cfg.AdminToken) {
		s.limiter.fail(key)
		writeError(w, http.StatusUnauthorized, "密码不正确。")
		return
	}
	s.limiter.clear(key)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sessionValue(s.cfg.AdminToken),
		Path:     "/",
		MaxAge:   sessionAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.SecureCookies(),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.SecureCookies(),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ------------------------------------------------------------- dashboard

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	repos, err := s.db.ListRepos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据库失败。")
		return
	}
	kv, _ := s.db.KVAll()
	views := make([]repoView, 0, len(repos))
	for _, repo := range repos {
		views = append(views, repoToView(s.cfg, s.adminPath, repo))
	}
	writeJSON(w, http.StatusOK, overviewView{
		BaseURL:               s.cfg.BaseURL,
		AdminPath:             "/" + s.adminPath + "/",
		Source:                sourceFromDB(s.cfg, s.adminPath, kv),
		Stats:                 statsFromRepos(repos),
		Repos:                 views,
		AuthRequired:          s.cfg.AdminToken != "",
		GitHubTokenConfigured: s.cfg.GitHubToken != "",
	})
}

func (s *Server) handleAddRepo(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		URL       string `json:"url"`
		TintColor string `json:"tintColor"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	owner, name, err := github.ParseRepoURL(payload.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	canonical := "https://github.com/" + owner + "/" + name
	if _, err := s.db.GetRepoByURL(canonical); err == nil {
		writeError(w, http.StatusConflict, "这个仓库已经在源里了。")
		return
	}

	tint, err := normalizeTint(payload.TintColor)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	taken, err := s.db.TakenSlugs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据库失败。")
		return
	}

	repo, err := s.db.CreateRepo(store.Repo{
		URL:         canonical,
		Owner:       owner,
		Name:        name,
		Slug:        source.UniqueSlug(name, taken),
		DisplayName: name,
		TintColor:   tint,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存仓库失败。")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	updated, err := refresh.RefreshRepo(ctx, s.refreshConfig(), s.db, repo, s.factory)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "已保存，但首次刷新失败。")
		return
	}
	writeJSON(w, http.StatusCreated, repoToView(s.cfg, s.adminPath, updated))
}

// handleRepoByID covers GET / PATCH / DELETE /api/repos/{id} and POST .../refresh.
func (s *Server) handleRepoByID(w http.ResponseWriter, r *http.Request) {
	if !s.authed(r) {
		writeError(w, http.StatusUnauthorized, "请先登录再管理这个源。")
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/repos/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "仓库不存在。")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "仓库不存在。")
		return
	}

	if len(parts) == 2 {
		if parts[1] != "refresh" || r.Method != http.MethodPost {
			writeError(w, http.StatusNotFound, "页面不存在。")
			return
		}
		s.handleRefreshOne(w, r, id)
		return
	}
	if len(parts) > 1 {
		writeError(w, http.StatusNotFound, "页面不存在。")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleRepoDetail(w, id)
	case http.MethodPatch:
		s.handlePatchRepo(w, r, id)
	case http.MethodDelete:
		s.handleDeleteRepo(w, id)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (s *Server) handleRepoDetail(w http.ResponseWriter, id int64) {
	repo, err := s.db.GetRepo(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "仓库不存在。")
		return
	}
	status := repo.Cache.Status
	if status == "" {
		status = refresh.StatusPending
	}
	writeJSON(w, http.StatusOK, detailView{
		Repo:    repoToView(s.cfg, s.adminPath, repo),
		Apps:    repo.Cache.Apps,
		Release: repo.Cache.Release,
		Error:   nonEmpty(repo.Cache.Error),
		Status:  status,
	})
}

func (s *Server) handlePatchRepo(w http.ResponseWriter, r *http.Request, id int64) {
	repo, err := s.db.GetRepo(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "仓库不存在。")
		return
	}

	var payload struct {
		DisplayName *string `json:"displayName"`
		TintColor   *string `json:"tintColor"`
		IconURL     *string `json:"iconUrl"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fields := map[string]any{}
	if payload.DisplayName != nil {
		name := strings.TrimSpace(*payload.DisplayName)
		if name == "" {
			name = repo.Name
		}
		fields["display_name"] = name
	}
	if payload.TintColor != nil {
		tint, err := normalizeTint(*payload.TintColor)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		fields["tint_color"] = tint
	}
	if payload.IconURL != nil {
		fields["icon_url"] = strings.TrimSpace(*payload.IconURL)
		icons.DropIcon(s.cfg, repo.Slug)
	}

	if len(fields) > 0 {
		if err := s.db.UpdateRepo(id, fields); err != nil {
			writeError(w, http.StatusInternalServerError, "保存修改失败。")
			return
		}
		updated, err := s.db.GetRepo(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "读取仓库失败。")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		updated, err = refresh.RefreshRepo(ctx, s.refreshConfig(), s.db, updated, s.factory)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "已保存，但刷新失败。")
			return
		}
		writeJSON(w, http.StatusOK, repoToView(s.cfg, s.adminPath, updated))
		return
	}

	writeJSON(w, http.StatusOK, repoToView(s.cfg, s.adminPath, repo))
}

func (s *Server) handleDeleteRepo(w http.ResponseWriter, id int64) {
	repo, err := s.db.GetRepo(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "仓库不存在。")
		return
	}
	if err := s.db.DeleteRepo(id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除仓库失败。")
		return
	}
	icons.DropIcon(s.cfg, repo.Slug)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRefreshOne(w http.ResponseWriter, r *http.Request, id int64) {
	repo, err := s.db.GetRepo(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "仓库不存在。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	updated, err := refresh.RefreshRepo(ctx, s.refreshConfig(), s.db, repo, s.factory)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "刷新失败。")
		return
	}
	writeJSON(w, http.StatusOK, repoToView(s.cfg, s.adminPath, updated))
}

func (s *Server) handleRefreshAll(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	count := refresh.EnsureFresh(ctx, s.refreshConfig(), s.db, s.ttl(), true, s.factory)

	repos, err := s.db.ListRepos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据库失败。")
		return
	}
	views := make([]repoView, 0, len(repos))
	for _, repo := range repos {
		views = append(views, repoToView(s.cfg, s.adminPath, repo))
	}
	writeJSON(w, http.StatusOK, map[string]any{"refreshed": count, "repos": views})
}

func (s *Server) handleReorder(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	repos, err := s.db.ListRepos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据库失败。")
		return
	}
	known := make(map[int64]bool, len(repos))
	for _, repo := range repos {
		known[repo.ID] = true
	}
	for _, id := range payload.IDs {
		if !known[id] {
			writeError(w, http.StatusBadRequest, "新顺序里包含未知的仓库 ID。")
			return
		}
	}
	if err := s.db.Reorder(payload.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, "保存新顺序失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --------------------------------------------------------------- settings

func (s *Server) handleSourceConfig(w http.ResponseWriter, r *http.Request) {
	kv, err := s.db.KVAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据库失败。")
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, sourceFromDB(s.cfg, s.adminPath, kv))
	case http.MethodPut:
		var payload struct {
			Name         string `json:"name"`
			Slug         string `json:"slug"`
			Subtitle     string `json:"subtitle"`
			Description  string `json:"description"`
			TintColor    string `json:"tintColor"`
			CacheSeconds int    `json:"cacheSeconds"`
		}
		if err := decodeJSON(r, &payload); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		tint, err := normalizeTint(payload.TintColor)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			name = "SourceBox"
		}
		updates := map[string]string{
			"source_name":        name,
			"source_slug":        store.Slugify(payload.Slug),
			"source_subtitle":    strings.TrimSpace(payload.Subtitle),
			"source_description": strings.TrimSpace(payload.Description),
			"source_tint":        tint,
			"cache_seconds":      strconv.Itoa(max(payload.CacheSeconds, 0)),
		}
		for key, value := range updates {
			if err := s.db.KVSet(key, value); err != nil {
				writeError(w, http.StatusInternalServerError, "保存设置失败。")
				return
			}
		}
		icons.DropIcon(s.cfg, "_source")
		refreshed, _ := s.db.KVAll()
		writeJSON(w, http.StatusOK, sourceFromDB(s.cfg, s.adminPath, refreshed))
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	repos, err := s.db.ListRepos()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取数据库失败。")
		return
	}
	kv, _ := s.db.KVAll()
	writeJSON(w, http.StatusOK, buildDocument(s.cfg, s.adminPath, kv, repos))
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	payload := r.URL.Query().Get("data")
	if payload == "" || len(payload) > 2048 {
		writeError(w, http.StatusBadRequest, "数据值不能超过 2048 个字符。")
		return
	}
	svg, err := icons.QRSVG(payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "该值编码失败。")
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, svg)
}

// ---------------------------------------------------------------- static

const contentSecurityPolicy = "default-src 'self'; img-src 'self' data:; " +
	"style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; " +
	"font-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// serveIndex serves the SPA shell with every absolute asset path rewritten to
// live under the secret prefix.
func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	raw, err := os.ReadFile(filepath.Join(s.cfg.WebDir, "index.html"))
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w,
			"<h1>SourceBox</h1><p>The web app is not built. Run <code>cd web &amp;&amp; npm install &amp;&amp; npm run build</code>, "+
				"or use <code>npm run dev</code> on port 5173.</p>")
		return
	}

	prefix := "/" + s.adminPath
	page := string(raw)
	page = strings.ReplaceAll(page, `="/assets/`, `="`+prefix+`/assets/`)
	page = strings.ReplaceAll(page, `href="/favicon.svg"`, `href="`+prefix+`/favicon.svg"`)
	page = strings.Replace(page, "</head>", `<script src="`+prefix+`/runtime-config.js"></script></head>`, 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, page)
}

// handleRuntimeConfig tells the SPA where its API lives.
func (s *Server) handleRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, `window.__SOURCEBOX_BASE__="/`+s.adminPath+`";`)
}

func (s *Server) serveWebFile(w http.ResponseWriter, r *http.Request, rest string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	clean := filepath.Clean("/" + rest)
	candidate := filepath.Join(s.cfg.WebDir, filepath.FromSlash(clean))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		http.ServeFile(w, r, candidate)
		return
	}
	if strings.Contains(filepath.Base(clean), ".") {
		http.NotFound(w, r)
		return
	}
	s.serveIndex(w, r)
}

func normalizeTint(value string) (string, error) {
	text := strings.ToUpper(strings.TrimSpace(value))
	if text == "" {
		return config.DefaultTint, nil
	}
	if !tintPattern.MatchString(text) {
		return "", errors.New("Accent color must look like #5B7CFA.")
	}
	return text, nil
}

