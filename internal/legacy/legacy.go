package legacy

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sourcebox/sourcebox/internal/config"
	"github.com/sourcebox/sourcebox/internal/github"
	"github.com/sourcebox/sourcebox/internal/source"
	"github.com/sourcebox/sourcebox/internal/store"
)

type legacyConfig struct {
	Source struct {
		Name         string `yaml:"name"`
		Slug         string `yaml:"slug"`
		Subtitle     string `yaml:"subtitle"`
		Description  string `yaml:"description"`
		TintColor    string `yaml:"tint_color"`
		CacheSeconds *int   `yaml:"cache_seconds"`
	} `yaml:"source"`
	Repositories []struct {
		URL       string `yaml:"url"`
		TintColor string `yaml:"tint_color"`
		Icon      string `yaml:"icon"`
	} `yaml:"repositories"`
}

// ImportConfig seeds the database from a GithubStore config.yml on first boot.
func ImportConfig(db *store.Store, cfg config.Config) int {
	repos, err := db.ListRepos()
	if err != nil || len(repos) > 0 {
		return 0
	}

	path := locate(cfg)
	if path == "" {
		return 0
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var parsed legacyConfig
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		log.Printf("legacy: could not parse %s: %v", path, err)
		return 0
	}

	kv, _ := db.KVAll()
	setIfMissing := func(key, value string) {
		if value == "" || kv[key] != "" {
			return
		}
		_ = db.KVSet(key, value)
		kv[key] = value
	}
	setIfMissing("source_name", strings.TrimSpace(parsed.Source.Name))
	setIfMissing("source_slug", store.Slugify(strings.TrimSpace(parsed.Source.Slug)))
	setIfMissing("source_subtitle", strings.TrimSpace(parsed.Source.Subtitle))
	setIfMissing("source_description", strings.TrimSpace(parsed.Source.Description))
	if strings.HasPrefix(strings.TrimSpace(parsed.Source.TintColor), "#") {
		setIfMissing("source_tint", strings.TrimSpace(parsed.Source.TintColor))
	}
	if parsed.Source.CacheSeconds != nil && kv["cache_seconds"] == "" {
		_ = db.KVSet("cache_seconds", strconv.Itoa(*parsed.Source.CacheSeconds))
	}

	taken, _ := db.TakenSlugs()
	imported := 0
	for _, entry := range parsed.Repositories {
		owner, name, err := github.ParseRepoURL(entry.URL)
		if err != nil {
			continue
		}
		canonical := "https://github.com/" + owner + "/" + name
		if _, err := db.GetRepoByURL(canonical); err == nil {
			continue
		}
		slug := source.UniqueSlug(name, taken)
		taken[slug] = true
		tint := entry.TintColor
		if !strings.HasPrefix(tint, "#") {
			tint = config.DefaultTint
		}
		if _, err := db.CreateRepo(store.Repo{
			URL:         canonical,
			Owner:       owner,
			Name:        name,
			Slug:        slug,
			DisplayName: name,
			TintColor:   tint,
			IconURL:     entry.Icon,
		}); err != nil {
			continue
		}
		imported++
	}
	if imported > 0 {
		log.Printf("legacy: imported %d repositories from %s", imported, path)
	}
	return imported
}

func locate(cfg config.Config) string {
	candidates := []string{cfg.SeedConfig, filepath.Join(cfg.DataDir, "config.yml"), "/app/config.yml"}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

