package main

//go:generate gen-func-wrappers -replaceForJSON=fs.FileReader:fs.File $GOFILE

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/ungerik/go-fs"
	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/htmlform"
	"github.com/domonda/go-function/httpfun"
	"github.com/domonda/golog/log"
)

func main() {
	httperr.DebugShowInternalErrorsInResponse = true

	function.StringScanners = function.StringScanners.
		WithTypeScanner(
			reflect.TypeFor[fs.FileReader](),
			function.StringScannerFunc(func(sourceStr string, destPtr any) error {
				*destPtr.(*fs.FileReader) = fs.File(sourceStr)
				return nil
			}),
		)

	handler, err := htmlform.NewHandler(wrappedExample, "Example Form", httpfun.RespondStaticHTML("<h1>Success!</h1>"))
	if err != nil {
		log.FatalAndPanic(err)
	}

	handler.SetArgDefaultValue("aBool", true)
	handler.SetArgDefaultValue("anInt", 666)
	handler.SetArgDefaultValue("aFloat", 3.1415)

	handler.SetArgOptions(
		"color",
		[]htmlform.Option{
			{Label: "Red", Value: ColorRed},
			{Label: "Green", Value: ColorGreen},
			{Label: "Blue", Value: ColorBlue},
		},
	)
	handler.SetArgDefaultValue("color", ColorGreen)

	log.Info("Listening on http://localhost:8080").Log()
	err = http.ListenAndServe(":8080", handler)
	log.FatalAndPanic(err)
}

type Color int

const (
	ColorRed = iota
	ColorGreen
	ColorBlue
)

// Example function
//
// Arguments:
//   - aBool:  A bool
//   - anInt:  An integer
//   - aFloat: A float
//   - color:  Select a color
//   - file:   Upload file
func Example(ctx context.Context, aBool bool, anInt int, aFloat float64, color Color, file fs.FileReader /*, aDate date.Date, aTime time.Time*/) error {
	log.InfoCtx(ctx, "Example").
		Bool("aBool", aBool).
		Int("anInt", anInt).
		Float("aFloat", aFloat).
		Any("color", color).
		Str("file", file.Name()).
		// Str("aDate", string(aDate)).
		// Time("aTime", aTime).
		Log()

	return nil
}

// Replace wrappedExample and wrappedExampleT further below
// with the following var statement using function.WrapperTODO
// and run go generate to test creating a fresh new wrapper:
// var wrappedExample = function.WrapperTODO(Example)

// wrappedExample wraps Example as function.Wrapper (generated code)
var wrappedExample wrappedExampleT

// wrappedExampleT wraps Example as function.Wrapper (generated code)
type wrappedExampleT struct{}

func (wrappedExampleT) String() string {
	return "Example(ctx context.Context, aBool bool, anInt int, aFloat float64, color Color, file fs.FileReader) error"
}

func (wrappedExampleT) Name() string {
	return "Example"
}

func (wrappedExampleT) NumArgs() int      { return 6 }
func (wrappedExampleT) ContextArg() bool  { return true }
func (wrappedExampleT) NumResults() int   { return 1 }
func (wrappedExampleT) ErrorResult() bool { return true }

func (wrappedExampleT) ArgNames() []string {
	return []string{"ctx", "aBool", "anInt", "aFloat", "color", "file"}
}

func (wrappedExampleT) ArgDescriptions() []string {
	return []string{"", "A bool", "An integer", "A float", "Select a color", "Upload file"}
}

func (wrappedExampleT) ArgTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeFor[context.Context](),
		reflect.TypeFor[bool](),
		reflect.TypeFor[int](),
		reflect.TypeFor[float64](),
		reflect.TypeFor[Color](),
		reflect.TypeFor[fs.FileReader](),
	}
}

func (wrappedExampleT) ResultTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeFor[error](),
	}
}

func (wrappedExampleT) Call(ctx context.Context, args []any) (results []any, err error) {
	err = Example(ctx, args[0].(bool), args[1].(int), args[2].(float64), args[3].(Color), args[4].(fs.FileReader)) // wrapped call
	return results, err
}

func (f wrappedExampleT) CallWithStrings(ctx context.Context, strs ...string) (results []any, err error) {
	var a struct {
		aBool  bool
		anInt  int
		aFloat float64
		color  Color
		file   fs.FileReader
	}
	if 0 < len(strs) {
		err := function.ScanString(strs[0], &a.aBool)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "aBool", strs[0])
		}
	}
	if 1 < len(strs) {
		err := function.ScanString(strs[1], &a.anInt)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "anInt", strs[1])
		}
	}
	if 2 < len(strs) {
		err := function.ScanString(strs[2], &a.aFloat)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "aFloat", strs[2])
		}
	}
	if 3 < len(strs) {
		err := function.ScanString(strs[3], &a.color)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "color", strs[3])
		}
	}
	if 4 < len(strs) {
		err := function.ScanString(strs[4], &a.file)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "file", strs[4])
		}
	}
	err = Example(ctx, a.aBool, a.anInt, a.aFloat, a.color, a.file) // wrapped call
	return results, err
}

func (f wrappedExampleT) CallWithNamedStrings(ctx context.Context, strs map[string]string) (results []any, err error) {
	var a struct {
		aBool  bool
		anInt  int
		aFloat float64
		color  Color
		file   fs.FileReader
	}
	if str, ok := strs["aBool"]; ok {
		err := function.ScanString(str, &a.aBool)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "aBool", str)
		}
	}
	if str, ok := strs["anInt"]; ok {
		err := function.ScanString(str, &a.anInt)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "anInt", str)
		}
	}
	if str, ok := strs["aFloat"]; ok {
		err := function.ScanString(str, &a.aFloat)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "aFloat", str)
		}
	}
	if str, ok := strs["color"]; ok {
		err := function.ScanString(str, &a.color)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "color", str)
		}
	}
	if str, ok := strs["file"]; ok {
		err := function.ScanString(str, &a.file)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "file", str)
		}
	}
	err = Example(ctx, a.aBool, a.anInt, a.aFloat, a.color, a.file) // wrapped call
	return results, err
}

func (f wrappedExampleT) CallWithJSON(ctx context.Context, argsJSON []byte) (results []any, err error) {
	var a struct {
		ABool  bool
		AnInt  int
		AFloat float64
		Color  Color
		File   fs.File
	}
	err = json.Unmarshal(argsJSON, &a)
	if err != nil {
		return nil, function.NewErrParseArgsJSON(err, f, argsJSON)
	}
	err = Example(ctx, a.ABool, a.AnInt, a.AFloat, a.Color, a.File) // wrapped call
	return results, err
}
