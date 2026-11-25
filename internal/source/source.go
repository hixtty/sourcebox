package source

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sourcebox/sourcebox/internal/github"
	"github.com/sourcebox/sourcebox/internal/store"
)

var debugHints = []string{"debug", "symbols", "dsym", "-tests", "testflight"}

// Document is the AltStore source JSON.
type Document struct {
	Name        string        `json:"name"`
	Subtitle    string        `json:"subtitle"`
	Description string        `json:"description"`
	IconURL     string        `json:"iconURL"`
	TintColor   string        `json:"tintColor"`
	Apps        []store.App   `json:"apps"`
	Errors      []SourceError `json:"errors,omitempty"`
}

// SourceError is one repository that could not contribute apps.
type SourceError struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Error string `json:"error"`
}

// NormalizeVersion strips a leading v from a release tag.
func NormalizeVersion(tag string) string {
	version := strings.TrimSpace(tag)
	if len(version) > 1 && (version[0] == 'v' || version[0] == 'V') && version[1] >= '0' && version[1] <= '9' {
		version = version[1:]
	}
	version = strings.ReplaceAll(version, "/", "-")
	version = strings.TrimSpace(version)
	if version == "" {
		return "0.0.0"
	}
	return version
}

// PickIPAs keeps .ipa assets, release builds before debug builds, larger first.
func PickIPAs(assets []github.Asset) []github.Asset {
	picked := make([]github.Asset, 0, len(assets))
	for _, asset := range assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), ".ipa") {
			picked = append(picked, asset)
		}
	}

	sort.SliceStable(picked, func(i, j int) bool {
		leftDebug := isDebugAsset(picked[i].Name)
		rightDebug := isDebugAsset(picked[j].Name)
		if leftDebug != rightDebug {
			return !leftDebug
		}
		if picked[i].Size != picked[j].Size {
			return picked[i].Size > picked[j].Size
		}
		return picked[i].Name < picked[j].Name
	})
	return picked
}

func isDebugAsset(name string) bool {
	lower := strings.ToLower(name)
	for _, hint := range debugHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// UniqueSlug appends -2, -3, … until the slug is free.
func UniqueSlug(base string, taken map[string]bool) string {
	slug := store.Slugify(base)
	if !taken[slug] {
		return slug
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s-%d", slug, index)
		if !taken[candidate] {
			return candidate
		}
	}
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// BundleIdentifier builds a stable reverse-DNS identifier for a repository.
func BundleIdentifier(owner, repo string) string {
	clean := func(value string) string {
		out := nonAlphaNum.ReplaceAllString(strings.ToLower(value), "")
		if out == "" {
			return "app"
		}
		return out
	}
	return "github." + clean(owner) + "." + clean(repo)
}

// AppEntries builds one app entry per .ipa asset on the latest release.
func AppEntries(repo store.Repo, release *github.ReleaseInfo, assets []github.Asset, baseURL string) []store.App {
	entries := make([]store.App, 0, len(assets))
	multiple := len(assets) > 1
	version := NormalizeVersion(release.Tag)

	description := trimText(release.Body, 8000)
	if description == "" {
		tag := release.Tag
		if tag == "" {
			tag = version
		}
		description = fmt.Sprintf("%s — latest release %s.", repo.DisplayName, tag)
	}

	separators := regexp.MustCompile(`[._-]+`)
	for _, asset := range assets {
		name := repo.DisplayName
		bundle := BundleIdentifier(repo.Owner, repo.Name)
		if multiple {
			stem := asset.Name
			if strings.HasSuffix(strings.ToLower(stem), ".ipa") {
				stem = stem[:len(stem)-4]
			}
			label := strings.TrimSpace(separators.ReplaceAllString(stem, " "))
			if label == "" {
				label = version
			}
			name = repo.DisplayName + " · " + label
			bundle = BundleIdentifier(repo.Owner, repo.Name) + "." + store.Slugify(stem)
		}

		entries = append(entries, store.App{
			Name:                 name,
			BundleIdentifier:     bundle,
			DeveloperName:        repo.Owner,
			Subtitle:             repo.Owner + "/" + repo.Name,
			LocalizedDescription: description,
			IconURL:              baseURL + "/icons/" + repo.Slug + ".png",
			TintColor:            repo.TintColor,
			Versions: []store.Version{{
				Version:              version,
				BuildVersion:         version,
				Date:                 release.PublishedAt,
				DownloadURL:          asset.URL,
				LocalizedDescription: description,
				Size:                 asset.Size,
			}},
		})
	}
	return entries
}

func trimText(text string, limit int) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= limit {
		return trimmed
	}
	return strings.TrimRight(trimmed[:limit-1], " \n\t") + "…"
}

