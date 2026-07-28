package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	ConfigDirName  string = ".gopher"
	ConfigFileName string = "gopher.json"
	UserFileName   string = "config.json"
	TemplateDir    string = "templates"
	GoModFile      string = "go.mod"
	ModuleKeyword  string = "module"

	XdgConfigEnv  string = "XDG_CONFIG_HOME"
	DefaultOutDir string = "."
)

type ErrReadConfig struct {
	path string
	err  error
}

func (e *ErrReadConfig) Error() string {
	return "failed to read '" + e.path + "': " + e.err.Error()
}

type ErrParseConfig struct {
	path string
	err  error
}

func (e *ErrParseConfig) Error() string {
	return "failed to parse '" + e.path + "': " + e.err.Error()
}

// Config holds the settings gopher resolves before running a command. It is
// merged from the user config, the project config, and the surrounding module
type Config struct {
	Module      string            `json:"module,omitempty"`
	OutDir      string            `json:"out_dir,omitempty"`
	TemplateDir string            `json:"template_dir,omitempty"`
	Defaults    map[string]string `json:"defaults,omitempty"`

	root string
}

// Load resolves the configuration for the supplied working directory. Project
// settings win over user settings, and both lose to explicit flags
func Load(dir string) (*Config, error) {
	cfg := &Config{
		OutDir:   DefaultOutDir,
		Defaults: make(map[string]string),
	}

	if err := cfg.merge(filepath.Join(UserDir(), UserFileName)); err != nil {
		return nil, err
	}

	root, moduleRoot := findMarkers(dir)

	cfg.root = root

	if len(cfg.root) > 0 {
		if err := cfg.merge(filepath.Join(cfg.root, ConfigDirName, ConfigFileName)); err != nil {
			return nil, err
		}
	}

	if len(cfg.Module) == 0 {
		cfg.Module = moduleAt(moduleRoot)
	}

	return cfg, nil
}

// Root returns the detected project root, empty when gopher runs outside one
func (c *Config) Root() string {
	return c.root
}

// TemplateDirs returns the override directories in lookup order. The embedded
// templates are the fallback and are not listed here
func (c *Config) TemplateDirs() []string {
	dirs := make([]string, 0, 3)

	if len(c.root) > 0 {
		dirs = append(dirs, filepath.Join(c.root, ConfigDirName, TemplateDir))
	}

	if len(c.TemplateDir) > 0 {
		dirs = append(dirs, expand(c.TemplateDir))
	}

	dirs = append(dirs, UserTemplateDir())

	return dirs
}

// Default returns the configured default for the supplied flag
func (c *Config) Default(name string) (string, bool) {
	if c.Defaults == nil {
		return "", false
	}

	value, ok := c.Defaults[name]

	return value, ok
}

// merge overlays the config file at the supplied path when it exists
func (c *Config) merge(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return &ErrReadConfig{path, err}
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		return &ErrParseConfig{path, err}
	}

	if len(loaded.Module) > 0 {
		c.Module = loaded.Module
	}

	if len(loaded.OutDir) > 0 {
		c.OutDir = loaded.OutDir
	}

	if len(loaded.TemplateDir) > 0 {
		c.TemplateDir = loaded.TemplateDir
	}

	for name, value := range loaded.Defaults {
		c.Defaults[name] = value
	}

	return nil
}

// UserDir returns the directory holding the user's gopher configuration
func UserDir() string {
	if xdg := os.Getenv(XdgConfigEnv); len(xdg) > 0 {
		return filepath.Join(xdg, "gopher")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".config", "gopher")
}

// UserTemplateDir returns the directory holding the user's template overrides
func UserTemplateDir() string {
	dir := UserDir()
	if len(dir) == 0 {
		return ""
	}

	return filepath.Join(dir, TemplateDir)
}

// findMarkers climbs from dir to the filesystem root, reporting the first
// directory holding a project marker and the first holding a go.mod
//
// One walk serves both. The project root is whichever of .gopher or go.mod
// turns up first, while the module needs the go.mod specifically, so resolving
// the two separately stat'd go.mod at every level twice. Neither marker stops
// the climb until both are settled
func findMarkers(dir string) (string, string) {
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", ""
	}

	var root string
	var module string

	for {
		if len(module) == 0 {
			if _, err := os.Stat(filepath.Join(current, GoModFile)); err == nil {
				module = current

				if len(root) == 0 {
					root = current
				}
			}
		}

		if len(root) == 0 {
			if _, err := os.Stat(filepath.Join(current, ConfigDirName)); err == nil {
				root = current
			}
		}

		if len(root) > 0 && len(module) > 0 {
			return root, module
		}

		parent := filepath.Dir(current)
		if parent == current {
			return root, module
		}

		current = parent
	}
}

// FindModule returns the module path of the go.mod covering the supplied
// directory, or an empty string when the directory is not inside a module
func FindModule(dir string) string {
	return findModule(dir)
}

// findModule walks up looking for a go.mod and returns the module path
func findModule(dir string) string {
	return moduleAt(walkUp(dir, func(current string) bool {
		_, err := os.Stat(filepath.Join(current, GoModFile))

		return err == nil
	}))
}

// moduleAt reads the module path declared by the go.mod in the supplied
// directory, which is empty when the walk found no module
func moduleAt(root string) string {
	if len(root) == 0 {
		return ""
	}

	data, err := os.ReadFile(filepath.Join(root, GoModFile))
	if err != nil {
		return ""
	}

	return parseModule(data)
}

// walkUp climbs from dir to the filesystem root, returning the first match
func walkUp(dir string, match func(string) bool) string {
	current, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	for {
		if match(current) {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}

		current = parent
	}
}

// parseModule pulls the module path out of a go.mod
func parseModule(data []byte) string {
	rest := string(data)

	for len(rest) > 0 {
		line, remainder, _ := strings.Cut(rest, "\n")
		rest = remainder

		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(trimmed, ModuleKeyword) {
			continue
		}

		path := strings.TrimSpace(strings.TrimPrefix(trimmed, ModuleKeyword))
		if len(path) == 0 || strings.HasPrefix(path, "(") {
			continue
		}

		return strings.Trim(path, `"`)
	}

	return ""
}

// expand resolves a leading ~ to the user's home directory
func expand(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
