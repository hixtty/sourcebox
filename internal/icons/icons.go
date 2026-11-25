package icons

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/sourcebox/sourcebox/internal/config"
	"github.com/sourcebox/sourcebox/internal/store"
)

// IconTTL is how long a proxied remote icon is reused.
const IconTTL = 24 * time.Hour

var hexPattern = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

// HexRGB converts "#RRGGBB" into components, falling back to the default accent.
func HexRGB(value string) (int, int, int) {
	text := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(text) == 3 {
		text = string([]byte{text[0], text[0], text[1], text[1], text[2], text[2]})
	}
	if !hexPattern.MatchString(text) {
		text = strings.TrimPrefix(config.DefaultTint, "#")
	}
	red, _ := strconv.ParseInt(text[0:2], 16, 32)
	green, _ := strconv.ParseInt(text[2:4], 16, 32)
	blue, _ := strconv.ParseInt(text[4:6], 16, 32)
	return int(red), int(green), int(blue)
}

// MediaType sniffs the served content type of an icon payload.
func MediaType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return "image/jpeg"
	case len(data) > 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp"
	case bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n"), []byte("<svg")):
		return "image/svg+xml"
	}
	return "application/octet-stream"
}

// GeneratedIcon builds a rounded placeholder icon with the first letter.
func GeneratedIcon(label, tint string, size int) ([]byte, error) {
	if size < 32 {
		size = 32
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	red, green, blue := HexRGB(tint)
	mask := roundedRectMask(size, int(float64(size)*0.23))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			alpha := mask.AlphaAt(x, y).A
			img.SetRGBA(x, y, color.RGBA{R: uint8(red), G: uint8(green), B: uint8(blue), A: alpha})
		}
	}

	if letter := firstLetter(label); letter != 0 {
		drawLetter(img, letter, size)
	}

	buffer := &bytes.Buffer{}
	if err := png.Encode(buffer, img); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func firstLetter(label string) rune {
	for _, char := range strings.ToUpper(strings.TrimSpace(label)) {
		return char
	}
	return 0
}

func roundedRectMask(size, radius int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, size, size))
	const samples = 4
	const total = samples * samples

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			hits := 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/samples
					py := float64(y) + (float64(sy)+0.5)/samples
					if insideRounded(px, py, float64(size), float64(radius)) {
						hits++
					}
				}
			}
			mask.SetAlpha(x, y, color.Alpha{A: uint8(255 * hits / total)})
		}
	}
	return mask
}

func insideRounded(x, y, size, radius float64) bool {
	if x < 0 || y < 0 || x > size || y > size {
		return false
	}
	cx := min(max(x, radius), size-radius)
	cy := min(max(y, radius), size-radius)
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= radius*radius
}

func drawLetter(img *image.RGBA, letter rune, size int) {
	face := basicfont.Face7x13
	mask := image.NewAlpha(image.Rect(0, 0, 20, 20))
	drawer := &font.Drawer{
		Dst:  mask,
		Src:  image.NewUniform(color.Alpha{A: 255}),
		Face: face,
		Dot:  fixed.P(2, 15),
	}
	drawer.DrawString(string(letter))

	minX, minY, maxX, maxY := mask.Bounds().Max.X, mask.Bounds().Max.Y, -1, -1
	for y := 0; y < mask.Bounds().Max.Y; y++ {
		for x := 0; x < mask.Bounds().Max.X; x++ {
			if mask.AlphaAt(x, y).A > 8 {
				minX = min(minX, x)
				minY = min(minY, y)
				maxX = max(maxX, x)
				maxY = max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return
	}

	sourceWidth := maxX - minX + 1
	sourceHeight := maxY - minY + 1
	targetHeight := max(int(float64(size)*0.46), 8)
	ratio := float64(targetHeight) / float64(sourceHeight)
	targetWidth := max(int(float64(sourceWidth)*ratio), 1)
	offsetX := (size - targetWidth) / 2
	offsetY := (size - targetHeight) / 2

	for y := 0; y < targetHeight; y++ {
		for x := 0; x < targetWidth; x++ {
			sx := minX + int(float64(x)/ratio)
			sy := minY + int(float64(y)/ratio)
			coverage := mask.AlphaAt(sx, sy).A
			if coverage == 0 {
				continue
			}
			px := offsetX + x
			py := offsetY + y
			if !image.Pt(px, py).In(img.Bounds()) {
				continue
			}
			blendWhite(img, px, py, coverage)
		}
	}
}

func blendWhite(img *image.RGBA, x, y int, coverage uint8) {
	existing := img.RGBAAt(x, y)
	alpha := float64(coverage) / 255
	mix := func(base, top uint8) uint8 {
		return uint8(float64(base)*(1-alpha) + float64(top)*alpha)
	}
	img.SetRGBA(x, y, color.RGBA{
		R: mix(existing.R, 255),
		G: mix(existing.G, 255),
		B: mix(existing.B, 255),
		A: max(existing.A, coverage),
	})
}

func iconPath(cfg config.Config, key string) string {
	return filepath.Join(cfg.IconsDir(), key+".png")
}

// CachedIcon returns a cached icon body when it is still fresh.
func CachedIcon(cfg config.Config, key string) []byte {
	path := iconPath(cfg, key)
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > IconTTL {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

// StoreIcon writes an icon body to the cache.
func StoreIcon(cfg config.Config, key string, data []byte) {
	if err := os.MkdirAll(cfg.IconsDir(), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(iconPath(cfg, key), data, 0o644)
}

// DropIcon removes a cached icon so the next request regenerates it.
func DropIcon(cfg config.Config, key string) {
	_ = os.Remove(iconPath(cfg, key))
}

func fetchRemote(url string) []byte {
	if !isPublicHTTP(url) {
		return nil
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil || len(data) < 64 {
		return nil
	}
	if MediaType(data) == "application/octet-stream" {
		return nil
	}
	return data
}

// isPublicHTTP blocks loopback, private and link-local targets so an icon URL
// cannot be pointed at the host's own network.
func isPublicHTTP(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	addresses, err := net.LookupIP(host)
	if err != nil || len(addresses) == 0 {
		return false
	}
	for _, address := range addresses {
		if address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() ||
			address.IsLinkLocalMulticast() || address.IsUnspecified() {
			return false
		}
	}
	return true
}

// RepoIcon resolves a repository icon: override URL, GitHub avatar, then placeholder.
func RepoIcon(cfg config.Config, repo store.Repo) ([]byte, string) {
	if cached := CachedIcon(cfg, repo.Slug); cached != nil {
		return cached, MediaType(cached)
	}

	remote := strings.TrimSpace(repo.IconURL)
	if remote == "" {
		remote = strings.TrimSpace(repo.Cache.Repo.AvatarURL)
	}
	if strings.HasPrefix(remote, "http://") || strings.HasPrefix(remote, "https://") {
		if data := fetchRemote(remote); data != nil {
			StoreIcon(cfg, repo.Slug, data)
			return data, MediaType(data)
		}
	}

	label := repo.DisplayName
	if label == "" {
		label = repo.Name
	}
	data, err := GeneratedIcon(label, repo.TintColor, 256)
	if err != nil {
		return nil, "image/png"
	}
	return data, "image/png"
}

// SourceIcon resolves the source-level icon.
func SourceIcon(cfg config.Config, name, tint string) []byte {
	if cached := CachedIcon(cfg, "_source"); cached != nil {
		return cached
	}
	data, err := GeneratedIcon(name, tint, 256)
	if err != nil {
		return nil
	}
	return data
}

// QRSVG renders a QR code as a standalone SVG document.
func QRSVG(payload string) (string, error) {
	code, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		return "", err
	}
	code.DisableBorder = false
	bitmap := code.Bitmap()
	size := len(bitmap)
	if size == 0 {
		return "", fmt.Errorf("empty QR code")
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`,
		size, size,
	))
	builder.WriteString(`<rect width="100%" height="100%" fill="#fff"/>`)
	builder.WriteString(`<path fill="#000" d="`)
	for y, row := range bitmap {
		for x, dark := range row {
			if dark {
				builder.WriteString(fmt.Sprintf("M%d %dh1v1h-1z", x, y))
			}
		}
	}
	builder.WriteString(`"/></svg>`)
	return builder.String(), nil
}


