package entity

// ArtifactMode describes how an artifact reaches its destination
type ArtifactMode int

const (
	// ModeCreate writes a new file and refuses to clobber an existing one
	ModeCreate ArtifactMode = iota
	// ModeAppend merges the rendered declarations into an existing file
	ModeAppend
	// ModeEnsure writes the file only when it is missing. It suits scaffolding
	// at a fixed path that the caller is expected to edit afterwards
	ModeEnsure
)

// String returns the name of the mode
func (m ArtifactMode) String() string {
	switch m {
	case ModeCreate:
		return "create"
	case ModeAppend:
		return "append"
	case ModeEnsure:
		return "ensure"
	default:
		return "create"
	}
}

// MarshalJSON encodes the mode as its name
func (m ArtifactMode) MarshalJSON() ([]byte, error) {
	return []byte(`"` + m.String() + `"`), nil
}

// ArtifactStatus reports what happened to an artifact
type ArtifactStatus int

const (
	StatusCreated ArtifactStatus = iota
	StatusAppended
	StatusUnchanged
)

// String returns the name of the status
func (s ArtifactStatus) String() string {
	switch s {
	case StatusCreated:
		return "created"
	case StatusAppended:
		return "appended"
	case StatusUnchanged:
		return "unchanged"
	default:
		return "created"
	}
}

// Artifact is a single rendered file waiting to be written
type Artifact struct {
	Path     string         `json:"path"`
	Template string         `json:"template"`
	Mode     ArtifactMode   `json:"mode"`
	Status   ArtifactStatus `json:"-"`
	Content  []byte         `json:"-"`
}
