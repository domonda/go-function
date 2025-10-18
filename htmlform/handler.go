package htmlform

import (
	"fmt"
	"html/template"
	"net/http"
	"reflect"

	"github.com/ungerik/go-fs"
	"github.com/ungerik/go-fs/multipartfs"
	"github.com/ungerik/go-httpx/httperr"

	"github.com/domonda/go-function"
	"github.com/domonda/go-function/httpfun"
	"github.com/domonda/go-types"
)

var typeOfFileReader = reflect.TypeFor[fs.FileReader]()

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
	Options  []Option
}

// Handler is an http.Handler that generates and processes HTML forms for a wrapped function.
// It automatically creates form fields based on function parameters and handles form submission.
//
// The handler responds to:
//   - GET requests: Displays the HTML form
//   - POST requests: Processes form submission and calls the wrapped function
//
// Example usage:
//
//	func CreateUser(ctx context.Context, name, email string, age int) error {
//	    // Implementation
//	    return nil
//	}
//
//	handler := htmlform.MustNewHandler(
//	    function.MustReflectWrapper("CreateUser", CreateUser),
//	    "Create User",
//	    httpfun.RespondStaticHTML("<h1>Success!</h1>"),
//	)
//	http.Handle("/create-user", handler)
type Handler struct {
	wrappedFunc     function.Wrapper
	argValidator    map[string]types.ValidatErr
	argRequired     map[string]bool
	argOptions      map[string][]Option
	argDefaultValue map[string]any
	argInputType    map[string]string
	form            struct {
		Title            string
		Fields           []formField
		SubmitButtonText string
	}
	template     *template.Template
	resultWriter httpfun.ResultsWriter
}

// NewHandler creates a new form handler for the given wrapped function.
//
// Parameters:
//   - wrappedFunc: The function to generate a form for
//   - title: The HTML page title and form heading
//   - resultWriter: Handler for the function's results after form submission
//
// Returns an error if the form template fails to parse.
//
// Example:
//
//	handler, err := htmlform.NewHandler(
//	    wrapper,
//	    "User Registration",
//	    httpfun.RespondJSON,
//	)
func NewHandler(wrappedFunc function.Wrapper, title string, resultWriter httpfun.ResultsWriter) (handler *Handler, err error) {
	handler = &Handler{
		wrappedFunc:     wrappedFunc,
		argValidator:    make(map[string]types.ValidatErr),
		argRequired:     make(map[string]bool),
		argOptions:      make(map[string][]Option),
		argDefaultValue: make(map[string]any),
		argInputType:    make(map[string]string),
		resultWriter:    resultWriter,
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
// The validator is called server-side when the form is submitted.
//
// Example:
//
//	handler.SetArgValidator("email", func(value any) error {
//	    email, _ := value.(string)
//	    if !strings.Contains(email, "@") {
//	        return errors.New("invalid email")
//	    }
//	    return nil
//	})
func (handler *Handler) SetArgValidator(arg string, validator types.ValidatErr) {
	handler.argValidator[arg] = validator
}

// SetArgRequired overrides the automatic required field detection.
// By default, non-pointer types and non-string types are required.
//
// Example:
//
//	handler.SetArgRequired("email", true)   // Make required
//	handler.SetArgRequired("age", false)    // Make optional
func (handler *Handler) SetArgRequired(arg string, required bool) {
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
func (handler *Handler) SetArgOptions(arg string, options []Option) {
	handler.argOptions[arg] = options
}

// SetArgDefaultValue sets the default value displayed in the form field.
//
// Example:
//
//	handler.SetArgDefaultValue("age", 18)
//	handler.SetArgDefaultValue("active", true)
//	handler.SetArgDefaultValue("country", "US")
func (handler *Handler) SetArgDefaultValue(arg string, value any) {
	handler.argDefaultValue[arg] = value
}

// SetArgInputType overrides the automatically detected HTML input type.
//
// Supported types include:
//   - "text", "email", "password", "url", "tel", "search"
//   - "number", "range"
//   - "date", "datetime-local", "time", "month", "week"
//   - "checkbox", "radio"
//   - "file"
//   - "textarea"
//   - "color"
//
// Example:
//
//	handler.SetArgInputType("email", "email")
//	handler.SetArgInputType("password", "password")
//	handler.SetArgInputType("bio", "textarea")
//	handler.SetArgInputType("age", "range")
func (handler *Handler) SetArgInputType(arg string, value string) {
	handler.argInputType[arg] = value
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
// Panics are recovered and converted to HTTP errors.
func (handler *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			httpfun.HandleError(httperr.AsError(r), response, request)
		}
	}()

	switch request.Method {
	case "GET":
		handler.get(response, request)
	case "POST":
		handler.post(response, request)
	default:
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// get handles GET requests by rendering the HTML form.
// It builds form fields based on the function's parameters and configured options.
func (handler *Handler) get(response http.ResponseWriter, _ *http.Request) {
	handler.form.Fields = nil
	for i, argName := range handler.wrappedFunc.ArgNames() {
		if i == 0 && handler.wrappedFunc.ContextArg() {
			continue
		}
		argDescription := handler.wrappedFunc.ArgDescriptions()[i]
		argType := handler.wrappedFunc.ArgTypes()[i]
		field := formField{
			Name:     argName,
			Label:    argDescription,
			Type:     "text",
			Required: requiredBasedOnType(argType),
		}
		if field.Label == "" {
			field.Label = argName
		}
		if defaultValue, ok := handler.argDefaultValue[argName]; ok {
			field.Value = fmt.Sprint(defaultValue)
		}
		if required, ok := handler.argRequired[argName]; ok {
			field.Required = required
		}
		options, isSelect := handler.argOptions[argName]
		switch {
		case isSelect:
			field.Type = "select"
			field.Options = options

		case argType.Implements(typeOfFileReader):
			field.Type = "file"

		// case argType == reflect.TypeOf(date.Date("")) || argType == reflect.TypeOf(date.NullableDate("")):
		// 	field.Type = "date"

		// case argType == reflect.TypeOf(time.Time{}):
		// 	field.Type = "datetime-local"

		default:
			switch argType.Kind() {
			case reflect.Bool:
				field.Type = "checkbox"
			case reflect.Float32, reflect.Float64:
				field.Type = "number"
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				field.Type = "number"
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				field.Type = "number"
			}
		}

		if inputType, ok := handler.argInputType[argName]; ok {
			field.Type = inputType
		}

		handler.form.Fields = append(handler.form.Fields, field)
	}

	err := handler.template.Execute(response, &handler.form)
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
	}
}

// post handles POST requests by parsing form data and calling the wrapped function.
// It supports multipart form data including file uploads up to 100MB.
// Form values are converted to function arguments and validated before execution.
func (handler *Handler) post(response http.ResponseWriter, request *http.Request) {
	formfs, err := multipartfs.FromRequestForm(request, 100*1024*1024)
	if err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}
	defer formfs.Close()

	argsMap := make(map[string]string)
	for key, vals := range formfs.Form.Value {
		argsMap[key] = vals[0]
	}
	for key := range formfs.Form.File {
		file, err := formfs.FormFile(key)
		if err != nil {
			// Should never happen
			panic(fmt.Errorf("can't get form file %s because %w", key, err))
		}
		argsMap[key] = string(file)
	}

	results, err := handler.wrappedFunc.CallWithNamedStrings(request.Context(), argsMap)

	err = handler.resultWriter.WriteResults(results, err, response, request)
	if err != nil {
		httpfun.HandleError(err, response, request)
	}
}

// requiredBasedOnType determines if a form field should be required based on its Go type.
// Returns false for: strings, pointer types, and types implementing IsNull().
// Returns true for all other types (int, bool, struct, etc.).
func requiredBasedOnType(t reflect.Type) bool {
	if t == reflect.TypeFor[string]() {
		return false
	}
	if t.Kind() == reflect.Ptr {
		return false
	}
	if t.Implements(reflect.TypeFor[interface{ IsNull() bool }]()) {
		return false
	}
	return true
}
