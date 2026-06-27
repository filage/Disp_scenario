package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMappedIsDeterministicAndNamespaced(t *testing.T) {
	first := mapped("recording", "legacy-1")
	if first != mapped("recording", "legacy-1") {
		t.Fatal("mapped UUID must be deterministic")
	}
	if first == mapped("run", "legacy-1") {
		t.Fatal("mapped UUID must include entity namespace")
	}
}

func TestLoadLegacyStorePreservesNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	if err := os.WriteFile(path, []byte(`{"videos":[{"id":"v1","sizeBytes":9007199254740993}],"groundTruth":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := loadLegacyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := store["videos"][0]["sizeBytes"].(json.Number)
	if !ok {
		t.Fatalf("expected json.Number, got %T", store["videos"][0]["sizeBytes"])
	}
	if value.String() != "9007199254740993" {
		t.Fatalf("number lost precision: %s", value)
	}
}

func TestNormalizeStoreValue(t *testing.T) {
	timestamp := "2026-06-24T09:34:37.599Z"
	normalized := normalizeStoreValue("createdAt", timestamp)
	parsed, ok := normalized.(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T", normalized)
	}
	if parsed.Format(time.RFC3339Nano) != timestamp {
		t.Fatalf("unexpected timestamp: %s", parsed)
	}

	payload := normalizeStoreValue("payload", map[string]any{"ok": true})
	if string(payload.([]byte)) != `{"ok":true}` {
		t.Fatalf("unexpected JSON payload: %s", payload)
	}
}

func TestStoreKeyCoversMigrationSpecs(t *testing.T) {
	for _, spec := range specs() {
		if storeKey(spec.oldTable) == "" {
			t.Fatalf("missing store key for %s", spec.oldTable)
		}
	}
}

func TestResolveLegacyPathMapsWindowsProjectPathToMountedRoot(t *testing.T) {
	t.Setenv("LEGACY_ROOT", "/legacy")

	got := resolveLegacyPath(`D:\codex_projects\dev_33\analyst-app\uploads\video-file`)
	want := filepath.Join("/legacy", "uploads", "video-file")
	if got != want {
		t.Fatalf("unexpected mounted path: got %q, want %q", got, want)
	}
}

func TestResolveLegacyPathKeepsRelativePathUnderLegacyRoot(t *testing.T) {
	t.Setenv("LEGACY_ROOT", "/legacy")

	got := resolveLegacyPath(filepath.Join("uploads", "video-file"))
	want := filepath.Join("/legacy", "uploads", "video-file")
	if got != want {
		t.Fatalf("unexpected relative path: got %q, want %q", got, want)
	}
}
