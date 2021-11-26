package gen

import "testing"

func Test_exportedName(t *testing.T) {
	tests := map[string]string{
		"id":             "ID",
		"identification": "Identification",
		"documentId":     "DocumentId",
		"xml":            "XML",
		"xmlParser":      "XMLParser",
		"json":           "JSON",
		"jsonParser":     "JSONParser",
		"http":           "HTTP",
		"httpRequest":    "HTTPRequest",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := exportedName(name); got != want {
				t.Errorf("exportedName() = %v, want %v", got, want)
			}
		})
	}
}
