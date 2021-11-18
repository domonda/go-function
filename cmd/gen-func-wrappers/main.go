package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/domonda/go-function/cmd/gen-func-wrappers/gen"
)

var (
	// genFilename   string
	// namePrefix    string
	// exportedFuncs bool
	verbose   bool
	printOnly bool
	printHelp bool
)

func main() {
	// flag.BoolVar(&exportedFuncs, "exported", false, "generate function.Wrapper implementation types exported package functions")
	// flag.StringVar(&genFilename, "genfile", "generated.go", "name of the file to be generated")
	// flag.StringVar(&namePrefix, "prefix", "Func", "prefix for function type names in the same package")
	flag.BoolVar(&verbose, "verbose", false, "prints information of what's happening")
	flag.BoolVar(&printOnly, "print", false, "prints to stdout instead of writing files")
	flag.BoolVar(&printHelp, "help", false, "prints this help output")
	flag.Parse()
	if printHelp {
		flag.PrintDefaults()
		os.Exit(2)
	}

	var (
		args     = flag.Args()
		cwd, _   = os.Getwd()
		filePath string
	)
	if len(args) == 0 {
		filePath = cwd
	} else {
		recursive := strings.HasSuffix(args[0], "...")
		if args[0] == "." || args[0] == "./..." {
			filePath = cwd
		} else {
			filePath = filepath.Clean(strings.TrimSuffix(args[0], "..."))
		}
		if recursive {
			filePath = filepath.Join(filePath, "...")
		}
	}
	info, err := os.Stat(strings.TrimSuffix(filePath, "..."))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-func-wrappers error:", err)
		os.Exit(2)
	}

	var printOnlyWriter io.Writer
	if printOnly {
		printOnlyWriter = os.Stdout
	}
	if info.IsDir() {
		err = gen.RewriteDir(filePath, verbose, printOnlyWriter)
	} else {
		err = gen.RewriteFile(filePath, verbose, printOnlyWriter)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-func-wrappers error:", err)
		os.Exit(2)
	}
}