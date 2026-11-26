package httpapi

import (
	"strconv"

	"github.com/sourcebox/sourcebox/internal/config"
	"github.com/sourcebox/sourcebox/internal/refresh"
	"github.com/sourcebox/sourcebox/internal/source"
	"github.com/sourcebox/sourcebox/internal/store"
)

var statusLabels = map[string]string{
	refresh.StatusOK:        "就绪",
	refresh.StatusNoRelease: "无 Release",
	refresh.StatusNoIPA:     "无 IPA",
	refresh.StatusError:     "错误",
	refresh.StatusPending:   "等待中",
}

var problemStatuses = map[string]bool{
	refresh.StatusError:     true,
	refresh.StatusNoIPA:     true,
	refresh.StatusNoRelease: true,
}

type sourceView struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Subtitle     string `json:"subtitle"`
	Description  string `json:"description"`
	TintColor    string `json:"tintColor"`
	CacheSeconds int    `json:"cacheSeconds"`
	URL          string `json:"url"`
}

type repoView struct {
	ID            int64   `json:"id"`
	URL           string  `json:"url"`
	Owner         string  `json:"owner"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	DisplayName   string  `json:"displayName"`
	TintColor     string  `json:"tintColor"`
	IconURL       *string `json:"iconUrl"`
	IconSrc       string  `json:"iconSrc"`
	Status        string  `json:"status"`
	StatusLabel   string  `json:"statusLabel"`
	Error         *string `json:"error"`
	ReleaseTag    *string `json:"releaseTag"`
	ReleaseDate   *string `json:"releaseDate"`
	RefreshedAt   *string `json:"refreshedAt"`
	AppCount      int     `json:"appCount"`
	LatestVersion *string `json:"latestVersion"`
	LatestSize    *int64  `json:"latestSize"`
	Description   string  `json:"description"`
	Stars         int     `json:"stars"`
}

type statsView struct {
	Repositories int     `json:"repositories"`
	Apps         int     `json:"apps"`
	Errors       int     `json:"errors"`
	Pending      int     `json:"pending"`
	LastRefresh  *string `json:"lastRefresh"`
}

type overviewView struct {
	BaseURL               string     `json:"baseUrl"`
	AdminPath             string     `json:"adminPath"`
	Source                sourceView `json:"source"`
	Stats                 statsView  `json:"stats"`
	Repos                 []repoView `json:"repos"`
	AuthRequired          bool       `json:"authRequired"`
	GitHubTokenConfigured bool       `json:"githubTokenConfigured"`
}

type detailView struct {
	Repo    repoView       `json:"repo"`
	Apps    []store.App    `json:"apps"`
	Release *store.Release `json:"release"`
	Error   *string        `json:"error"`
	Status  string         `json:"status"`
}

func ptr[T any](value T) *T { return &value }

func nonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// sourceFromDB merges defaults, environment and stored overrides. publicPath is
// the random segment in front of both the dashboard and the source document.
func sourceFromDB(cfg config.Config, publicPath string, kv map[string]string) sourceView {
	value := func(key, fallback string) string {
		if stored := kv[key]; stored != "" {
			return stored
		}
		return fallback
	}

	cacheSeconds := cfg.CacheSeconds
	if raw := kv["cache_seconds"]; raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			cacheSeconds = parsed
		}
	}
	if cacheSeconds < 0 {
		cacheSeconds = 0
	}

	slug := value("source_slug", "source")
	return sourceView{
		Name:         value("source_name", "SourceBox"),
		Slug:         slug,
		Subtitle:     value("source_subtitle", "自托管的 AltStore 源"),
		Description:  value("source_description", "由 GitHub Release 的 .ipa 文件生成的 AltStore / SideStore 源。"),
		TintColor:    value("source_tint", config.DefaultTint),
		CacheSeconds: cacheSeconds,
		URL:          cfg.BaseURL + "/" + publicPath + "/" + slug + ".json",
	}
}

func repoToView(cfg config.Config, publicPath string, repo store.Repo) repoView {
	cache := repo.Cache
	status := cache.Status
	if status == "" {
		status = refresh.StatusPending
	}

	view := repoView{
		ID:          repo.ID,
		URL:         repo.URL,
		Owner:       repo.Owner,
		Name:        repo.Name,
		Slug:        repo.Slug,
		DisplayName: repo.DisplayName,
		TintColor:   repo.TintColor,
		IconURL:     nonEmpty(repo.IconURL),
		IconSrc:     cfg.BaseURL + "/" + publicPath + "/icons/" + repo.Slug + ".png",
		Status:      status,
		StatusLabel: statusLabels[status],
		Error:       nonEmpty(cache.Error),
		RefreshedAt: nonEmpty(cache.FetchedAt),
		AppCount:    len(cache.Apps),
		Description: cache.Repo.Description,
		Stars:       cache.Repo.Stars,
	}
	if view.StatusLabel == "" {
		view.StatusLabel = status
	}
	if cache.Release != nil {
		view.ReleaseTag = nonEmpty(cache.Release.Tag)
		view.ReleaseDate = nonEmpty(cache.Release.PublishedAt)
	}
	if len(cache.Apps) > 0 && len(cache.Apps[0].Versions) > 0 {
		latest := cache.Apps[0].Versions[0]
		view.LatestVersion = nonEmpty(latest.Version)
		view.LatestSize = ptr(latest.Size)
	}
	return view
}

func statsFromRepos(repos []store.Repo) statsView {
	stats := statsView{Repositories: len(repos)}
	var last string
	for _, repo := range repos {
		cache := repo.Cache
		stats.Apps += len(cache.Apps)
		if problemStatuses[cache.Status] {
			stats.Errors++
		}
		if cache.Status == "" || cache.Status == refresh.StatusPending {
			stats.Pending++
		}
		if cache.FetchedAt > last {
			last = cache.FetchedAt
		}
	}
	stats.LastRefresh = nonEmpty(last)
	return stats
}

// buildDocument assembles the public AltStore source JSON.
func buildDocument(cfg config.Config, publicPath string, kv map[string]string, repos []store.Repo) source.Document {
	configView := sourceFromDB(cfg, publicPath, kv)
	assetBase := cfg.BaseURL + "/" + publicPath
	document := source.Document{
		Name:        configView.Name,
		Subtitle:    configView.Subtitle,
		Description: configView.Description,
		IconURL:     assetBase + "/source-icon.png",
		TintColor:   configView.TintColor,
		Apps:        []store.App{},
	}
	for _, repo := range repos {
		switch repo.Cache.Status {
		case refresh.StatusOK:
			for _, app := range repo.Cache.Apps {
				// Cached entries may predate the random prefix, so rebuild the
				// icon URL from the current one on every render.
				app.IconURL = assetBase + "/icons/" + repo.Slug + ".png"
				document.Apps = append(document.Apps, app)
			}
		case refresh.StatusError, refresh.StatusNoIPA, refresh.StatusNoRelease:
			message := repo.Cache.Error
			if message == "" {
				message = "This repository could not be read."
			}
			document.Errors = append(document.Errors, source.SourceError{
				Name:  repo.DisplayName,
				URL:   repo.URL,
				Error: message,
			})
		}
	}
	return document
}

