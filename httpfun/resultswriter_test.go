package httpfun_test

import (
	"os"
	"path"
	"strings"
	"testing"

	"github.com/domonda/go-function/httpfun"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectContentType(t *testing.T) {
	testdataDir := "testdata"

	scenarios := []struct {
		filename            string
		expectedContentType string
	}{
		{filename: "example.json", expectedContentType: "application/json"},
		{filename: "plain", expectedContentType: "text/plain"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.filename, func(t *testing.T) {
			// given
			file := path.Join(testdataDir, scenario.filename)
			content, err := os.ReadFile(file)
			require.NoError(t, err)

			// when
			contentType := httpfun.DetectContentType(content)

			// then
			t.Logf("Actual content type: '%s'", contentType)
			assert.True(t, strings.Contains(contentType, scenario.expectedContentType))
		})
	}
}
