// Package storage provides file upload and deletion abstractions.
package storage

import (
	"context"
	"io"
)

// Service defines the interface for uploading and deleting files.
type Service interface {
	// UploadFile stores a file and returns its URL or path.
	UploadFile(ctx context.Context, file io.Reader, originalFilename string) (string, error)
	// DeleteFile removes a previously uploaded file by URL.
	DeleteFile(ctx context.Context, fileURL string) error
}
