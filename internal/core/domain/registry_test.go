package domain

import (
	"strings"
	"testing"

	"github.com/nxdir-s/gopher/internal/adapters"
	"github.com/nxdir-s/gopher/internal/core/entity"
	"github.com/nxdir-s/gopher/internal/core/valobj"
	"github.com/nxdir-s/gopher/templates"
)

// TestRegistryIsWellFormed guards the invariants the cli and describe output
// rely on. A malformed spec would otherwise only surface at generation time
func TestRegistryIsWellFormed(t *testing.T) {
	registry := NewRegistry()

	seen := make(map[valobj.GenType]struct{})

	for _, spec := range registry.Specs() {
		t.Run(spec.Type.String(), func(t *testing.T) {
			if _, ok := seen[spec.Type]; ok {
				t.Fatalf("duplicate spec for type %s", spec.Type)
			}

			seen[spec.Type] = struct{}{}

			if spec.Type == valobj.GenUnknown {
				t.Error("spec has no type")
			}

			if len(spec.Summary) == 0 {
				t.Error("spec has no summary")
			}

			if len(spec.Templates) == 0 {
				t.Error("spec declares no templates")
			}

			for i := range spec.Templates {
				if len(spec.Templates[i].Name) == 0 || len(spec.Templates[i].Out) == 0 {
					t.Errorf("template ref %d is incomplete: %+v", i, spec.Templates[i])
				}
			}

			names := make(map[string]struct{}, len(spec.Flags))

			for i := range spec.Flags {
				flagSpec := spec.Flags[i]

				if len(flagSpec.Name) == 0 {
					t.Errorf("flag %d has no name", i)
				}

				if _, ok := names[flagSpec.Name]; ok {
					t.Errorf("duplicate flag: %s", flagSpec.Name)
				}

				names[flagSpec.Name] = struct{}{}

				if len(flagSpec.Usage) == 0 {
					t.Errorf("flag %s has no usage", flagSpec.Name)
				}

				if flagSpec.Required && len(flagSpec.Default) > 0 {
					t.Errorf("flag %s is required but has a default", flagSpec.Name)
				}

				if flagSpec.Type == entity.FlagBool && flagSpec.Default != "true" && flagSpec.Default != "false" {
					t.Errorf("bool flag %s has a non bool default: %q", flagSpec.Name, flagSpec.Default)
				}
			}
		})
	}
}

// TestRegistryTemplatesResolve checks that every static template a spec
// references is present in the embedded set
func TestRegistryTemplatesResolve(t *testing.T) {
	store := adapters.NewStoreAdapter(templates.FS, templates.Root)

	for _, spec := range NewRegistry().Specs() {
		for i := range spec.Templates {
			name := spec.Templates[i].Name

			if strings.Contains(name, "{{") {
				continue
			}

			t.Run(spec.Type.String()+"/"+name, func(t *testing.T) {
				if _, err := store.Load(name); err != nil {
					t.Errorf("%s", err.Error())
				}
			})
		}
	}
}

// TestKindsHaveTemplates checks that every advertised kind is shipped
func TestKindsHaveTemplates(t *testing.T) {
	store := adapters.NewStoreAdapter(templates.FS, templates.Root)

	kinds := map[string][]string{
		"adapter": AdapterKinds,
		"valobj":  ValobjKinds,
		"module":  ModuleKinds,
	}

	for prefix, names := range kinds {
		for _, kind := range names {
			t.Run(prefix+"/"+kind, func(t *testing.T) {
				if _, err := store.Load(prefix + "/" + kind); err != nil {
					t.Errorf("%s", err.Error())
				}
			})
		}
	}
}

func TestRegistrySpecLookup(t *testing.T) {
	registry := NewRegistry()

	spec, err := registry.Spec(valobj.GenAdapter)
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	if spec.Type != valobj.GenAdapter {
		t.Errorf("got type %s, want adapter", spec.Type)
	}

	if _, err := registry.Spec(valobj.GenUnknown); err == nil {
		t.Error("expected an error for an unregistered type")
	}
}
