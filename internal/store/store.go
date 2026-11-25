package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS kv (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS repos (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	url          TEXT NOT NULL UNIQUE,
	owner        TEXT NOT NULL,
	name         TEXT NOT NULL,
	slug         TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	tint_color   TEXT NOT NULL,
	icon_url     TEXT,
	sort_order   INTEGER NOT NULL DEFAULT 0,
	created_at   TEXT NOT NULL,
	cache_json   TEXT NOT NULL DEFAULT '{}'
);
`

// Version mirrors one AltStore version entry.
type Version struct {
	Version              string `json:"version"`
	BuildVersion         string `json:"buildVersion"`
	Date                 string `json:"date"`
	DownloadURL          string `json:"downloadURL"`
	LocalizedDescription string `json:"localizedDescription"`
	Size                 int64  `json:"size"`
}

// App mirrors one AltStore app entry.
type App struct {
	Name                 string    `json:"name"`
	BundleIdentifier     string    `json:"bundleIdentifier"`
	DeveloperName        string    `json:"developerName"`
	Subtitle             string    `json:"subtitle"`
	LocalizedDescription string    `json:"localizedDescription"`
	IconURL              string    `json:"iconURL"`
	TintColor            string    `json:"tintColor"`
	Versions             []Version `json:"versions"`
}

// RepoMeta is the subset of GitHub repository data kept for the dashboard.
type RepoMeta struct {
	Description string `json:"description"`
	AvatarURL   string `json:"avatarUrl"`
	Stars       int    `json:"stars"`
	HTMLURL     string `json:"htmlUrl"`
}

// Release is the cached latest release.
type Release struct {
	Tag         string `json:"tag"`
	Name        string `json:"name"`
	PublishedAt string `json:"publishedAt"`
	Body        string `json:"body"`
}

// Cache is persisted as JSON so a refresh never blocks a read.
type Cache struct {
	Status    string   `json:"status"`
	Error     string   `json:"error,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	FetchedAt string   `json:"fetchedAt,omitempty"`
	Repo      RepoMeta `json:"repo"`
	Release   *Release `json:"release,omitempty"`
	Apps      []App    `json:"apps,omitempty"`
}

// Repo is one configured GitHub repository.
type Repo struct {
	ID          int64
	URL         string
	Owner       string
	Name        string
	Slug        string
	DisplayName string
	TintColor   string
	IconURL     string
	SortOrder   int
	CreatedAt   string
	Cache       Cache
}

// Store is the SQLite-backed persistence layer.
type Store struct {
	db   *sql.DB
	path string
}

// Open creates the database (and schema) at path.
func Open(path string) (*Store, error) {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single connection keeps writes serialized without SQLITE_BUSY retries.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// ---------------------------------------------------------------- kv

// KVAll returns every key/value pair.
func (s *Store) KVAll() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM kv")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

// KVSet upserts one key.
func (s *Store) KVSet(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO kv(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// ----------------------------------------------------------------- repos

const repoColumns = "id, url, owner, name, slug, display_name, tint_color, COALESCE(icon_url, ''), sort_order, created_at, cache_json"

// ListRepos returns repositories in display order.
func (s *Store) ListRepos() ([]Repo, error) {
	rows, err := s.db.Query("SELECT " + repoColumns + " FROM repos ORDER BY sort_order ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []Repo
	for rows.Next() {
		repo, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, rows.Err()
}

// GetRepo loads one repository by id.
func (s *Store) GetRepo(id int64) (Repo, error) {
	row := s.db.QueryRow("SELECT "+repoColumns+" FROM repos WHERE id = ?", id)
	return scanRepo(row)
}

// GetRepoBySlug loads one repository by slug.
func (s *Store) GetRepoBySlug(slug string) (Repo, error) {
	row := s.db.QueryRow("SELECT "+repoColumns+" FROM repos WHERE slug = ?", slug)
	return scanRepo(row)
}

// GetRepoByURL loads one repository by canonical URL.
func (s *Store) GetRepoByURL(url string) (Repo, error) {
	row := s.db.QueryRow("SELECT "+repoColumns+" FROM repos WHERE url = ?", url)
	return scanRepo(row)
}

// TakenSlugs returns every slug currently in use.
func (s *Store) TakenSlugs() (map[string]bool, error) {
	rows, err := s.db.Query("SELECT slug FROM repos")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	taken := map[string]bool{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		taken[slug] = true
	}
	return taken, rows.Err()
}

// CreateRepo inserts a repository and returns the stored row.
func (s *Store) CreateRepo(repo Repo) (Repo, error) {
	var next int
	if err := s.db.QueryRow("SELECT COALESCE(MAX(sort_order), -1) + 1 FROM repos").Scan(&next); err != nil {
		return Repo{}, err
	}

	result, err := s.db.Exec(
		`INSERT INTO repos(url, owner, name, slug, display_name, tint_color, icon_url, sort_order, created_at, cache_json)
		 VALUES(?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, '{}')`,
		repo.URL, repo.Owner, repo.Name, repo.Slug, repo.DisplayName, repo.TintColor,
		repo.IconURL, next, nowISO(),
	)
	if err != nil {
		return Repo{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Repo{}, err
	}
	return s.GetRepo(id)
}

// UpdateRepo applies the given fields (display_name, tint_color, icon_url, cache_json).
func (s *Store) UpdateRepo(id int64, fields map[string]any) error {
	allowed := map[string]bool{
		"display_name": true,
		"tint_color":   true,
		"icon_url":     true,
		"cache_json":   true,
	}

	sets := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	for key, value := range fields {
		if !allowed[key] {
			continue
		}
		sets = append(sets, key+" = ?")
		args = append(args, value)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	_, err := s.db.Exec("UPDATE repos SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

// SaveCache persists a refreshed cache document.
func (s *Store) SaveCache(id int64, cache Cache) error {
	encoded, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return s.UpdateRepo(id, map[string]any{"cache_json": string(encoded)})
}

// DeleteRepo removes a repository.
func (s *Store) DeleteRepo(id int64) error {
	_, err := s.db.Exec("DELETE FROM repos WHERE id = ?", id)
	return err
}

// Reorder writes a new display order.
func (s *Store) Reorder(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	for index, id := range ids {
		if _, err := tx.Exec("UPDATE repos SET sort_order = ? WHERE id = ?", index, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --------------------------------------------------------------- helpers

type scanner interface {
	Scan(dest ...any) error
}

func scanRepo(row scanner) (Repo, error) {
	var repo Repo
	var cacheJSON string
	err := row.Scan(
		&repo.ID, &repo.URL, &repo.Owner, &repo.Name, &repo.Slug, &repo.DisplayName,
		&repo.TintColor, &repo.IconURL, &repo.SortOrder, &repo.CreatedAt, &cacheJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Repo{}, ErrNotFound
	}
	if err != nil {
		return Repo{}, err
	}
	if cacheJSON != "" {
		if err := json.Unmarshal([]byte(cacheJSON), &repo.Cache); err != nil {
			repo.Cache = Cache{}
		}
	}
	return repo, nil
}

// ErrNotFound is returned when a repository does not exist.
var ErrNotFound = errors.New("repository not found")

// Slugify produces a URL-safe slug.
func Slugify(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "app"
	}
	return slug
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

