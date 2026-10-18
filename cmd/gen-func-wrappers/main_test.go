package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFiles writes the files with their contents by path relative to dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		err := os.MkdirAll(filepath.Dir(path), 0700)
		require.NoError(t, err)
		err = os.WriteFile(path, []byte(content), 0600)
		require.NoError(t, err)
	}
}

// setFlags sets the global flag variables for a test and restores them afterwards.
func setFlags(t *testing.T, validateFlag, printOnlyFlag bool) {
	t.Helper()
	oldValidate, oldPrintOnly, oldLocalImports := validate, printOnly, localImports
	t.Cleanup(func() { validate, printOnly, localImports = oldValidate, oldPrintOnly, oldLocalImports })
	validate, printOnly, localImports = validateFlag, printOnlyFlag, ""
}

// TestRewritePaths_MultiplePaths tests that every path argument is processed
// and that the result reflects a failure of any of them,
// because a CI run of -validate with several paths must check all of them.
func TestRewritePaths_MultiplePaths(t *testing.T) {
	outdated := "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n\nfunc F() {}\n"
	dirA, dirB := t.TempDir(), t.TempDir()
	writeFiles(t, dirA, map[string]string{"a.go": outdated})
	writeFiles(t, dirB, map[string]string{"b.go": outdated})

	setFlags(t, true, false)
	var stdout, stderr bytes.Buffer
	exitCode := rewritePaths([]string{dirA, filepath.Join(dirB, "b.go")}, nil, &stdout, &stderr)
	assert.Equal(t, 1, exitCode, "validation should fail")
	assert.Contains(t, stderr.String(), filepath.Join(dirA, "a.go"), "first path should be validated")
	assert.Contains(t, stderr.String(), filepath.Join(dirB, "b.go"), "second path should be validated")

	setFlags(t, false, false)
	stderr.Reset()
	exitCode = rewritePaths([]string{dirA, filepath.Join(dirB, "b.go")}, nil, &stdout, &stderr)
	require.Equal(t, 0, exitCode, stderr.String())
	for _, file := range []string{filepath.Join(dirA, "a.go"), filepath.Join(dirB, "b.go")} {
		source, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.NotContains(t, string(source), "WrapperTODO", "%s should be rewritten", file)
	}
}

// TestRewritePaths_DetectLocalImportPrefixesRecursive tests that the local
// import prefix is detected from the go.mod file for a recursive path,
// because the generated imports are grouped by it like goimports does.
func TestRewritePaths_DetectLocalImportPrefixesRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":      "module example.com/org/proj\n\ngo 1.22\n",
		"sub/sub.go":  "package sub\n\ntype T struct{}\n",
		"funcs.go":    "package proj\n\nimport \"example.com/org/proj/sub\"\n\nfunc F(t sub.T) {}\n",
		"wrappers.go": "package proj\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n",
	})

	setFlags(t, false, true)
	var stdout, stderr bytes.Buffer
	exitCode := rewritePaths([]string{dir + string(filepath.Separator) + "..."}, nil, &stdout, &stderr)
	require.Equal(t, 0, exitCode, stderr.String())
	// The local import is grouped after the third-party import
	assert.Contains(t, stdout.String(), "\t\"github.com/domonda/go-function\"\n\n\t\"example.com/org/proj/sub\"\n")
}

// TestJSONTypeReplacementsFlag tests that the -replaceForJSON flag
// can be repeated and takes comma separated replacements,
// because the documentation shows both forms.
func TestJSONTypeReplacementsFlag(t *testing.T) {
	replacements := make(map[string]string)
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.Func("replaceForJSON", "", jsonTypeReplacementsFlag(replacements))
	err := flags.Parse([]string{
		"-replaceForJSON=fs.FileReader:fs.File",
		"-replaceForJSON", "io.Reader:*strings.Reader,fmt.Stringer:*strings.Builder",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"fs.FileReader": "fs.File",
		"io.Reader":     "*strings.Reader",
		"fmt.Stringer":  "*strings.Builder",
	}, replacements)

	for _, invalid := range []string{"fs.FileReader", "fs.FileReader:", ":fs.File", "a:b:c"} {
		err = jsonTypeReplacementsFlag(replacements)(invalid)
		assert.Error(t, err, invalid)
		assert.True(t, strings.Contains(err.Error(), invalid), "error should name the invalid value %q", invalid)
	}
}

// TestRewritePaths_ExitCode tests that -validate exits with code 1 only
// if the only failures are missing or outdated wrappers, and with code 2
// for any other error, because CI has to tell generated code that a run
// of the generator fixes apart from an error that the generator can't fix.
func TestRewritePaths_ExitCode(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"outdated/o.go":     "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n\nfunc F() {}\n",
		"unparsable/u.go":   "package p\n\nfunc F( {\n",
		"unresolvable/r.go": "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(Missing)\n",
		// The outdated file comes first in sorted order
		"mixed/a.go":       "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n\nfunc F() {}\n",
		"mixed/b.go":       "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar gWrapper = function.WrapperTODO(Missing)\n",
		"mixedtree/a/a.go": "package a\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n\nfunc F() {}\n",
		"mixedtree/b/b.go": "package b\n\nimport \"github.com/domonda/go-function\"\n\nvar gWrapper = function.WrapperTODO(Missing)\n",
	})
	var (
		outdated     = filepath.Join(dir, "outdated")
		nonexistent  = filepath.Join(dir, "nonexistent")
		unparsable   = filepath.Join(dir, "unparsable")
		unresolvable = filepath.Join(dir, "unresolvable")
	)
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "only outdated wrappers", args: []string{outdated, filepath.Join(outdated, "o.go")}, want: 1},
		{name: "nonexistent path", args: []string{nonexistent}, want: 2},
		{name: "parse error", args: []string{unparsable}, want: 2},
		{name: "unresolvable wrapped function", args: []string{unresolvable}, want: 2},
		// Another error is reported with code 2 whatever the order of the paths
		{name: "outdated wrappers before other error", args: []string{outdated, nonexistent}, want: 2},
		{name: "other error before outdated wrappers", args: []string{nonexistent, outdated}, want: 2},
		// Also after an outdated file of the same path
		{name: "outdated wrappers before other error in a directory", args: []string{filepath.Join(dir, "mixed")}, want: 2},
		{name: "outdated wrappers before other error in a tree", args: []string{filepath.Join(dir, "mixedtree") + "/..."}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setFlags(t, true, false)
			var stdout, stderr bytes.Buffer
			exitCode := rewritePaths(tt.args, nil, &stdout, &stderr)
			assert.Equal(t, tt.want, exitCode, stderr.String())
		})
	}
}

// TestRewritePaths_ReportsAllOutdatedFiles tests that -validate reports
// every file with missing or outdated wrappers, not only the first one,
// so CI shows everything that a run of the generator changes.
func TestRewritePaths_ReportsAllOutdatedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.go": "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar fWrapper = function.WrapperTODO(F)\n\nfunc F() {}\n",
		"b.go": "package p\n\nimport \"github.com/domonda/go-function\"\n\nvar gWrapper = function.WrapperTODO(G)\n\nfunc G() {}\n",
	})
	setFlags(t, true, false)
	var stdout, stderr bytes.Buffer

	exitCode := rewritePaths([]string{dir}, nil, &stdout, &stderr)

	assert.Equal(t, 1, exitCode, stderr.String())
	assert.Contains(t, stderr.String(), "a.go")
	assert.Contains(t, stderr.String(), "b.go")
}
