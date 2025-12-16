package gen

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// DetectLocalImportPrefixes attempts to auto-detect local import prefixes
// by finding and parsing the nearest go.mod file starting from the given path.
//
// The function searches upward from the given directory until it finds a go.mod file,
// then extracts the module path to use as the local import prefix.
//
// Parameters:
//   - startPath: The directory path to start searching from
//
// Returns:
//   - A slice containing the detected module path with a trailing slash,
//     or nil if no go.mod file is found
//
// Example:
//
//	If go.mod contains: module github.com/myorg/myproject
//	Returns: []string{"github.com/myorg/"}
func DetectLocalImportPrefixes(startPath string) []string {
	// Clean the path and ensure it's absolute
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil
	}

	// If it's a file, start from its directory
	info, err := os.Stat(absPath)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		absPath = filepath.Dir(absPath)
	}

	// Search upward for go.mod
	currentDir := absPath
	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			// Parse go.mod to extract module path
			modFile, err := modfile.Parse(goModPath, data, nil)
			if err == nil && modFile.Module != nil && modFile.Module.Mod.Path != "" {
				modulePath := modFile.Module.Mod.Path
				// Extract the org/owner prefix (everything up to the last slash before the repo name)
				// e.g., "github.com/domonda/go-function" -> "github.com/domonda/"
				parts := strings.Split(modulePath, "/")
				if len(parts) >= 2 {
					// Use the first two parts (e.g., "github.com/domonda/")
					prefix := strings.Join(parts[:2], "/") + "/"
					return []string{prefix}
				}
			}
			break
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached the root without finding go.mod
			break
		}
		currentDir = parentDir
	}

	return nil
}
