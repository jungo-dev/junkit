package storage

import "go.uber.org/fx"

// Options configures local file storage.
type Options struct {
	// BaseDir is the local directory files are written to, e.g. "./uploads".
	BaseDir string
	// BaseURL is the URL prefix under which BaseDir is served, e.g. "http://localhost:8080/uploads".
	BaseURL string
}

// Module provides an Fx provider for storage Service.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(storage.Options{BaseDir: "./uploads", BaseURL: cfg.PublicURL + "/uploads"}),
//	    storage.Module,
//	)
var Module = fx.Module("storage",
	fx.Provide(NewService),
)

// NewService constructs a storage Service instance.
func NewService(opts Options) (Service, error) {
	return NewLocalService(opts.BaseDir, opts.BaseURL)
}
