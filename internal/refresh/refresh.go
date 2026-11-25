package refresh

import (
	"context"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/sourcebox/sourcebox/internal/config"
	"github.com/sourcebox/sourcebox/internal/github"
	"github.com/sourcebox/sourcebox/internal/source"
	"github.com/sourcebox/sourcebox/internal/store"
)

// Status values stored in the cache.
const (
	StatusOK        = "ok"
	StatusNoRelease = "no_release"
	StatusNoIPA     = "no_ipa"
	StatusError     = "error"
	StatusPending   = "pending"
)

// API is the GitHub surface the refresher needs.
type API interface {
	Repo(ctx context.Context, owner, name string) (*github.RepoInfo, error)
	LatestRelease(ctx context.Context, owner, name string) (*github.ReleaseInfo, error)
}

// Factory builds a fresh API client per worker.
type Factory func() API

// NewFactory wires the real GitHub client.
func NewFactory(cfg config.Config) Factory {
	return func() API { return github.NewClient(cfg.GitHubToken) }
}

var refreshMu sync.Mutex

// BuildCache fetches one repository and produces its cache document.
func BuildCache(ctx context.Context, baseURL string, repo store.Repo, api API) store.Cache {
	cache := store.Cache{
		Status:    StatusError,
		FetchedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}

	info, err := api.Repo(ctx, repo.Owner, repo.Name)
	if err != nil {
		return failed(cache, err)
	}
	cache.Repo = store.RepoMeta{
		Description: info.Description,
		AvatarURL:   info.AvatarURL,
		Stars:       info.Stars,
		HTMLURL:     info.HTMLURL,
	}

	release, err := api.LatestRelease(ctx, repo.Owner, repo.Name)
	if err != nil {
		return failed(cache, err)
	}
	if release == nil {
		cache.Status = StatusNoRelease
		cache.Error = "还没有发布过 Release。"
		return cache
	}

	cache.Release = &store.Release{
		Tag:         release.Tag,
		Name:        release.Name,
		PublishedAt: release.PublishedAt,
		Body:        release.Body,
	}

	assets := source.PickIPAs(release.Assets)
	if len(assets) == 0 {
		label := release.Tag
		if label == "" {
			label = release.Name
		}
		if label == "" {
			label = "latest"
		}
		cache.Status = StatusNoIPA
	cache.Error = "最新 Release " + label + " 里没有 .ipa 文件。"
		return cache
	}

	cache.Apps = source.AppEntries(repo, release, assets, baseURL)
	cache.Status = StatusOK
	cache.Error = ""
	cache.Kind = ""
	return cache
}

func failed(cache store.Cache, err error) store.Cache {
	cache.Status = StatusError
	var ghErr *github.Error
	if errors.As(err, &ghErr) {
		cache.Kind = ghErr.Kind
		cache.Error = ghErr.Message
		return cache
	}
	cache.Kind = "internal"
	log.Printf("refresh: unexpected error: %v", err)
	cache.Error = "读取这个仓库时出现意外错误。"
	return cache
}

// RefreshRepo refetches one repository and saves the result.
func RefreshRepo(ctx context.Context, cfg config.Config, db *store.Store, repo store.Repo, factory Factory) (store.Repo, error) {
	api := factory()
	cache := BuildCache(ctx, cfg.BaseURL, repo, api)
	if err := db.SaveCache(repo.ID, cache); err != nil {
		return store.Repo{}, err
	}
	return db.GetRepo(repo.ID)
}

// EnsureFresh refreshes repositories whose cache is older than ttl.
func EnsureFresh(ctx context.Context, cfg config.Config, db *store.Store, ttl time.Duration, force bool, factory Factory) int {
	repos, err := db.ListRepos()
	if err != nil || len(repos) == 0 {
		return 0
	}

	refreshMu.Lock()
	defer refreshMu.Unlock()

	targets := make([]store.Repo, 0, len(repos))
	for _, repo := range repos {
		if force || isStale(repo.Cache.FetchedAt, ttl) {
			targets = append(targets, repo)
		}
	}
	if len(targets) == 0 {
		return 0
	}

	workers := min(6, len(targets))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, repo := range targets {
		wg.Add(1)
		go func(item store.Repo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, _ = RefreshRepo(ctx, cfg, db, item, factory)
		}(repo)
	}
	wg.Wait()
	return len(targets)
}

func isStale(fetchedAt string, ttl time.Duration) bool {
	if fetchedAt == "" {
		return true
	}
	stamp, err := time.Parse(time.RFC3339, fetchedAt)
	if err != nil {
		return true
	}
	return time.Since(stamp) > ttl
}

// TTL resolves the refresh window, letting the settings page override the env.
func TTL(cfg config.Config, kv map[string]string) time.Duration {
	seconds := cfg.CacheSeconds
	if raw, ok := kv["cache_seconds"]; ok && raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			seconds = parsed
		}
	}
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds) * time.Second
}

