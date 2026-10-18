package main

//go:generate go run ../../cmd/gen-func-wrappers $GOFILE

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/ungerik/go-fs"
	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/htmlform"
	"github.com/domonda/go-function/httpfun"
	"github.com/domonda/golog/log"
)

func main() {
	httperr.DebugShowInternalErrorsInResponse = true

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
	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	err = server.ListenAndServe()
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
	// Without an uploaded file the argument is nil
	fileName := "<no file>"
	if file != nil {
		fileName = file.Name()
	}
	log.InfoCtx(ctx, "Example").
		Bool("aBool", aBool).
		Int("anInt", anInt).
		Float("aFloat", aFloat).
		Any("color", color).
		Str("file", fileName).
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
	return "main.Example(ctx context.Context, aBool bool, anInt int, aFloat float64, color main.Color, file fs.FileReader) error"
}

func (wrappedExampleT) Name() string {
	return "main.Example"
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

func (f wrappedExampleT) Call(ctx context.Context, args []any) (results []any, err error) {
	if len(args) > 5 {
		return nil, fmt.Errorf("%d arguments passed, but function %s takes 5", len(args), f)
	}
	var a struct {
		aBool  bool
		anInt  int
		aFloat float64
		color  Color
		file   fs.FileReader
	}
	if 0 < len(args) && args[0] != nil {
		v, ok := args[0].(bool)
		if !ok {
			return nil, fmt.Errorf("can't use %T as type bool for argument 'aBool' of function %s", args[0], f)
		}
		a.aBool = v
	}
	if 1 < len(args) && args[1] != nil {
		v, ok := args[1].(int)
		if !ok {
			return nil, fmt.Errorf("can't use %T as type int for argument 'anInt' of function %s", args[1], f)
		}
		a.anInt = v
	}
	if 2 < len(args) && args[2] != nil {
		v, ok := args[2].(float64)
		if !ok {
			return nil, fmt.Errorf("can't use %T as type float64 for argument 'aFloat' of function %s", args[2], f)
		}
		a.aFloat = v
	}
	if 3 < len(args) && args[3] != nil {
		v, ok := args[3].(Color)
		if !ok && reflect.TypeOf(args[3]).AssignableTo(reflect.TypeFor[Color]()) {
			v, ok = reflect.TypeAssert[Color](reflect.ValueOf(args[3]).Convert(reflect.TypeFor[Color]()))
		}
		if !ok {
			return nil, fmt.Errorf("can't use %T as type Color for argument 'color' of function %s", args[3], f)
		}
		a.color = v
	}
	if 4 < len(args) && args[4] != nil {
		v, ok := args[4].(fs.FileReader)
		if !ok && reflect.TypeOf(args[4]).AssignableTo(reflect.TypeFor[fs.FileReader]()) {
			v, ok = reflect.TypeAssert[fs.FileReader](reflect.ValueOf(args[4]).Convert(reflect.TypeFor[fs.FileReader]()))
		}
		if !ok {
			return nil, fmt.Errorf("can't use %T as type fs.FileReader for argument 'file' of function %s", args[4], f)
		}
		a.file = v
	}
	err = Example(ctx, a.aBool, a.anInt, a.aFloat, a.color, a.file) // wrapped call
	return results, err
}

func (f wrappedExampleT) CallWithStrings(ctx context.Context, strs ...string) (results []any, err error) {
	if len(strs) > 5 {
		return nil, fmt.Errorf("%d arguments passed, but function %s takes 5", len(strs), f)
	}
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
	var args map[string]json.RawMessage
	err = json.Unmarshal(argsJSON, &args)
	if err != nil {
		return nil, function.NewErrParseArgsJSON(err, f, argsJSON)
	}
	var a struct {
		aBool  bool
		anInt  int
		aFloat float64
		color  Color
		file   fs.FileReader
	}
	if raw, ok := args["aBool"]; ok {
		err = json.Unmarshal(raw, &a.aBool)
		if err != nil {
			return nil, function.NewErrParseArgJSON(err, f, "aBool")
		}
	}
	if raw, ok := args["anInt"]; ok {
		err = json.Unmarshal(raw, &a.anInt)
		if err != nil {
			return nil, function.NewErrParseArgJSON(err, f, "anInt")
		}
	}
	if raw, ok := args["aFloat"]; ok {
		err = json.Unmarshal(raw, &a.aFloat)
		if err != nil {
			return nil, function.NewErrParseArgJSON(err, f, "aFloat")
		}
	}
	if raw, ok := args["color"]; ok {
		err = json.Unmarshal(raw, &a.color)
		if err != nil {
			return nil, function.NewErrParseArgJSON(err, f, "color")
		}
	}
	if raw, ok := args["file"]; ok {
		err = json.Unmarshal(raw, &a.file)
		if err != nil {
			return nil, function.NewErrParseArgJSON(err, f, "file")
		}
	}
	err = Example(ctx, a.aBool, a.anInt, a.aFloat, a.color, a.file) // wrapped call
	return results, err
}
