package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultTint = "#5B7CFA"

// Config holds everything the server reads from the environment at boot.
type Config struct {
	DataDir      string
	BaseURL      string
	AdminPath    string
	AdminToken   string
	GitHubToken  string
	CacheSeconds int
	Host         string
	Port         int
	SeedConfig   string
	WebDir       string
}

// Load reads the environment, falling back to local-friendly defaults.
func Load() Config {
	port := envInt("PORT", 8080)
	baseURL := strings.TrimRight(envString("BASE_URL", "http://localhost:"+strconv.Itoa(port)), "/")

	dataDir, err := filepath.Abs(envString("DATA_DIR", "./data"))
	if err != nil {
		dataDir = envString("DATA_DIR", "./data")
	}
	webDir, err := filepath.Abs(envString("WEB_DIR", "./web/dist"))
	if err != nil {
		webDir = envString("WEB_DIR", "./web/dist")
	}

	return Config{
		DataDir:      dataDir,
		BaseURL:      baseURL,
		AdminPath:    strings.Trim(strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_PATH"))), "/"),
		AdminToken:   strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		GitHubToken:  strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		CacheSeconds: max(envInt("CACHE_SECONDS", 600), 0),
		Host:         envString("HOST", "0.0.0.0"),
		Port:         port,
		SeedConfig:   strings.TrimSpace(os.Getenv("SEED_CONFIG")),
		WebDir:       webDir,
	}
}

func (c Config) DBPath() string { return filepath.Join(c.DataDir, "sourcebox.db") }

func (c Config) IconsDir() string { return filepath.Join(c.DataDir, "icons") }

func (c Config) Addr() string {
	return c.Host + ":" + strconv.Itoa(c.Port)
}

func (c Config) SecureCookies() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

