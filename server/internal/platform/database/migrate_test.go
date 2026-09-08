package database

import (
	"net/url"
	"path/filepath"
	"testing"
)

func TestMigrationSourceURLUsesAValidAbsoluteFileURL(t *testing.T) {
	directory := t.TempDir()
	got, err := migrationSourceURL(directory)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse %q: %v", got, err)
	}
	if parsed.Scheme != "file" || (parsed.Path == "" && parsed.Opaque == "") {
		t.Fatalf("migration source URL = %q", got)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := parsed.Path
	if parsed.Opaque != "" {
		sourcePath = parsed.Opaque
	}
	wantPath := filepath.ToSlash(absolute)
	if sourcePath != wantPath {
		t.Fatalf("migration source path = %q, want %q", sourcePath, wantPath)
	}
}
