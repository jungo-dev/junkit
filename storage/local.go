package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// LocalService implements Service using the local filesystem.
type LocalService struct {
	baseDir string
	baseURL string
}

// NewLocalService creates a new LocalService instance.
//
// Usage:
//
//	svc, err := storage.NewLocalService("./uploads", "http://localhost:8080/uploads")
func NewLocalService(baseDir, baseURL string) (*LocalService, error) {
	if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create storage directory %q: %w", baseDir, err)
	}

	return &LocalService{baseDir: baseDir, baseURL: baseURL}, nil
}

// UploadFile stores a file under a generated unique filename.
func (s *LocalService) UploadFile(_ context.Context, file io.Reader, originalFilename string) (string, error) {
	ext := filepath.Ext(originalFilename)
	uniqueFilename := fmt.Sprintf("%s_%d%s", uuid.New().String(), time.Now().Unix(), ext)
	dst := filepath.Join(s.baseDir, uniqueFilename)

	out, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		return "", fmt.Errorf("failed to copy file contents: %w", err)
	}

	return fmt.Sprintf("%s/%s", s.baseURL, uniqueFilename), nil
}

// DeleteFile removes a previously uploaded file from the local filesystem.
func (s *LocalService) DeleteFile(_ context.Context, fileURL string) error {
	filename := filepath.Base(fileURL)
	if filename == "" || filename == "." || filename == "/" {
		return fmt.Errorf("invalid file URL: %s", fileURL)
	}

	dst := filepath.Join(s.baseDir, filename)
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}
