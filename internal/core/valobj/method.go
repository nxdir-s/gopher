package valobj

// Method is a parsed interface method, shaped for templates that need to
// re-emit the signature and forward a call to it
type Method struct {
	Name       Naming `json:"name"`
	Params     string `json:"params"`
	Results    string `json:"results"`
	Args       string `json:"args"`
	HasResults bool   `json:"has_results"`
}
