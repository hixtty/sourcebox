package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Error kinds surfaced to the UI.
const (
	KindNotFound  = "not_found"
	KindBlocked   = "blocked"
	KindRateLimit = "rate_limit"
	KindNetwork   = "network"
	KindHTTP      = "http"
	KindPayload   = "payload"
)

// Error is a GitHub lookup failure worth showing to the operator.
type Error struct {
	Kind    string
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// RepoInfo is the repository metadata shown on the dashboard.
type RepoInfo struct {
	Owner       string
	Name        string
	Description string
	AvatarURL   string
	Stars       int
	HTMLURL     string
}

// Asset is one release asset.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// ReleaseInfo is the latest published release.
type ReleaseInfo struct {
	Tag         string
	Name        string
	Body        string
	PublishedAt string
	Assets      []Asset
}

// Client talks to the GitHub REST API.
type Client struct {
	http  *http.Client
	token string
	base  string
}

// NewClient builds a client; an empty token keeps the anonymous rate limit.
func NewClient(token string) *Client {
	return &Client{
		http:  &http.Client{Timeout: 20 * time.Second},
		token: strings.TrimSpace(token),
		base:  "https://api.github.com",
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return &Error{Kind: KindNetwork, Message: "Could not build the GitHub request."}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "sourcebox/1.0")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{Kind: KindNetwork, Message: "Could not reach GitHub: " + networkReason(err)}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return &Error{Kind: KindNotFound, Status: 404, Message: "Repository or release not found."}
	case resp.StatusCode == http.StatusUnavailableForLegalReasons:
		return &Error{Kind: KindBlocked, Status: 451, Message: "GitHub blocked this repository (DMCA or policy takedown)."}
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		if remaining == "" {
			remaining = "unknown"
		}
		return &Error{
			Kind:    KindRateLimit,
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("GitHub API rate limit reached (remaining: %s). Add GITHUB_TOKEN or retry later.", remaining),
		}
	case resp.StatusCode >= 400:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return &Error{
			Kind:    KindHTTP,
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("GitHub returned HTTP %d. %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &Error{Kind: KindPayload, Message: "GitHub returned an unexpected response."}
	}
	return nil
}

// Repo fetches repository metadata.
func (c *Client) Repo(ctx context.Context, owner, name string) (*RepoInfo, error) {
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Stars       int    `json:"stargazers_count"`
		HTMLURL     string `json:"html_url"`
		Owner       struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"owner"`
	}
	if err := c.get(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), &payload); err != nil {
		return nil, err
	}

	login := payload.Owner.Login
	if login == "" {
		login = owner
	}
	repoName := payload.Name
	if repoName == "" {
		repoName = name
	}
	htmlURL := payload.HTMLURL
	if htmlURL == "" {
		htmlURL = "https://github.com/" + owner + "/" + name
	}
	return &RepoInfo{
		Owner:       login,
		Name:        repoName,
		Description: payload.Description,
		AvatarURL:   payload.Owner.AvatarURL,
		Stars:       payload.Stars,
		HTMLURL:     htmlURL,
	}, nil
}

// LatestRelease fetches the latest published release, or nil when there is none.
func (c *Client) LatestRelease(ctx context.Context, owner, name string) (*ReleaseInfo, error) {
	var payload struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}

	err := c.get(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name)+"/releases/latest", &payload)
	if err != nil {
		if ghErr, ok := err.(*Error); ok && ghErr.Kind == KindNotFound {
			return nil, nil
		}
		return nil, err
	}

	tag := payload.TagName
	if tag == "" {
		tag = payload.Name
	}
	assets := make([]Asset, 0, len(payload.Assets))
	for _, asset := range payload.Assets {
		if asset.BrowserDownloadURL == "" {
			continue
		}
		assets = append(assets, Asset{Name: asset.Name, URL: asset.BrowserDownloadURL, Size: asset.Size})
	}
	return &ReleaseInfo{
		Tag:         tag,
		Name:        payload.Name,
		Body:        payload.Body,
		PublishedAt: payload.PublishedAt,
		Assets:      assets,
	}, nil
}

func networkReason(err error) string {
	text := err.Error()
	if len(text) > 160 {
		text = text[:160] + "…"
	}
	return text
}

// ParseRepoURL turns a pasted GitHub URL into owner and repository.
func ParseRepoURL(raw string) (string, string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", "", fmt.Errorf("请输入 GitHub 仓库地址。")
	}
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	parsed, err := url.Parse(text)
	if err != nil {
		return "", "", fmt.Errorf("这看起来不是 GitHub 地址。")
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	if host != "github.com" {
		return "", "", fmt.Errorf("只支持 github.com 的仓库地址。")
	}

	parts := make([]string, 0, 4)
	for _, part := range strings.Split(strings.Trim(parsed.Path, "/"), "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) < 2 {
		return "", "", fmt.Errorf("地址里需要同时包含作者和仓库名。")
	}
	owner := parts[0]
	name := strings.TrimSuffix(parts[1], ".git")
	if owner == "" || name == "" {
		return "", "", fmt.Errorf("地址里需要同时包含作者和仓库名。")
	}
	return owner, name, nil
}

