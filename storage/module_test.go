package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/jungo-dev/junkit/storage"
)

func TestNewService(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uploads")

	svc, err := storage.NewService(storage.Options{BaseDir: dir, BaseURL: "http://localhost/uploads"})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("NewService() returned a nil Service")
	}
}
