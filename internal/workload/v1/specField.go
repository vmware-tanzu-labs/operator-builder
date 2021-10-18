package v1

import "fmt"

// APISpecField represents a single field in a custom API type.
type APISpecField struct {
	FieldName          string
	ManifestFieldName  string
	DataType           FieldType
	DefaultVal         string
	ZeroVal            string
	APISpecContent     string
	SampleField        string
	DocumentationLines []string
}

func (api *APISpecField) setSampleAndDefault(name string, value interface{}) {
	if api.DataType == FieldString {
		api.DefaultVal = fmt.Sprintf("%q", value)
		api.SampleField = fmt.Sprintf("%s: %q", name, value)
	} else {
		api.DefaultVal = fmt.Sprintf("%v", value)
		api.SampleField = fmt.Sprintf("%s: %v", name, value)
	}
}
