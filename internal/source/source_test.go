package source

import (
	"testing"

	"github.com/sourcebox/sourcebox/internal/github"
	"github.com/sourcebox/sourcebox/internal/store"
)

func TestParseRepoURLShapes(t *testing.T) {
	cases := []struct {
		input string
		owner string
		name  string
	}{
		{"https://github.com/octo/demo", "octo", "demo"},
		{"github.com/octo/demo.git", "octo", "demo"},
		{"https://www.github.com/octo/demo/releases", "octo", "demo"},
		{"  https://github.com/octo/demo/  ", "octo", "demo"},
	}
	for _, tc := range cases {
		owner, name, err := github.ParseRepoURL(tc.input)
		if err != nil {
			t.Fatalf("ParseRepoURL(%q) returned error: %v", tc.input, err)
		}
		if owner != tc.owner || name != tc.name {
			t.Fatalf("ParseRepoURL(%q) = %q/%q, want %q/%q", tc.input, owner, name, tc.owner, tc.name)
		}
	}
}

func TestParseRepoURLRejectsBadInput(t *testing.T) {
	for _, input := range []string{"", "https://gitlab.com/octo/demo", "https://github.com/octo", "not a url"} {
		if _, _, err := github.ParseRepoURL(input); err == nil {
			t.Fatalf("ParseRepoURL(%q) should fail", input)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"v1.2.3": "1.2.3",
		"V2.0":   "2.0",
		"":       "0.0.0",
		"1.0":    "1.0",
	}
	for input, want := range cases {
		if got := NormalizeVersion(input); got != want {
			t.Fatalf("NormalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPickIPAsPrefersReleaseBuilds(t *testing.T) {
	assets := []github.Asset{
		{Name: "App-debug.ipa", Size: 900},
		{Name: "App.ipa", Size: 500},
		{Name: "App-large.ipa", Size: 700},
		{Name: "notes.txt", Size: 10},
	}
	picked := PickIPAs(assets)
	if len(picked) != 3 {
		t.Fatalf("expected 3 .ipa assets, got %d", len(picked))
	}
	want := []string{"App-large.ipa", "App.ipa", "App-debug.ipa"}
	for index, name := range want {
		if picked[index].Name != name {
			t.Fatalf("picked[%d] = %q, want %q", index, picked[index].Name, name)
		}
	}
}

func TestUniqueSlug(t *testing.T) {
	if got := UniqueSlug("My App", map[string]bool{}); got != "my-app" {
		t.Fatalf("got %q", got)
	}
	if got := UniqueSlug("My App", map[string]bool{"my-app": true}); got != "my-app-2" {
		t.Fatalf("got %q", got)
	}
	taken := map[string]bool{"my-app": true, "my-app-2": true}
	if got := UniqueSlug("My App", taken); got != "my-app-3" {
		t.Fatalf("got %q", got)
	}
}

func testRepo() store.Repo {
	return store.Repo{
		Owner:       "octo",
		Name:        "demo",
		Slug:        "demo",
		DisplayName: "Demo App",
		TintColor:   "#5B7CFA",
	}
}

func testRelease() *github.ReleaseInfo {
	return &github.ReleaseInfo{
		Tag:         "v1.4.0",
		Name:        "1.4.0",
		Body:        "Fixes and features.",
		PublishedAt: "2026-01-02T03:04:05Z",
	}
}

func TestAppEntriesSingleAsset(t *testing.T) {
	entries := AppEntries(testRepo(), testRelease(), []github.Asset{
		{Name: "Demo.ipa", URL: "https://example.com/Demo.ipa", Size: 1234},
	}, "https://source.example")

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Name != "Demo App" {
		t.Fatalf("name = %q", entry.Name)
	}
	if entry.BundleIdentifier != "github.octo.demo" {
		t.Fatalf("bundle = %q", entry.BundleIdentifier)
	}
	if entry.IconURL != "https://source.example/icons/demo.png" {
		t.Fatalf("iconURL = %q", entry.IconURL)
	}
	if entry.Versions[0].DownloadURL != "https://example.com/Demo.ipa" {
		t.Fatalf("downloadURL = %q", entry.Versions[0].DownloadURL)
	}
	if entry.Versions[0].Version != "1.4.0" || entry.Versions[0].Size != 1234 {
		t.Fatalf("version payload = %+v", entry.Versions[0])
	}
}

func TestAppEntriesMultipleAssets(t *testing.T) {
	entries := AppEntries(testRepo(), testRelease(), []github.Asset{
		{Name: "Demo.ipa", URL: "https://example.com/a.ipa", Size: 10},
		{Name: "Demo-tweak.ipa", URL: "https://example.com/b.ipa", Size: 20},
	}, "https://source.example")

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].BundleIdentifier == entries[1].BundleIdentifier {
		t.Fatalf("bundle identifiers must differ: %q", entries[0].BundleIdentifier)
	}
	if entries[1].Name != "Demo App · Demo tweak" {
		t.Fatalf("second name = %q", entries[1].Name)
	}
}

