package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestList_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ls, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	keys, err := ls.List(context.Background(), "nonexistent/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestList_WithFiles(t *testing.T) {
	dir := t.TempDir()
	ls, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create files under a prefix dir
	prefix := "meta/fake-uuid"
	mustWrite(t, filepath.Join(dir, prefix, "metadata-20260101-000000.dump.age"), "a")
	mustWrite(t, filepath.Join(dir, prefix, "metadata-20260102-000000.dump.age"), "b")
	mustWrite(t, filepath.Join(dir, prefix, "metadata-20260103-000000.dump.age"), "c")

	keys, err := ls.List(context.Background(), prefix+"/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(keys), keys)
	}
	expected := []string{
		"meta/fake-uuid/metadata-20260101-000000.dump.age",
		"meta/fake-uuid/metadata-20260102-000000.dump.age",
		"meta/fake-uuid/metadata-20260103-000000.dump.age",
	}
	for i, want := range expected {
		if keys[i] != want {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], want)
		}
	}
}

func TestList_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	ls, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	prefix := "meta"
	mustWrite(t, filepath.Join(dir, prefix, "file.txt"), "x")
	mustWrite(t, filepath.Join(dir, prefix, "subdir/.keep"), "")

	keys, err := ls.List(context.Background(), prefix+"/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "meta/file.txt" {
		t.Errorf("expected [meta/file.txt], got %v", keys)
	}
}

func TestList_Sorted(t *testing.T) {
	dir := t.TempDir()
	ls, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	prefix := "meta/fake-uuid"
	mustWrite(t, filepath.Join(dir, prefix, "metadata-20260103-000000.dump.age"), "c")
	mustWrite(t, filepath.Join(dir, prefix, "metadata-20260101-000000.dump.age"), "a")
	mustWrite(t, filepath.Join(dir, prefix, "metadata-20260102-000000.dump.age"), "b")

	keys, err := ls.List(context.Background(), prefix+"/")
	if err != nil {
		t.Fatal(err)
	}
	if keys[0] > keys[1] || keys[1] > keys[2] {
		t.Errorf("expected lexical sort, got %v", keys)
	}
}
