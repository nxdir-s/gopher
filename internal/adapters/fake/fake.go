// Package fake holds in memory implementations of the secondary ports so the
// core can be tested without touching the filesystem
package fake

import (
	"context"
	"io/fs"
	"sort"
)

// OriginPrefix marks the origin a fake reports, the way the store adapter marks
// the templates it resolves from the embedded set
const OriginPrefix string = "fake:"

type ErrTemplateNotFound struct {
	name string
}

func (e *ErrTemplateNotFound) Error() string {
	return "template not found: " + e.name
}

type ErrFileNotFound struct {
	path string
}

func (e *ErrFileNotFound) Error() string {
	return "file not found: " + e.path
}

// Unwrap satisfies the ports.FileWriter contract: a missing file reads as
// fs.ErrNotExist through errors.Is
func (e *ErrFileNotFound) Unwrap() error {
	return fs.ErrNotExist
}

// Store is an in memory ports.TemplateSource
type Store struct {
	Templates map[string][]byte
}

// NewStore creates a store seeded with the supplied templates
func NewStore(templates map[string]string) *Store {
	store := &Store{
		Templates: make(map[string][]byte, len(templates)),
	}

	for name, src := range templates {
		store.Templates[name] = []byte(src)
	}

	return store
}

// Load returns the source of the named template
func (s *Store) Load(name string) ([]byte, error) {
	src, ok := s.Templates[name]
	if !ok {
		return nil, &ErrTemplateNotFound{name}
	}

	return src, nil
}

// List returns every template name in sorted order
func (s *Store) List() ([]string, error) {
	names := make([]string, 0, len(s.Templates))
	for name := range s.Templates {
		names = append(names, name)
	}

	sort.Strings(names)

	return names, nil
}

// Embedded returns the same names as List, nothing is overridden in memory
func (s *Store) Embedded() ([]string, error) {
	return s.List()
}

// Origin returns a synthetic origin for the named template
func (s *Store) Origin(name string) (string, bool) {
	return OriginPrefix + name, false
}

// Writer is an in memory ports.FileWriter
type Writer struct {
	Files map[string][]byte
}

// NewWriter creates an empty writer
func NewWriter() *Writer {
	return &Writer{
		Files: make(map[string][]byte),
	}
}

// Seed marks a path as already existing
func (w *Writer) Seed(path string, data string) {
	w.Files[path] = []byte(data)
}

// Read returns the data recorded for the supplied path
func (w *Writer) Read(path string) ([]byte, error) {
	data, ok := w.Files[path]
	if !ok {
		return nil, &ErrFileNotFound{path}
	}

	return data, nil
}

// Write records the supplied data
func (w *Writer) Write(ctx context.Context, path string, data []byte) error {
	w.Files[path] = data

	return nil
}

// Exists reports whether the path was written or seeded
func (w *Writer) Exists(path string) bool {
	_, ok := w.Files[path]

	return ok
}

// Paths returns the written paths in sorted order
func (w *Writer) Paths() []string {
	paths := make([]string, 0, len(w.Files))
	for path := range w.Files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	return paths
}
