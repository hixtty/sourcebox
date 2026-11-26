package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sourcebox/sourcebox/internal/config"
	"github.com/sourcebox/sourcebox/internal/github"
	"github.com/sourcebox/sourcebox/internal/refresh"
	"github.com/sourcebox/sourcebox/internal/store"
)

type fakeAPI struct {
	release *github.ReleaseInfo
}

func (f *fakeAPI) Repo(_ context.Context, owner, name string) (*github.RepoInfo, error) {
	return &github.RepoInfo{
		Owner:       owner,
		Name:        name,
		Description: "A demo application.",
		Stars:       42,
		HTMLURL:     "https://github.com/" + owner + "/" + name,
	}, nil
}

func (f *fakeAPI) LatestRelease(_ context.Context, _, _ string) (*github.ReleaseInfo, error) {
	return f.release, nil
}

func newTestServer(t *testing.T, adminToken string) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := config.Config{
		DataDir:      dir,
		BaseURL:      "http://testserver",
		AdminPath:    "control",
		AdminToken:   adminToken,
		CacheSeconds: 600,
	}
	release := &github.ReleaseInfo{
		Tag:         "v2.1.0",
		Name:        "2.1.0",
		Body:        "Release notes.",
		PublishedAt: "2026-02-03T04:05:06Z",
		Assets: []github.Asset{
			{Name: "Demo.ipa", URL: "https://example.com/Demo.ipa", Size: 2048},
			{Name: "Demo.dSYM.zip", URL: "https://example.com/sym.zip", Size: 10},
		},
	}
	factory := func() refresh.API { return &fakeAPI{release: release} }
	return New(cfg, db, factory), db
}

func do(t *testing.T, server *Server, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	if strings.HasPrefix(path, "/api/") {
		path = "/" + server.adminPath + path
	}
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	return recorder
}

func TestAddRepositoryBuildsSource(t *testing.T) {
	server, _ := newTestServer(t, "")

	response := do(t, server, http.MethodPost, "/api/repos",
		map[string]string{"url": "https://github.com/octo/demo", "tintColor": "#112233"}, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}

	var added repoView
	if err := json.Unmarshal(response.Body.Bytes(), &added); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if added.Status != "ok" || added.LatestVersion == nil || *added.LatestVersion != "2.1.0" {
		t.Fatalf("unexpected view: %+v", added)
	}
	if added.TintColor != "#112233" {
		t.Fatalf("tint = %q", added.TintColor)
	}

	sourceResponse := do(t, server, http.MethodGet, "/control/source.json", nil, nil)
	if sourceResponse.Code != http.StatusOK {
		t.Fatalf("source status = %d", sourceResponse.Code)
	}
	var document struct {
		Apps []store.App `json:"apps"`
	}
	if err := json.Unmarshal(sourceResponse.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode source: %v", err)
	}
	if len(document.Apps) != 1 {
		t.Fatalf("apps = %d", len(document.Apps))
	}
	if document.Apps[0].Versions[0].DownloadURL != "https://example.com/Demo.ipa" {
		t.Fatalf("downloadURL = %q", document.Apps[0].Versions[0].DownloadURL)
	}
}

func TestDuplicateRepositoryRejected(t *testing.T) {
	server, _ := newTestServer(t, "")
	body := map[string]string{"url": "https://github.com/octo/demo"}
	if code := do(t, server, http.MethodPost, "/api/repos", body, nil).Code; code != http.StatusCreated {
		t.Fatalf("first add status = %d", code)
	}
	if code := do(t, server, http.MethodPost, "/api/repos", body, nil).Code; code != http.StatusConflict {
		t.Fatalf("duplicate status = %d", code)
	}
}

func TestRejectsNonGitHubURL(t *testing.T) {
	server, _ := newTestServer(t, "")
	code := do(t, server, http.MethodPost, "/api/repos",
		map[string]string{"url": "https://gitlab.com/octo/demo"}, nil).Code
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d", code)
	}
}

func TestReleaseWithoutIPAShowsErrorEntry(t *testing.T) {
	server, _ := newTestServer(t, "")
	server.factory = func() refresh.API {
		return &fakeAPI{release: &github.ReleaseInfo{
			Tag:         "v1.0.0",
			PublishedAt: "2026-02-03T04:05:06Z",
			Assets:      []github.Asset{{Name: "Demo.zip", URL: "https://example.com/d.zip", Size: 10}},
		}}
	}

	if code := do(t, server, http.MethodPost, "/api/repos",
		map[string]string{"url": "https://github.com/octo/demo"}, nil).Code; code != http.StatusCreated {
		t.Fatalf("status = %d", code)
	}

	var document struct {
		Apps   []store.App `json:"apps"`
		Errors []struct {
			Name string `json:"name"`
		} `json:"errors"`
	}
	response := do(t, server, http.MethodGet, "/control/source.json", nil, nil)
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(document.Apps) != 0 {
		t.Fatalf("apps = %d", len(document.Apps))
	}
	if len(document.Errors) != 1 || document.Errors[0].Name != "demo" {
		t.Fatalf("errors = %+v", document.Errors)
	}
}

func TestAdminTokenGuardsDashboard(t *testing.T) {
	server, _ := newTestServer(t, "s3cret")

	if code := do(t, server, http.MethodGet, "/api/overview", nil, nil).Code; code != http.StatusUnauthorized {
		t.Fatalf("unauthed status = %d", code)
	}
	bad := do(t, server, http.MethodPost, "/api/login", map[string]string{"token": "wrong"}, nil)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", bad.Code)
	}

	good := do(t, server, http.MethodPost, "/api/login", map[string]string{"token": "s3cret"}, nil)
	if good.Code != http.StatusOK {
		t.Fatalf("login status = %d", good.Code)
	}
	cookies := good.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set a session cookie")
	}
	if code := do(t, server, http.MethodGet, "/api/overview", nil, cookies[0]).Code; code != http.StatusOK {
		t.Fatalf("authed status = %d", code)
	}
}

func TestSourceSettingsRoundTrip(t *testing.T) {
	server, _ := newTestServer(t, "")
	payload := map[string]any{
		"name":         "My Shelf",
		"slug":         "shelf",
		"subtitle":     "Private builds",
		"description":  "Only what I need.",
		"tintColor":    "#AABBCC",
		"cacheSeconds": 120,
	}
	if code := do(t, server, http.MethodPut, "/api/source", payload, nil).Code; code != http.StatusOK {
		t.Fatalf("put status = %d", code)
	}

	var view sourceView
	response := do(t, server, http.MethodGet, "/api/source", nil, nil)
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Slug != "shelf" || view.TintColor != "#AABBCC" {
		t.Fatalf("view = %+v", view)
	}
	if view.URL != "http://testserver/control/shelf.json" {
		t.Fatalf("url = %q", view.URL)
	}
	if code := do(t, server, http.MethodGet, "/control/shelf.json", nil, nil).Code; code != http.StatusOK {
		t.Fatalf("named source status = %d", code)
	}
	if code := do(t, server, http.MethodGet, "/control/missing.json", nil, nil).Code; code != http.StatusNotFound {
		t.Fatalf("missing source status = %d", code)
	}
}

func TestHealth(t *testing.T) {
	server, _ := newTestServer(t, "")
	response := do(t, server, http.MethodGet, "/health", nil, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body struct {
		OK           bool `json:"ok"`
		Repositories int  `json:"repositories"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.OK || body.Repositories != 0 {
		t.Fatalf("body = %+v", body)
	}
}

func TestDashboardIsHiddenBehindTheSecretPath(t *testing.T) {
	server, _ := newTestServer(t, "")

	raw := func(path string) int {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		return recorder.Code
	}

	if code := raw("/"); code != http.StatusNotFound {
		t.Fatalf("root status = %d, want 404", code)
	}
	if code := raw("/api/overview"); code != http.StatusNotFound {
		t.Fatalf("unprefixed api status = %d, want 404", code)
	}
	if code := raw("/source.json"); code != http.StatusNotFound {
		t.Fatalf("unprefixed source status = %d, want 404", code)
	}
	if code := raw("/icons/demo.png"); code != http.StatusNotFound {
		t.Fatalf("unprefixed icon status = %d, want 404", code)
	}
	if code := raw("/control/api/overview"); code != http.StatusOK {
		t.Fatalf("prefixed api status = %d, want 200", code)
	}
	if code := raw("/control/source.json"); code != http.StatusOK {
		t.Fatalf("prefixed source status = %d, want 200", code)
	}

	request := httptest.NewRequest(http.MethodGet, "/control/runtime-config.js", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("runtime config status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `__SOURCEBOX_BASE__="/control"`) {
		t.Fatalf("runtime config body = %q", recorder.Body.String())
	}
}

func TestGeneratedAdminPathIsStable(t *testing.T) {
	server, db := newTestServer(t, "")
	server.adminPath = ""
	server.cfg.AdminPath = ""
	server.resolveAdminPath()

	first := server.adminPath
	if len(first) < 10 || !adminPathPattern.MatchString(first) {
		t.Fatalf("generated path %q does not look random", first)
	}
	if reservedPaths[first] {
		t.Fatalf("generated path %q is reserved", first)
	}

	other := New(server.cfg, db, server.factory)
	if other.adminPath != first {
		t.Fatalf("path not reused across restarts: %q vs %q", other.adminPath, first)
	}
}

