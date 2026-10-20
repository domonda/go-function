package htmlform

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/ungerik/go-fs"
	"github.com/ungerik/go-fs/multipartfs"
	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/httpfun"
)

var (
	typeOfFileReader      = reflect.TypeFor[fs.FileReader]()
	typeOfFile            = reflect.TypeFor[fs.File]()
	typeOfAny             = reflect.TypeFor[any]()
	typeOfBytes           = reflect.TypeFor[[]byte]()
	typeOfDuration        = reflect.TypeFor[time.Duration]()
	typeOfTextUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// ArgValidator validates the value of a function argument
// parsed from a submitted form before the function is called.
//
// The ctx is the context of the HTTP request.
// The value has the type of the function argument
// and is the zero value of that type if the argument
// was not submitted with the form.
//
// Returning an error rejects the form submission
// with 400 Bad Request, and the error text
// is shown to the client in the response.
type ArgValidator func(ctx context.Context, value any) error

// Option represents a choice in a select dropdown field.
// The Label is displayed to the user, while Value is submitted with the form.
type Option struct {
	Label string
	Value any
}

// formField represents an HTML form input field with its properties.
// It's used internally to generate form HTML.
type formField struct {
	Name     string
	Label    string
	Type     string
	Value    string
	Required bool
	Multiple bool
	// Step is the step attribute of number inputs,
	// "any" for floats, because the default step of 1 only allows integers
	Step    string
	Options []Option
}

// Handler is an http.Handler that generates and processes HTML forms for a wrapped function.
// It automatically creates form fields based on function parameters and handles form submission.
//
// The handler responds to:
//   - GET requests: Displays the HTML form
//   - POST requests: Processes form submission and calls the wrapped function
//
// POST requests are protected against cross-site request forgery,
// see SetCrossOriginProtection, and limited in size, see SetMaxRequestBodySize.
//
// Example usage:
//
//	func CreateUser(ctx context.Context, name, email string, age int) error {
//	    // Implementation
//	    return nil
//	}
//
//	handler := htmlform.MustNewHandler(
//	    function.MustReflectWrapper(CreateUser, "ctx", "name", "email", "age"),
//	    "Create User",
//	    httpfun.RespondStaticHTML("<h1>Success!</h1>"),
//	)
//	http.Handle("/create-user", handler)
type Handler struct {
	wrappedFunc     function.Wrapper
	argValidator    map[string]ArgValidator
	argRequired     map[string]bool
	argOptions      map[string][]Option
	argDefaultValue map[string]any
	argInputType    map[string]string
	form            struct {
		Title            string
		Fields           []formField
		SubmitButtonText string
	}
	template              *template.Template
	resultWriter          httpfun.ResultsWriter
	maxRequestBodySize    int64
	crossOriginProtection *http.CrossOriginProtection
}

// NewHandler creates a new form handler for the given wrapped function.
//
// Parameters:
//   - wrappedFunc: The function to generate a form for
//   - title: The HTML page title and form heading
//   - resultWriter: Handler for the function's results after form submission
//
// A nil resultWriter writes nothing for a successful submission
// like httpfun.RespondNothing, errors are answered by httpfun.HandleError.
//
// Returns an error if the form template fails to parse,
// or if an argument can't be passed with a form: a file argument
// of a type that an uploaded fs.File can't be assigned to, like fs.MemFile
// or a pointer to a slice of files, or an argument with a file type
// in a struct field or map, which an upload can't fill
// and a text value would set to the path of a file on the server.
//
// Example:
//
//	handler, err := htmlform.NewHandler(
//	    wrapper,
//	    "User Registration",
//	    httpfun.RespondJSON,
//	)
func NewHandler(wrappedFunc function.Wrapper, title string, resultWriter httpfun.ResultsWriter) (handler *Handler, err error) {
	argNames := wrappedFunc.ArgNames()
	for i, argType := range wrappedFunc.ArgTypes() {
		if i == 0 && wrappedFunc.ContextArg() || argNames[i] == "_" {
			continue
		}
		switch {
		case isFileArgType(argType):
			if !isUploadableFileArgType(argType) {
				return nil, fmt.Errorf("htmlform: can't pass uploaded files as argument %q of type %s of function %s, use a type that fs.File can be assigned to, or a pointer, slice, or array of it", argNames[i], argType, wrappedFunc)
			}
		default:
			if fileType := nestedFileType(argType); fileType != nil {
				return nil, fmt.Errorf("htmlform: argument %q of type %s of function %s contains the file type %s, which can't be uploaded, a text value would be the path of a file on the server", argNames[i], argType, wrappedFunc, fileType)
			}
		}
	}
	if resultWriter == nil {
		resultWriter = httpfun.RespondNothing
	}
	handler = &Handler{
		wrappedFunc:     wrappedFunc,
		argValidator:    make(map[string]ArgValidator),
		argRequired:     make(map[string]bool),
		argOptions:      make(map[string][]Option),
		argDefaultValue: make(map[string]any),
		argInputType:    make(map[string]string),
		resultWriter:    resultWriter,
		// 100 MiB
		maxRequestBodySize:    100 << 20,
		crossOriginProtection: http.NewCrossOriginProtection(),
	}
	handler.form.Title = title
	handler.form.SubmitButtonText = "Submit"
	handler.template, err = template.New("form").Parse(FormTemplate)
	if err != nil {
		return nil, err
	}
	return handler, nil
}

// MustNewHandler is like NewHandler but panics on error.
// Use this in initialization code where form creation failures should be fatal.
func MustNewHandler(fun function.Wrapper, title string, successHandler httpfun.ResultsWriter) (handler *Handler) {
	handler, err := NewHandler(fun, title, successHandler)
	if err != nil {
		panic(err)
	}
	return handler
}

// SetArgValidator registers a custom validator for a specific function argument.
// The validator is called server-side when the form is submitted
// with the context of the request and the argument value
// parsed from the form, which is the value the function will be called with.
//
// If a validator returns an error, then the wrapped function is not called
// and a 400 Bad Request error naming the argument is handled
// like an error returned by the function: it is passed to the
// ResultsWriter and then to httpfun.HandleError if the writer returns it.
// The text of the validator error is shown to the client in that response,
// so it must not contain internal details.
//
// Example:
//
//	handler.SetArgValidator("email", func(ctx context.Context, value any) error {
//	    email, _ := value.(string)
//	    if !strings.Contains(email, "@") {
//	        return errors.New("invalid email")
//	    }
//	    return nil
//	})
//
// A nil validator removes the validator of the argument.
//
// Panics if arg is not the name of an argument of the wrapped function,
// so that a typo or renamed argument doesn't silently remove the setting.
func (handler *Handler) SetArgValidator(arg string, validator ArgValidator) {
	handler.mustBeArg("SetArgValidator", arg)
	if validator == nil {
		delete(handler.argValidator, arg)
		return
	}
	handler.argValidator[arg] = validator
}

// SetArgRequired overrides the automatic required field detection.
// By default, arguments are required unless their type is string,
// a pointer, or implements IsNull() bool.
// Required fields are enforced by browsers with the required attribute
// of the form field, bool checkboxes are never marked as required.
//
// Example:
//
//	handler.SetArgRequired("email", true)   // Make required
//	handler.SetArgRequired("age", false)    // Make optional
//
// Panics if arg is not the name of an argument of the wrapped function,
// so that a typo or renamed argument doesn't silently remove the setting.
func (handler *Handler) SetArgRequired(arg string, required bool) {
	handler.mustBeArg("SetArgRequired", arg)
	handler.argRequired[arg] = required
}

// SetArgOptions configures a field to display as a dropdown select with the given options.
//
// Example:
//
//	handler.SetArgOptions("country", []htmlform.Option{
//	    {Label: "United States", Value: "US"},
//	    {Label: "Canada", Value: "CA"},
//	    {Label: "Mexico", Value: "MX"},
//	})
//
// Panics if arg is not the name of an argument of the wrapped function,
// so that a typo or renamed argument doesn't silently remove the setting.
func (handler *Handler) SetArgOptions(arg string, options []Option) {
	handler.mustBeArg("SetArgOptions", arg)
	handler.argOptions[arg] = options
}

// SetArgDefaultValue sets the default value displayed in the form field.
//
// Example:
//
//	handler.SetArgDefaultValue("age", 18)
//	handler.SetArgDefaultValue("active", true)
//	handler.SetArgDefaultValue("country", "US")
//
// Panics if arg is not the name of an argument of the wrapped function,
// so that a typo or renamed argument doesn't silently remove the setting.
func (handler *Handler) SetArgDefaultValue(arg string, value any) {
	handler.mustBeArg("SetArgDefaultValue", arg)
	handler.argDefaultValue[arg] = value
}

// SetArgInputType overrides the automatically detected HTML input type.
//
// Supported types include:
//   - "text", "email", "password", "url", "tel", "search"
//   - "number", "range"
//   - "date", "datetime-local", "time", "month", "week"
//   - "checkbox"
//   - "radio" with the options set by SetArgOptions
//   - "file", only for file arguments
//   - "textarea"
//   - "color"
//
// Example:
//
//	handler.SetArgInputType("email", "email")
//	handler.SetArgInputType("password", "password")
//	handler.SetArgInputType("bio", "textarea")
//	handler.SetArgInputType("age", "range")
//
// Panics if arg is not the name of an argument of the wrapped function,
// so that a typo or renamed argument doesn't silently remove the setting,
// and for "file" if the argument doesn't take uploaded files,
// because the upload would be ignored.
func (handler *Handler) SetArgInputType(arg string, value string) {
	handler.mustBeArg("SetArgInputType", arg)
	if value == "file" {
		if argType := handler.argType(arg); !isFileArgType(argType) {
			panic(fmt.Sprintf("htmlform.Handler.SetArgInputType: argument %q of type %s of function %s doesn't take uploaded files", arg, argType, handler.wrappedFunc.Name()))
		}
	}
	handler.argInputType[arg] = value
}

// SetMaxRequestBodySize sets the maximum size in bytes of a POST request body
// including uploaded files. Larger requests are answered
// with 413 Request Entity Too Large without calling the function.
// Default: 100 MiB.
//
// A size <= 0 does not disable the limit but rejects every form submission,
// like http.MaxBytesReader does. Pass math.MaxInt64 for no practical limit.
//
// Uploaded files larger than httpfun.MaxMultipartFormMemory
// are buffered in temporary files instead of memory.
func (handler *Handler) SetMaxRequestBodySize(size int64) {
	handler.maxRequestBodySize = size
}

// SetCrossOriginProtection sets the protection of form submissions
// against cross-site request forgery (CSRF).
// POST requests rejected by protection.Check are answered
// with 403 Forbidden without calling the function.
//
// By default every Handler uses its own http.NewCrossOriginProtection(),
// which rejects POST requests that browsers send from other origins
// as indicated by the Sec-Fetch-Site or Origin request headers,
// and accepts requests without these headers like those of non-browser clients.
// Use http.CrossOriginProtection.AddTrustedOrigin of a custom protection
// to accept form submissions from other origins.
//
// Passing nil disables the protection, for example for a handler
// that is only called by non-browser clients and protected otherwise,
// like with credentials that browsers don't send automatically.
func (handler *Handler) SetCrossOriginProtection(protection *http.CrossOriginProtection) {
	handler.crossOriginProtection = protection
}

// SetSubmitButtonText customizes the text displayed on the form's submit button.
// Default is "Submit".
//
// Example:
//
//	handler.SetSubmitButtonText("Create Account")
//	handler.SetSubmitButtonText("Save Changes")
func (handler *Handler) SetSubmitButtonText(text string) {
	handler.form.SubmitButtonText = text
}

// ServeHTTP implements http.Handler.
// It handles both GET requests (display form) and POST requests (process form submission).
// Panics are recovered and converted to HTTP errors,
// except http.ErrAbortHandler, with which net/http aborts the response.
func (handler *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			if r == http.ErrAbortHandler {
				panic(r)
			}
			httpfun.HandleError(httperr.AsError(r), response, request)
		}
	}()

	switch request.Method {
	case http.MethodGet, http.MethodHead:
		handler.get(response, request)
	case http.MethodPost:
		handler.post(response, request)
	default:
		response.Header().Set("Allow", "GET, HEAD, POST")
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// get handles GET requests by rendering the HTML form.
// It builds form fields based on the function's parameters and configured options.
func (handler *Handler) get(response http.ResponseWriter, request *http.Request) {
	// The form data is built per request because the handler
	// is shared by concurrent requests and must not be modified
	form := handler.form
	form.Fields = nil
	argNames := handler.wrappedFunc.ArgNames()
	argDescriptions := handler.wrappedFunc.ArgDescriptions()
	argTypes := handler.wrappedFunc.ArgTypes()
	for i, argName := range argNames {
		if i == 0 && handler.wrappedFunc.ContextArg() || argName == "_" {
			// Arguments named _ are not used by the function
			continue
		}
		argType := argTypes[i]
		field := formField{
			Name:     argName,
			Label:    argDescriptions[i],
			Type:     "text",
			Required: requiredBasedOnType(argType),
		}
		if field.Label == "" {
			field.Label = argName
		}
		if handler.wrappedFunc.Variadic() && i == len(argNames)-1 {
			// Like in Go, a variadic argument can have no values
			field.Required = false
		}
		if required, ok := handler.argRequired[argName]; ok {
			field.Required = required
		}
		options, isSelect := handler.argOptions[argName]
		switch {
		case isSelect:
			field.Type = "select"
			field.Options = options

		case isFileArgType(argType):
			field.Type = "file"
			field.Multiple = argType.Kind() == reflect.Slice || argType.Kind() == reflect.Array && argType.Len() > 1

		case parsesOwnStrings(argType):
			// Number inputs can't submit strings like "5s" of a time.Duration

		default:
			switch argType.Kind() {
			case reflect.Bool:
				field.Type = "checkbox"
			case reflect.Float32, reflect.Float64:
				field.Type = "number"
				field.Step = "any"
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				field.Type = "number"
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				field.Type = "number"
			}
		}

		if inputType, ok := handler.argInputType[argName]; ok {
			field.Type = inputType
		}

		if defaultValue, ok := handler.argDefaultValue[argName]; ok {
			if isSelect {
				// Formatted like the option values by the template
				field.Value = fmt.Sprint(defaultValue)
			} else {
				field.Value, field.Step = formValue(defaultValue, field.Type, field.Step)
			}
		}

		form.Fields = append(form.Fields, field)
	}

	// Executed into a buffer, so that a template error
	// is answered without a partly written form
	var page bytes.Buffer
	err := handler.template.Execute(&page, &form)
	if err != nil {
		httpfun.HandleError(err, response, request)
		return
	}
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = response.Write(page.Bytes())
}

// formValue returns value formatted as value of a form field
// of inputType, so that submitting the form passes value again,
// and the step of the field, which is changed from step for
// a datetime-local field to accept the seconds of value.
func formValue(value any, inputType, step string) (string, string) {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "", step
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return "", step
	}
	value = v.Interface()
	if t, ok := value.(time.Time); ok {
		// The formats of the browser inputs, which the default TimeFormats parse
		t = t.In(function.TimeLocation)
		switch inputType {
		case "date":
			return t.Format(time.DateOnly), step
		case "datetime-local":
			if t.Second() == 0 && t.Nanosecond() == 0 {
				return t.Format(time.DateOnly + "T15:04"), step
			}
			// The default step of 60 seconds would reject the seconds
			return t.Format(time.DateOnly + "T15:04:05"), "1"
		}
	}
	if marshaler, ok := value.(encoding.TextMarshaler); ok {
		// The inverse of UnmarshalText, which ScanString uses
		if text, err := marshaler.MarshalText(); err == nil {
			return string(text), step
		}
	}
	if v.Type() == typeOfBytes {
		return string(v.Bytes()), step
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		// ScanString scans slice literals and JSON
		if j, err := json.Marshal(value); err == nil {
			return string(j), step
		}
	}
	return fmt.Sprint(value), step
}

// parsesOwnStrings returns true if ScanString parses strings
// for argType with its encoding.TextUnmarshaler method like slog.Level,
// or as time.Duration like "5s", instead of as number of its kind.
func parsesOwnStrings(argType reflect.Type) bool {
	return argType == typeOfDuration || reflect.PointerTo(argType).Implements(typeOfTextUnmarshaler)
}

// post handles POST requests by parsing form data and calling the wrapped function.
// It rejects cross-origin requests and request bodies larger than maxRequestBodySize,
// supports multipart form data including file uploads,
// and converts form values to validated function arguments.
func (handler *Handler) post(response http.ResponseWriter, request *http.Request) {
	if handler.crossOriginProtection != nil {
		err := handler.crossOriginProtection.Check(request)
		if err != nil {
			http.Error(response, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
	}

	request.Body = http.MaxBytesReader(response, request.Body, handler.maxRequestBodySize)
	// Parsing the form can buffer uploads in temporary files and still fail,
	// for example because of an invalid URL query, and net/http only removes
	// the temporary files of the original request, not of a request
	// that a middleware cloned, so remove them in any case
	defer func() {
		if request.MultipartForm != nil {
			_ = request.MultipartForm.RemoveAll()
		}
	}()
	formfs, err := multipartfs.FromRequestForm(request, httpfun.MaxMultipartFormMemory)
	if err != nil {
		var (
			maxBytesErr *http.MaxBytesError
			pathErr     *os.PathError
		)
		switch {
		case errors.As(err, &maxBytesErr):
			http.Error(response, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		case errors.As(err, &pathErr):
			// Buffering an upload in a temporary file failed,
			// which is an error of the server, not of the request.
			// err is not wrapped because httperr answers a wrapped
			// fs.ErrNotExist, like of a missing temp directory, with 404 Not Found
			httpfun.HandleError(fmt.Errorf("can't buffer form upload in temporary file: %s", err), response, request)
		default:
			// The error can contain details of the server like its network address
			http.Error(response, "invalid multipart form", http.StatusBadRequest)
		}
		return
	}
	defer formfs.Close()

	var results []any
	args, err := handler.parseArgs(formfs)
	if err == nil {
		err = handler.validateArgs(request.Context(), args)
	}
	if err == nil {
		results, err = handler.wrappedFunc.Call(request.Context(), args)
	}
	// Like errors of the function, errors of the request go to the ResultsWriter
	// so it can render them, for example as part of the form.
	// Errors of the request are answered with their 4xx status code
	// by httpfun.DefaultHandleError if the ResultsWriter returns them.
	err = handler.resultWriter.WriteResults(results, err, response, request)
	if err != nil {
		httpfun.HandleError(err, response, request)
	}
}

// parseArgs returns the arguments for function.CallWrapper.Call
// of the wrapped function, without the context argument,
// parsed from the submitted form.
// Arguments missing in the form are returned as zero values.
//
// File arguments, which have a type implementing fs.FileReader
// or are pointers, slices or arrays of such a type,
// are only taken from uploaded files, never from text values,
// because a text value would be used as path of a file on the server.
// A non-empty text value for a file argument is rejected
// with 400 Bad Request naming the argument.
// Browsers send an empty text value for a file input without a selected file.
//
// An argument value that can't be parsed is rejected with 400 Bad Request
// naming the argument but not showing its value.
func (handler *Handler) parseArgs(formfs *multipartfs.MultipartFileSystem) ([]any, error) {
	argNames := handler.wrappedFunc.ArgNames()
	argTypes := handler.wrappedFunc.ArgTypes()
	offs := 0
	if handler.wrappedFunc.ContextArg() {
		offs = 1
	}
	args := make([]any, len(argNames)-offs)
	for i := range args {
		argName, argType := argNames[offs+i], argTypes[offs+i]
		values := formfs.Form.Value[argName]
		switch {
		case argName == "_":
			// Not in the form and not used by the function
			args[i] = reflect.Zero(argType).Interface()

		case isFileArgType(argType):
			if slices.ContainsFunc(values, func(value string) bool { return value != "" }) {
				return nil, newRequestError(nil, http.StatusBadRequest, fmt.Sprintf("argument %q must be an uploaded file", argName))
			}
			arg, err := handler.fileArg(formfs, argName, argType)
			if err != nil {
				return nil, err
			}
			args[i] = arg

		case len(values) == 0:
			args[i] = reflect.Zero(argType).Interface()

		case argType == typeOfAny:
			// function.ReflectWrapper passes strings unparsed to arguments of type any
			args[i] = values[0]

		default:
			value := values[0]
			if len(values) > 1 && isSliceArgType(argType) {
				// Like httpfun.RequestMultipartFormArgs, multiple values
				// of a field, like of a select with the multiple attribute,
				// are the elements of a slice or array argument
				value = sliceLiteral(values)
			}
			destPtr := reflect.New(argType)
			err := function.ScanString(value, destPtr.Interface())
			if err != nil {
				err = function.NewErrParseArgString(err, handler.wrappedFunc, argName, value)
				return nil, newRequestError(err, http.StatusBadRequest, fmt.Sprintf("invalid value for argument %q", argName))
			}
			args[i] = destPtr.Elem().Interface()
		}
	}
	return args, nil
}

// fileArg returns the uploaded files of the form field argName
// as value of argType, which is a type implementing fs.FileReader
// or a pointer, slice or array of such a type.
// Without uploaded files the zero value of argType is returned.
func (handler *Handler) fileArg(formfs *multipartfs.MultipartFileSystem, argName string, argType reflect.Type) (any, error) {
	files := formfs.FormFiles(argName)
	if len(files) == 0 {
		return reflect.Zero(argType).Interface(), nil
	}
	notAssignable := func() error {
		return fmt.Errorf("can't pass uploaded file as argument '%s' of type %s of function %s", argName, argType, handler.wrappedFunc)
	}
	switch {
	case typeOfFile.AssignableTo(argType):
		return files[0], nil

	case argType.Kind() == reflect.Pointer && typeOfFile.AssignableTo(argType.Elem()):
		ptr := reflect.New(argType.Elem())
		ptr.Elem().Set(reflect.ValueOf(files[0]))
		return ptr.Interface(), nil

	case argType.Kind() == reflect.Slice && typeOfFile.AssignableTo(argType.Elem()):
		slice := reflect.MakeSlice(argType, len(files), len(files))
		for i, file := range files {
			slice.Index(i).Set(reflect.ValueOf(file))
		}
		return slice.Interface(), nil

	case argType.Kind() == reflect.Array && typeOfFile.AssignableTo(argType.Elem()):
		if len(files) > argType.Len() {
			return nil, newRequestError(nil, http.StatusBadRequest, fmt.Sprintf("argument %q takes at most %d files", argName, argType.Len()))
		}
		array := reflect.New(argType).Elem()
		for i, file := range files {
			array.Index(i).Set(reflect.ValueOf(file))
		}
		return array.Interface(), nil
	}
	return nil, notAssignable()
}

// isFileArgType returns true if values of argType are uploaded files:
// types implementing fs.FileReader, and pointers, slices or arrays of them
// at any depth like *[]fs.File, so that no text value becomes a file path.
// fileArg only fills the supported types and returns an error for others.
func isFileArgType(argType reflect.Type) bool {
	// Recursive types like type T []T would loop forever
	visited := make(map[reflect.Type]bool)
	for !visited[argType] {
		visited[argType] = true
		if argType.Implements(typeOfFileReader) {
			return true
		}
		switch argType.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			argType = argType.Elem()
		default:
			return false
		}
	}
	return false
}

// isUploadableFileArgType returns true if fileArg can pass uploaded files
// as argType: a type that fs.File can be assigned to,
// or a pointer, slice, or array of such a type.
func isUploadableFileArgType(argType reflect.Type) bool {
	if typeOfFile.AssignableTo(argType) {
		return true
	}
	switch argType.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return typeOfFile.AssignableTo(argType.Elem())
	}
	return false
}

// nestedFileType returns a type implementing fs.FileReader
// in the exported struct fields, map keys or values,
// or elements of argType, or nil if there is none.
func nestedFileType(argType reflect.Type) reflect.Type {
	visited := make(map[reflect.Type]bool)
	var find func(t reflect.Type) reflect.Type
	find = func(t reflect.Type) reflect.Type {
		if visited[t] {
			return nil
		}
		visited[t] = true
		if t.Implements(typeOfFileReader) {
			return t
		}
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			return find(t.Elem())
		case reflect.Map:
			if fileType := find(t.Key()); fileType != nil {
				return fileType
			}
			return find(t.Elem())
		case reflect.Struct:
			for field := range t.Fields() {
				if !field.IsExported() && !field.Anonymous {
					// Not set from JSON
					continue
				}
				if fileType := find(field.Type); fileType != nil {
					return fileType
				}
			}
		}
		return nil
	}
	return find(argType)
}

// isSliceArgType returns true if argType, or the type it points to,
// is a slice or array type other than []byte, which is scanned from one string.
func isSliceArgType(argType reflect.Type) bool {
	for argType.Kind() == reflect.Pointer {
		argType = argType.Elem()
	}
	return argType != typeOfBytes && (argType.Kind() == reflect.Slice || argType.Kind() == reflect.Array)
}

// sliceLiteral returns values as slice literal of JSON strings like ["1","2"],
// like httpfun.RequestQueryArgs passes multiple values,
// which function.ScanString scans as the elements of slices and arrays.
func sliceLiteral(values []string) string {
	var b strings.Builder
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(values) // Can't fail for strings
	return strings.TrimSuffix(b.String(), "\n")
}

// argType returns the type of the argument arg of the wrapped function.
func (handler *Handler) argType(arg string) reflect.Type {
	return handler.wrappedFunc.ArgTypes()[slices.Index(handler.wrappedFunc.ArgNames(), arg)]
}

// mustBeArg panics if arg is not the name of an argument
// of the wrapped function, the context argument excluded.
func (handler *Handler) mustBeArg(method, arg string) {
	argNames := handler.wrappedFunc.ArgNames()
	if handler.wrappedFunc.ContextArg() {
		argNames = argNames[1:]
	}
	if arg == "_" || !slices.Contains(argNames, arg) {
		panic(fmt.Sprintf("htmlform.Handler.%s: function %s has no argument %q", method, handler.wrappedFunc.Name(), arg))
	}
}

// requestError is an error caused by the request, not by the server.
// Its Error method returns a message without details of the server,
// because the ResultsWriter can show it to the client,
// the original error is available with errors.Unwrap for logging.
// It responds with its status code and that message.
type requestError struct {
	err        error
	statusCode int
	message    string
}

func newRequestError(err error, statusCode int, message string) *requestError {
	if err == nil {
		err = errors.New(message)
	}
	return &requestError{err: err, statusCode: statusCode, message: message}
}

func (e *requestError) Error() string { return e.message }

func (e *requestError) Unwrap() error { return e.err }

// ServeHTTP responds with the status code and message of the error,
// httpfun.DefaultHandleError uses it for errors wrapping a requestError.
func (e *requestError) ServeHTTP(response http.ResponseWriter, _ *http.Request) {
	http.Error(response, e.message, e.statusCode)
}

// validateArgs calls the validators set with SetArgValidator
// with ctx and the args returned by parseArgs.
//
// Returns a 400 Bad Request error naming the argument
// if its validator returns an error.
func (handler *Handler) validateArgs(ctx context.Context, args []any) error {
	argNames := handler.wrappedFunc.ArgNames()
	if handler.wrappedFunc.ContextArg() {
		argNames = argNames[1:]
	}
	for i, value := range args {
		validator, ok := handler.argValidator[argNames[i]]
		if !ok {
			continue
		}
		err := validator(ctx, value)
		if err != nil {
			// The text of the validator error is shown to the client like documented
			return newRequestError(err, http.StatusBadRequest, fmt.Sprintf("invalid argument %q: %s", argNames[i], err))
		}
	}
	return nil
}

// requiredBasedOnType determines if a form field should be required based on its Go type.
// Returns false for: strings, pointer types, and types implementing IsNull().
// Returns true for all other types (int, bool, struct, etc.).
func requiredBasedOnType(t reflect.Type) bool {
	if t == reflect.TypeFor[string]() {
		return false
	}
	if t.Kind() == reflect.Pointer {
		return false
	}
	if t.Implements(reflect.TypeFor[interface{ IsNull() bool }]()) {
		return false
	}
	return true
}
