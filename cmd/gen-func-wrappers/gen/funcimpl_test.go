package gen

import "testing"

func Test_exportedName(t *testing.T) {
	tests := map[string]string{
		"acl":            "ACL",
		"aclFile":        "ACLFile",
		"api":            "API",
		"apiKey":         "APIKey",
		"documentId":     "DocumentId",
		"http":           "HTTP",
		"httpRequest":    "HTTPRequest",
		"id":             "ID",
		"identification": "Identification",
		"json":           "JSON",
		"jsonParser":     "JSONParser",
		"xml":            "XML",
		"xmlParser":      "XMLParser",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := exportedName(name); got != want {
				t.Errorf("exportedName() = %v, want %v", got, want)
			}
		})
	}
}
