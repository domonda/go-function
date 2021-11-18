package main

//go:generate gen-func-wrappers

import (
	"context"
	"net/http"
	"reflect"

	"github.com/domonda/go-function"
	"github.com/domonda/golog/log"

	"github.com/ungerik/go-fs"
	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function/htmlform"
)

func main() {
	httperr.DebugShowInternalErrorsInResponse = true

	function.StringScanners = function.StringScanners.
		WithTypeScanner(
			reflect.TypeOf((*fs.FileReader)(nil)).Elem(),
			function.StringScannerFunc(func(sourceStr string, destPtr interface{}) error {
				*destPtr.(*fs.FileReader) = fs.File(sourceStr)
				return nil
			}),
		)

	handler, err := htmlform.NewHandler(wrappedExample, "Example Form", function.RespondStaticHTML("<h1>Success!</h1>"))
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
	if err != nil {
		log.FatalAndPanic(err)
	}
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
func Example(aBool bool, anInt int, aFloat float64, color Color, file fs.FileReader /*, aDate date.Date, aTime time.Time*/) error {
	log.Info("Example").
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
	return "Example(aBool bool, anInt int, aFloat float64, color Color, file fs.FileReader) error"
}

func (wrappedExampleT) Name() string {
	return "Example"
}

func (wrappedExampleT) NumArgs() int      { return 5 }
func (wrappedExampleT) ContextArg() bool  { return false }
func (wrappedExampleT) NumResults() int   { return 1 }
func (wrappedExampleT) ErrorResult() bool { return true }

func (wrappedExampleT) ArgNames() []string {
	return []string{"aBool", "anInt", "aFloat", "color", "file"}
}

func (wrappedExampleT) ArgDescriptions() []string {
	return []string{"A bool", "An integer", "A float", "Select a color", "Upload file"}
}

func (wrappedExampleT) ArgTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf((*bool)(nil)).Elem(),
		reflect.TypeOf((*int)(nil)).Elem(),
		reflect.TypeOf((*float64)(nil)).Elem(),
		reflect.TypeOf((*Color)(nil)).Elem(),
		reflect.TypeOf((*fs.FileReader)(nil)).Elem(),
	}
}

func (wrappedExampleT) ResultTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf((*error)(nil)).Elem(),
	}
}

func (f wrappedExampleT) Call(_ context.Context, args []interface{}) (results []interface{}, err error) {
	err = Example(args[0].(bool), args[1].(int), args[2].(float64), args[3].(Color), args[4].(fs.FileReader)) // call
	return results, err
}

func (f wrappedExampleT) CallWithStrings(_ context.Context, strs ...string) (results []interface{}, err error) {
	var aBool bool
	if len(strs) > 0 {
		err = function.ScanString(strs[0], &aBool)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "aBool")
		}
	}
	var anInt int
	if len(strs) > 1 {
		err = function.ScanString(strs[1], &anInt)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "anInt")
		}
	}
	var aFloat float64
	if len(strs) > 2 {
		err = function.ScanString(strs[2], &aFloat)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "aFloat")
		}
	}
	var color Color
	if len(strs) > 3 {
		err = function.ScanString(strs[3], &color)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "color")
		}
	}
	var file fs.FileReader
	if len(strs) > 4 {
		err = function.ScanString(strs[4], &file)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "file")
		}
	}
	err = Example(aBool, anInt, aFloat, color, file) // call
	return results, err
}

func (f wrappedExampleT) CallWithNamedStrings(_ context.Context, strs map[string]string) (results []interface{}, err error) {
	var aBool bool
	if str, ok := strs["aBool"]; ok {
		err = function.ScanString(str, &aBool)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "aBool")
		}
	}
	var anInt int
	if str, ok := strs["anInt"]; ok {
		err = function.ScanString(str, &anInt)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "anInt")
		}
	}
	var aFloat float64
	if str, ok := strs["aFloat"]; ok {
		err = function.ScanString(str, &aFloat)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "aFloat")
		}
	}
	var color Color
	if str, ok := strs["color"]; ok {
		err = function.ScanString(str, &color)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "color")
		}
	}
	var file fs.FileReader
	if str, ok := strs["file"]; ok {
		err = function.ScanString(str, &file)
		if err != nil {
			return nil, function.NewErrParseArgString(err, f, "file")
		}
	}
	err = Example(aBool, anInt, aFloat, color, file) // call
	return results, err
}
