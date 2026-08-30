package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jungo-dev/junkit/storage"
)

func TestNewLocalService_CreatesBaseDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uploads")

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("test setup invariant broken: %s should not exist yet", dir)
	}

	if _, err := storage.NewLocalService(dir, "http://localhost/uploads"); err != nil {
		t.Fatalf("NewLocalService() error = %v", err)
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("NewLocalService() did not create %s as a directory", dir)
	}
}

func TestLocalService_UploadFile(t *testing.T) {
	dir := t.TempDir()
	svc, err := storage.NewLocalService(dir, "http://localhost:8080/uploads")
	if err != nil {
		t.Fatalf("NewLocalService() error = %v", err)
	}

	content := "avatar bytes"
	url, err := svc.UploadFile(context.Background(), strings.NewReader(content), "avatar.png")
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}

	if !strings.HasPrefix(url, "http://localhost:8080/uploads/") {
		t.Fatalf("UploadFile() url = %q, want it prefixed with the base URL", url)
	}
	if !strings.HasSuffix(url, ".png") {
		t.Fatalf("UploadFile() url = %q, want the original extension preserved", url)
	}

	filename := filepath.Base(url)
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		t.Fatalf("uploaded file not found on disk at %s: %v", filename, err)
	}
	if string(data) != content {
		t.Fatalf("file contents = %q, want %q", data, content)
	}
}

func TestLocalService_UploadFile_UniqueNamesPerCall(t *testing.T) {
	dir := t.TempDir()
	svc, _ := storage.NewLocalService(dir, "http://localhost/uploads")

	url1, err := svc.UploadFile(context.Background(), strings.NewReader("a"), "same-name.jpg")
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	url2, err := svc.UploadFile(context.Background(), strings.NewReader("b"), "same-name.jpg")
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}

	if url1 == url2 {
		t.Fatalf("two uploads of the same original filename produced the same stored URL: %q", url1)
	}
}

func TestLocalService_DeleteFile(t *testing.T) {
	dir := t.TempDir()
	svc, _ := storage.NewLocalService(dir, "http://localhost/uploads")

	url, err := svc.UploadFile(context.Background(), strings.NewReader("content"), "a.txt")
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}

	if err := svc.DeleteFile(context.Background(), url); err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, filepath.Base(url))); !os.IsNotExist(err) {
		t.Fatal("file still exists on disk after DeleteFile()")
	}
}

func TestLocalService_DeleteFile_MissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	svc, _ := storage.NewLocalService(dir, "http://localhost/uploads")

	if err := svc.DeleteFile(context.Background(), "http://localhost/uploads/does-not-exist.png"); err != nil {
		t.Fatalf("DeleteFile() on a missing file error = %v, want nil", err)
	}
}

func TestLocalService_DeleteFile_GenuineRemovalErrorIsReturned(t *testing.T) {
	dir := t.TempDir()
	svc, _ := storage.NewLocalService(dir, "http://localhost/uploads")

	// os.Remove fails on a non-empty directory with an error other than
	// "not exist" — the one DeleteFile is documented to swallow silently.
	nested := filepath.Join(dir, "not-a-plain-file")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("test setup: failed to create nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "child"), []byte("x"), 0o644); err != nil {
		t.Fatalf("test setup: failed to create nested file: %v", err)
	}

	if err := svc.DeleteFile(context.Background(), "http://localhost/uploads/not-a-plain-file"); err == nil {
		t.Fatal("DeleteFile() error = nil, want a genuine removal error for a non-empty directory")
	}
}

func TestLocalService_DeleteFile_InvalidURL(t *testing.T) {
	dir := t.TempDir()
	svc, _ := storage.NewLocalService(dir, "http://localhost/uploads")

	tests := []string{"", "/", "."}
	for _, url := range tests {
		if err := svc.DeleteFile(context.Background(), url); err == nil {
			t.Errorf("DeleteFile(%q) error = nil, want an error for an invalid URL", url)
		}
	}
}
