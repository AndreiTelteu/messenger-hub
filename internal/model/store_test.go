package model

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProfileLockContentionAndRelease(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	first := NewStore(config, filepath.Join(root, "data-a"), filepath.Join(root, "cache-a"))
	second := NewStore(config, filepath.Join(root, "data-b"), filepath.Join(root, "cache-b"))

	releaseFirst, err := first.AcquireLock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.AcquireLock(); !errors.Is(err, ErrProfileLocked) {
		t.Fatalf("second lock error = %v, want ErrProfileLocked", err)
	}
	releaseFirst()
	releaseFirst() // release is idempotent

	releaseSecond, err := second.AcquireLock()
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	releaseSecond()
	info, err := os.Stat(filepath.Join(config, ".lock"))
	if err != nil {
		t.Fatalf("lock file was removed: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("lock mode = %o, want 600", got)
	}
}

func TestProfileLocksAreIndependent(t *testing.T) {
	root := t.TempDir()
	first := NewStore(filepath.Join(root, "one"), filepath.Join(root, "data"), filepath.Join(root, "cache"))
	second := NewStore(filepath.Join(root, "two"), filepath.Join(root, "data"), filepath.Join(root, "cache"))
	releaseFirst, err := first.AcquireLock()
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	releaseSecond, err := second.AcquireLock()
	if err != nil {
		t.Fatalf("independent profile lock: %v", err)
	}
	releaseSecond()
}

func TestStoreRoundTripAndPermissions(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "config"), filepath.Join(root, "data"), filepath.Join(root, "cache"))
	state := NewState()
	first := mustService(t, "Work", "https://work.example/app")
	second := mustService(t, "Personal", "personal.example")
	if err := state.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := state.Add(second); err != nil {
		t.Fatal(err)
	}
	if err := state.Move(second.ID, 1); err != nil {
		t.Fatal(err)
	}
	state.Window = WindowGeometry{Width: 1234, Height: 777, X: 42, Y: 24, Positioned: true}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, state) {
		t.Fatalf("loaded state differs\ngot:  %#v\nwant: %#v", loaded, state)
	}
	assertMode(t, filepath.Join(root, "config"), 0700)
	assertMode(t, filepath.Join(root, "config", "state.json"), 0600)
	entries, err := os.ReadDir(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("temporary file left behind: %v", entries)
	}
}

func TestStoreMissingAndInvalidConfig(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	store := NewStore(config, filepath.Join(root, "data"), filepath.Join(root, "cache"))
	got, err := store.Load()
	if err != nil || !reflect.DeepEqual(got, NewState()) {
		t.Fatalf("missing config = %#v, %v", got, err)
	}
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	bad := []byte(`{"version":1,"services":[{"id":"bad","name":"Oops","kind":"custom","url":"https://example.com","enabled":true}]}`)
	if err := os.WriteFile(filepath.Join(config, "state.json"), bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "invalid service ID") {
		t.Fatalf("invalid config error = %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(config, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(contents, bad) {
		t.Fatal("Load overwrote invalid config")
	}
}

func TestStoreRejectsTrailingJSON(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "state.json"), []byte(`{"version":1,"services":[]} {}`), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(config, filepath.Join(root, "data"), filepath.Join(root, "cache"))
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "trailing content") {
		t.Fatalf("trailing JSON error = %v", err)
	}
}

func TestProfilePathsAreIsolatedAndRejectTraversal(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "config"), filepath.Join(root, "data"), filepath.Join(root, "cache"))
	a := mustService(t, "A", "a.example")
	b := mustService(t, "B", "b.example")
	aData, err := store.DataProfileDir(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	bData, err := store.DataProfileDir(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	aCache, err := store.CacheProfileDir(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if aData == bData || aData == aCache {
		t.Fatalf("profile paths are not isolated: %q %q %q", aData, bData, aCache)
	}
	if filepath.Base(aData) != a.ID || filepath.Base(aCache) != a.ID {
		t.Fatal("profile path is not based on service ID")
	}
	for _, id := range []string{"../escape", a.ID + "/child", "WHATSAPP", ""} {
		if _, err := store.DataProfileDir(id); err == nil {
			t.Fatalf("accepted unsafe ID %q", id)
		}
	}
}

func TestSaveRejectsInvalidStateBeforeReplacingFile(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "config"), filepath.Join(root, "data"), filepath.Join(root, "cache"))
	good := NewState()
	if err := store.Save(good); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config", "state.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.Version = 99
	if err := store.Save(bad); err == nil {
		t.Fatal("Save accepted unsupported version")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("invalid Save replaced existing state")
	}
}

func TestSaveRejectsWindowCoordinatesOutsideX11Range(t *testing.T) {
	root := t.TempDir()
	store := NewStore(filepath.Join(root, "config"), filepath.Join(root, "data"), filepath.Join(root, "cache"))
	state := NewState()
	state.Window = WindowGeometry{Width: 1100, Height: 720, X: 1 << 31, Positioned: true}
	if err := store.Save(state); err == nil || !strings.Contains(err.Error(), "coordinate range") {
		t.Fatalf("invalid window position error = %v", err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %o, want %o", path, got, want)
	}
}
