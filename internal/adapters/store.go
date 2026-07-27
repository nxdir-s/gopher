package adapters

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const TemplateExt string = ".tmpl"

// EmbeddedPrefix marks templates that resolve to the compiled in defaults
const EmbeddedPrefix string = "embedded:"

type ErrTemplateNotFound struct {
	name string
}

func (e *ErrTemplateNotFound) Error() string {
	return "template not found: " + e.name
}

type ErrWalkTemplates struct {
	dir string
	err error
}

func (e *ErrWalkTemplates) Error() string {
	return "failed to walk templates in '" + e.dir + "': " + e.err.Error()
}

type StoreOpt func(a *StoreAdapter)

// WithTemplateDir prepends a directory to the template lookup chain. Directories
// added first are searched first
func WithTemplateDir(dir string) StoreOpt {
	return func(a *StoreAdapter) {
		if len(dir) == 0 {
			return
		}

		a.dirs = append(a.dirs, dir)
	}
}

type StoreAdapter struct {
	embedded fs.FS
	root     string
	dirs     []string
}

// NewStoreAdapter creates an adapter that resolves templates from the override
// directories first and falls back to the embedded defaults
func NewStoreAdapter(embedded fs.FS, root string, opts ...StoreOpt) *StoreAdapter {
	adapter := &StoreAdapter{
		embedded: embedded,
		root:     root,
		dirs:     make([]string, 0, 2),
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// Load returns the source of the named template
func (a *StoreAdapter) Load(name string) ([]byte, error) {
	for i := range a.dirs {
		override := filepath.Join(a.dirs[i], filepath.FromSlash(name)+TemplateExt)

		data, err := os.ReadFile(override)
		if err == nil {
			return data, nil
		}

		if !os.IsNotExist(err) {
			return nil, &ErrReadFile{override, err}
		}
	}

	data, err := fs.ReadFile(a.embedded, path.Join(a.root, name+TemplateExt))
	if err != nil {
		return nil, &ErrTemplateNotFound{name}
	}

	return data, nil
}

// List returns every template name available, embedded or overridden, sorted
func (a *StoreAdapter) List() ([]string, error) {
	names, err := a.Embedded()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(names))
	for i := range names {
		seen[names[i]] = struct{}{}
	}

	for i := range a.dirs {
		overrides, err := a.walkDir(a.dirs[i])
		if err != nil {
			return nil, err
		}

		for j := range overrides {
			if _, ok := seen[overrides[j]]; ok {
				continue
			}

			seen[overrides[j]] = struct{}{}
			names = append(names, overrides[j])
		}
	}

	sort.Strings(names)

	return names, nil
}

// Embedded returns the names of the templates compiled into the binary
func (a *StoreAdapter) Embedded() ([]string, error) {
	names := make([]string, 0, 16)

	err := fs.WalkDir(a.embedded, a.root, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.HasSuffix(entryPath, TemplateExt) {
			return nil
		}

		name := strings.TrimSuffix(strings.TrimPrefix(entryPath, a.root+"/"), TemplateExt)
		names = append(names, name)

		return nil
	})
	if err != nil {
		return nil, &ErrWalkTemplates{a.root, err}
	}

	sort.Strings(names)

	return names, nil
}

// walkDir returns the template names found in an override directory
func (a *StoreAdapter) walkDir(dir string) ([]string, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, nil
	}

	names := make([]string, 0, 8)

	err := filepath.WalkDir(dir, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.HasSuffix(entryPath, TemplateExt) {
			return nil
		}

		rel, err := filepath.Rel(dir, entryPath)
		if err != nil {
			return err
		}

		names = append(names, strings.TrimSuffix(filepath.ToSlash(rel), TemplateExt))

		return nil
	})
	if err != nil {
		return nil, &ErrWalkTemplates{dir, err}
	}

	return names, nil
}

// Origin returns the path the named template resolves to and whether it came
// from an override rather than the embedded defaults
func (a *StoreAdapter) Origin(name string) (string, bool) {
	for i := range a.dirs {
		override := filepath.Join(a.dirs[i], filepath.FromSlash(name)+TemplateExt)

		if _, err := os.Stat(override); err == nil {
			return override, true
		}
	}

	return EmbeddedPrefix + path.Join(a.root, name+TemplateExt), false
}
