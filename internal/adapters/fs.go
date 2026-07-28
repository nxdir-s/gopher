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

// Unwrap exposes the cause so errors.Is sees fs.ErrNotExist through the wrap,
// which the ports.FileWriter contract requires of Read
func (e *ErrReadFile) Unwrap() error {
	return e.err
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
	made     map[string]struct{}
}

// NewFsAdapter creates an adapter for reading and writing files
func NewFsAdapter(opts ...FsOpt) *FsAdapter {
	adapter := &FsAdapter{
		fileMode: DefaultFileMode,
		dirMode:  DefaultDirMode,
		made:     make(map[string]struct{}, 8),
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// Write creates the parent directories and writes the supplied data
//
// A directory this adapter already created is not created again. One request
// writes a batch of files into a handful of directories, so the repeats are the
// common case, and gopher is a short lived process whose output tree nothing
// else is editing underneath it
func (a *FsAdapter) Write(ctx context.Context, path string, data []byte) error {
	if dir := filepath.Dir(path); len(dir) > 0 {
		if _, made := a.made[dir]; !made {
			if err := os.MkdirAll(dir, a.dirMode); err != nil {
				return &ErrCreateDir{dir, err}
			}

			a.made[dir] = struct{}{}
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
