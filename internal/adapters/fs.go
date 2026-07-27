package adapters

import (
	"context"
	"os"
	"path/filepath"
)

const (
	DefaultDirMode  os.FileMode = 0o755
	DefaultFileMode os.FileMode = 0o644
)

type ErrCreateDir struct {
	path string
	err  error
}

func (e *ErrCreateDir) Error() string {
	return "failed to create directory '" + e.path + "': " + e.err.Error()
}

type ErrWriteFile struct {
	path string
	err  error
}

func (e *ErrWriteFile) Error() string {
	return "failed to write '" + e.path + "': " + e.err.Error()
}

type ErrReadFile struct {
	path string
	err  error
}

func (e *ErrReadFile) Error() string {
	return "failed to read '" + e.path + "': " + e.err.Error()
}

type FsOpt func(a *FsAdapter)

// WithFileMode sets the mode used for created files
func WithFileMode(mode os.FileMode) FsOpt {
	return func(a *FsAdapter) {
		a.fileMode = mode
	}
}

// WithDirMode sets the mode used for created directories
func WithDirMode(mode os.FileMode) FsOpt {
	return func(a *FsAdapter) {
		a.dirMode = mode
	}
}

type FsAdapter struct {
	fileMode os.FileMode
	dirMode  os.FileMode
}

// NewFsAdapter creates an adapter for reading and writing files
func NewFsAdapter(opts ...FsOpt) *FsAdapter {
	adapter := &FsAdapter{
		fileMode: DefaultFileMode,
		dirMode:  DefaultDirMode,
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// Write creates the parent directories and writes the supplied data
func (a *FsAdapter) Write(ctx context.Context, path string, data []byte) error {
	if dir := filepath.Dir(path); len(dir) > 0 {
		if err := os.MkdirAll(dir, a.dirMode); err != nil {
			return &ErrCreateDir{dir, err}
		}
	}

	if err := os.WriteFile(path, data, a.fileMode); err != nil {
		return &ErrWriteFile{path, err}
	}

	return nil
}

// Exists reports whether the supplied path is present
func (a *FsAdapter) Exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// Read reads the file at the supplied path
func (a *FsAdapter) Read(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ErrReadFile{path, err}
	}

	return data, nil
}
