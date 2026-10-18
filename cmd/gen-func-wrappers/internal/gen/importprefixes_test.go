package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetectLocalImportPrefixes tests auto-detection of local import prefixes from go.mod
func TestDetectLocalImportPrefixes(t *testing.T) {
	// Create a temporary directory structure with a go.mod file
	tmpDir := t.TempDir()

	// Test case 1: go.mod with standard GitHub module path
	goModContent := []byte("module github.com/myorg/myproject\n\ngo 1.21\n")
	goModPath := filepath.Join(tmpDir, "go.mod")
	err := os.WriteFile(goModPath, goModContent, 0644)
	require.NoError(t, err)

	// Test from the same directory
	prefixes := DetectLocalImportPrefixes(tmpDir)
	assert.Equal(t, []string{"github.com/myorg/"}, prefixes, "should detect org prefix from go.mod")

	// Test from a subdirectory
	subDir := filepath.Join(tmpDir, "pkg", "subpkg")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	prefixes = DetectLocalImportPrefixes(subDir)
	assert.Equal(t, []string{"github.com/myorg/"}, prefixes, "should find go.mod in parent directory")

	// Test case 2: go.mod with different hosting
	tmpDir2 := t.TempDir()
	goModContent2 := []byte("module gitlab.com/myteam/project\n\ngo 1.21\n")
	goModPath2 := filepath.Join(tmpDir2, "go.mod")
	err = os.WriteFile(goModPath2, goModContent2, 0644)
	require.NoError(t, err)

	prefixes = DetectLocalImportPrefixes(tmpDir2)
	assert.Equal(t, []string{"gitlab.com/myteam/"}, prefixes, "should work with different hosting platforms")

	// Test case 3: No go.mod file
	tmpDir3 := t.TempDir()
	prefixes = DetectLocalImportPrefixes(tmpDir3)
	assert.Nil(t, prefixes, "should return nil when no go.mod found")

	// Test case 4: Module path with more parts
	tmpDir4 := t.TempDir()
	goModContent4 := []byte("module github.com/domonda/go-function\n\ngo 1.21\n")
	goModPath4 := filepath.Join(tmpDir4, "go.mod")
	err = os.WriteFile(goModPath4, goModContent4, 0644)
	require.NoError(t, err)

	prefixes = DetectLocalImportPrefixes(tmpDir4)
	assert.Equal(t, []string{"github.com/domonda/"}, prefixes, "should extract org prefix from multi-part module path")
}

// TestDetectLocalImportPrefixes_FromFile tests detection when starting from a file path
func TestDetectLocalImportPrefixes_FromFile(t *testing.T) {
	// Create a temporary directory with go.mod and a file
	tmpDir := t.TempDir()
	goModContent := []byte("module github.com/test/repo\n\ngo 1.21\n")
	goModPath := filepath.Join(tmpDir, "go.mod")
	err := os.WriteFile(goModPath, goModContent, 0644)
	require.NoError(t, err)

	// Create a subdirectory with a file
	subDir := filepath.Join(tmpDir, "pkg")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	testFile := filepath.Join(subDir, "test.go")
	err = os.WriteFile(testFile, []byte("package pkg\n"), 0644)
	require.NoError(t, err)

	// Test detection from file path
	prefixes := DetectLocalImportPrefixes(testFile)
	assert.Equal(t, []string{"github.com/test/"}, prefixes, "should detect from file path by searching parent directories")
}

// TestDetectLocalImportPrefixes_InvalidGoMod tests handling of invalid go.mod files
func TestDetectLocalImportPrefixes_InvalidGoMod(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an invalid go.mod (no module line)
	goModContent := []byte("go 1.21\n")
	goModPath := filepath.Join(tmpDir, "go.mod")
	err := os.WriteFile(goModPath, goModContent, 0644)
	require.NoError(t, err)

	prefixes := DetectLocalImportPrefixes(tmpDir)
	assert.Nil(t, prefixes, "should return nil for go.mod without module declaration")
}

// TestDetectLocalImportPrefixes_ShortModulePath tests handling of module paths with fewer than 2 parts
func TestDetectLocalImportPrefixes_ShortModulePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a go.mod with a short module path (unlikely but possible)
	goModContent := []byte("module mymodule\n\ngo 1.21\n")
	goModPath := filepath.Join(tmpDir, "go.mod")
	err := os.WriteFile(goModPath, goModContent, 0644)
	require.NoError(t, err)

	prefixes := DetectLocalImportPrefixes(tmpDir)
	assert.Nil(t, prefixes, "should return nil for module path with < 2 parts")
}
