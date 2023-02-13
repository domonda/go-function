package htmlform

import (
	"fmt"
	"html/template"
	"net/http"
	"reflect"

	"github.com/domonda/go-function"
	"github.com/domonda/go-types"

	"github.com/ungerik/go-fs"
	"github.com/ungerik/go-fs/multipartfs"
	"github.com/ungerik/go-httpx/httperr"
)

var typeOfFileReader = reflect.TypeOf((*fs.FileReader)(nil)).Elem()

type Option struct {
	Label string
	Value any
}

type formField struct {
	Name     string
	Label    string
	Type     string
	Value    string
	Required bool
	Options  []Option
}

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
	resultWriter function.HTTPResultsWriter
}

func NewHandler(wrappedFunc function.Wrapper, title string, resultWriter function.HTTPResultsWriter) (handler *Handler, err error) {
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

func MustNewHandler(fun function.Wrapper, title string, successHandler function.HTTPResultsWriter) (handler *Handler) {
	handler, err := NewHandler(fun, title, successHandler)
	if err != nil {
		panic(err)
	}
	return handler
}

func (handler *Handler) SetArgValidator(arg string, validator types.ValidatErr) {
	handler.argValidator[arg] = validator
}

func (handler *Handler) SetArgRequired(arg string, required bool) {
	handler.argRequired[arg] = required
}

func (handler *Handler) SetArgOptions(arg string, options []Option) {
	handler.argOptions[arg] = options
}

func (handler *Handler) SetArgDefaultValue(arg string, value any) {
	handler.argDefaultValue[arg] = value
}

func (handler *Handler) SetArgInputType(arg string, value string) {
	handler.argInputType[arg] = value
}

func (handler *Handler) SetSubmitButtonText(text string) {
	handler.form.SubmitButtonText = text
}

func (handler *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			httperr.Handle(httperr.AsError(r), response, request)
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

func (handler *Handler) get(response http.ResponseWriter, request *http.Request) {
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
			Required: true,
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
		file, _ := formfs.FormFile(key)
		argsMap[key] = string(file)
	}

	results, err := handler.wrappedFunc.CallWithNamedStrings(request.Context(), argsMap)
	if httperr.Handle(err, response, request) {
		return
	}

	err = handler.resultWriter.WriteResults(results, nil, response, request)
	httperr.Handle(err, response, request)
}
