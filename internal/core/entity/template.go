package entity

// TemplateInfo describes a template and where it resolves from
type TemplateInfo struct {
	Name       string `json:"name"`
	Origin     string `json:"origin"`
	Overridden bool   `json:"overridden"`
}

// InitResult reports what a template export produced
type InitResult struct {
	Dir     string   `json:"dir"`
	Written []string `json:"written"`
	Skipped []string `json:"skipped"`
}
