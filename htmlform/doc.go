// Package htmlform provides utilities for automatically generating HTML forms from function signatures.
//
// # Overview
//
// The htmlform package enables you to create web forms that automatically generate
// HTML input fields based on a function's parameters. When the form is submitted,
// the values are parsed and passed to the function, making it easy to create
// web interfaces for any Go function.
//
// # Basic Usage
//
// Generate an HTML form from a function:
//
//	func CreateUser(ctx context.Context, name, email string, age int, active bool) error {
//	    fmt.Printf("Creating user: %s (%s), age %d, active: %v\n", name, email, age, active)
//	    return nil
//	}
//
//	func main() {
//	    wrapper := function.MustReflectWrapper(CreateUser, "ctx", "name", "email", "age", "active")
//	    handler := htmlform.MustNewHandler(
//	        wrapper,
//	        "Create New User",           // Form title
//	        httpfun.RespondStaticHTML("<h1>User created successfully!</h1>"),
//	    )
//
//	    http.Handle("/create-user", handler)
//	    http.ListenAndServe(":8080", nil)
//	}
//
// When users visit /create-user:
//   - GET request: Displays an HTML form with fields for name, email, age, and active
//   - POST request: Submits the form, calls CreateUser, and displays success message
//
// # Automatic Field Type Detection
//
// The package automatically selects appropriate HTML input types based on Go types:
//
//	string         → <input type="text">
//	int, uint      → <input type="number">
//	float          → <input type="number" step="any">
//	bool           → <input type="checkbox">
//	fs.FileReader  → <input type="file">
//	other types    → <input type="text">
//
// Types that parse their own strings, like time.Duration ("5s")
// and types implementing encoding.TextUnmarshaler, get text fields
// also if their kind is a number.
// Set input types like "datetime-local" or "date" for time.Time with SetArgInputType,
// default values of time.Time are then formatted for these inputs.
// Arguments named _ get no form field and are passed as zero values.
//
// # Customizing Fields
//
// You can customize form fields using the Handler methods:
//
//	handler := htmlform.MustNewHandler(wrapper, "User Form", resultWriter)
//
//	// Set field as required (overrides type-based detection)
//	handler.SetArgRequired("email", true)
//
//	// Set default value
//	handler.SetArgDefaultValue("age", 18)
//	handler.SetArgDefaultValue("active", true)
//
//	// Set custom input type
//	handler.SetArgInputType("email", "email")
//	handler.SetArgInputType("bio", "textarea")
//	handler.SetArgInputType("password", "password")
//
//	// Add dropdown options
//	handler.SetArgOptions("role", []htmlform.Option{
//	    {Label: "User", Value: "user"},
//	    {Label: "Admin", Value: "admin"},
//	    {Label: "Guest", Value: "guest"},
//	})
//
//	// Customize submit button
//	handler.SetSubmitButtonText("Create User")
//
// # Field Validation
//
// The SetArg* methods of Handler panic for names that are not
// arguments of the wrapped function, so a typo or a renamed argument
// can't silently remove a validator or another setting.
//
// Add custom validators for form fields:
//
//	handler.SetArgValidator("email", func(ctx context.Context, value any) error {
//	    email, ok := value.(string)
//	    if !ok || !strings.Contains(email, "@") {
//	        return errors.New("invalid email address")
//	    }
//	    return nil
//	})
//
//	handler.SetArgValidator("age", func(ctx context.Context, value any) error {
//	    age, ok := value.(int)
//	    if !ok || age < 18 {
//	        return errors.New("must be 18 or older")
//	    }
//	    return nil
//	})
//
// Validators are called with the context of the request and the argument values
// parsed from the submitted form, or the zero value of an argument that was not submitted.
// The function is called with exactly these values.
// If a validator returns an error, then the function is not called
// and a 400 Bad Request error naming the argument is passed to the result writer.
// The text of the validator error is shown to the client in that response.
//
// # Required Fields
//
// Fields are automatically marked as required based on their type:
//   - Non-pointer types like int → Required by default
//   - Pointer types (*int, *string) → Optional
//   - String type → Optional (empty string is valid)
//   - Types implementing IsNull() → Optional
//   - bool checkboxes are never marked as required, unchecked is false
//   - A variadic last argument → Optional, like in Go it can have no values
//
// Required fields are enforced by browsers with the required attribute
// of the form field, the server passes the zero value for a missing value.
//
// Override automatic detection:
//
//	handler.SetArgRequired("email", true)   // Force required
//	handler.SetArgRequired("age", false)    // Make optional
//
// # Custom Form Templates
//
// The default form template can be customized by assigning
// a custom html/template text to FormTemplate before creating handlers,
// because NewHandler parses FormTemplate:
//
//	htmlform.FormTemplate = `<!DOCTYPE html>
//	<html>
//	<head><title>{{.Title}}</title></head>
//	<body>
//	    <h1>{{.Title}}</h1>
//	    <form method="post" enctype="multipart/form-data">
//	        {{range .Fields}}
//	            <label>{{.Label}}</label>
//	            {{if eq .Type "checkbox"}}
//	                <input type="checkbox" name="{{.Name}}" value="true" {{if eq .Value "true"}}checked{{end}}>
//	            {{else}}
//	                <input type="{{.Type}}" name="{{.Name}}" value="{{.Value}}">
//	            {{end}}
//	        {{end}}
//	        <button>{{.SubmitButtonText}}</button>
//	    </form>
//	</body>
//	</html>`
//	handler, err := htmlform.NewHandler(wrapper, "My Form", resultWriter)
//
// # File Uploads
//
// Function arguments with a type implementing fs.FileReader,
// like fs.File or fs.FileReader, automatically get file upload fields.
// They are only taken from files uploaded with the form
// and passed as fs.File of the multipart form file,
// which is only readable while the request is handled.
// Without an uploaded file the argument is the zero value,
// which is nil for fs.FileReader.
// Pointers, slices and arrays of such types are file arguments too:
// a slice or array gets all files uploaded with the same field name,
// and its form field allows selecting multiple files.
// A non-empty text value submitted for a file argument is rejected
// with 400 Bad Request without calling the function,
// because it would be used as path of a file on the server.
// A file argument of a type that fs.File can't be assigned to,
// like fs.MemFile or a pointer to a slice of files, is an error of NewHandler,
// and so is a file type in a struct field or map of an argument,
// which uploads can't fill and a text value would set to a path on the server.
//
//	func ProcessDocument(ctx context.Context, name string, document fs.FileReader) error {
//	    data, err := document.ReadAll()
//	    if err != nil {
//	        return err
//	    }
//	    fmt.Printf("Processing %s: %d bytes\n", name, len(data))
//	    return nil
//	}
//
//	handler := htmlform.MustNewHandler(
//	    function.MustReflectWrapper(ProcessDocument, "ctx", "name", "document"),
//	    "Upload Document",
//	    httpfun.RespondStaticHTML("<p>Document processed!</p>"),
//	)
//
// The form will include a file input field for the document parameter.
//
// WARNING: Never expose functions with fs.File or fs.FileReader arguments
// to untrusted input through string arguments of httpfun handlers or cli commands,
// or through JSON (CallWithJSON, httpfun.RequestBodyJSONFieldsAsArgs,
// or a -replaceForJSON of gen-func-wrappers to fs.File):
// a string becomes the path of a file on the server
// that the function then reads or writes on behalf of the client.
// For the same reason don't register a process-global StringScanner
// that converts strings to fs.FileReader. NewHandler rejects arguments
// with file types in struct fields and maps, but can't detect types
// that read a file path in their own UnmarshalJSON or UnmarshalText method.
//
// # Complete Example
//
//	package main
//
//	import (
//	    "context"
//	    "fmt"
//	    "net/http"
//
//	    "github.com/domonda/go-function"
//	    "github.com/domonda/go-function/htmlform"
//	    "github.com/domonda/go-function/httpfun"
//	)
//
//	func RegisterUser(ctx context.Context, username, email, password string, age int, newsletter bool) error {
//	    fmt.Printf("Registering: %s (%s), age %d, newsletter: %v\n",
//	        username, email, age, newsletter)
//	    return nil
//	}
//
//	func main() {
//	    wrapper := function.MustReflectWrapper(RegisterUser, "ctx", "username", "email", "password", "age", "newsletter")
//	    handler := htmlform.MustNewHandler(
//	        wrapper,
//	        "User Registration",
//	        httpfun.RespondStaticHTML("<h2>Registration successful!</h2>"),
//	    )
//
//	    // Customize fields
//	    handler.SetArgInputType("email", "email")
//	    handler.SetArgInputType("password", "password")
//	    handler.SetArgRequired("email", true)
//	    handler.SetArgDefaultValue("age", 18)
//	    handler.SetSubmitButtonText("Register")
//
//	    http.Handle("/register", handler)
//	    fmt.Println("Server running on http://localhost:8080/register")
//	    http.ListenAndServe(":8080", nil)
//	}
//
// # Result Handling
//
// After form submission, use any httpfun.ResultsWriter to handle the response:
//
//	// Simple success message
//	httpfun.RespondStaticHTML("<h1>Success!</h1>")
//
//	// JSON response
//	httpfun.RespondJSON
//
//	// Redirect after success
//	httpfun.RespondRedirect("/thank-you")
//
//	// Custom response based on results
//	httpfun.ResultsWriterFunc(func(results []any, err error, w http.ResponseWriter, r *http.Request) error {
//	    if err != nil {
//	        return err
//	    }
//	    fmt.Fprintf(w, "<p>Created user with ID: %v</p>", results[0])
//	    return nil
//	})
//
// # Request Size Limit
//
// The body of a form submission including uploaded files is limited to 100 MiB,
// larger requests are answered with 413 Request Entity Too Large.
// Uploaded files larger than httpfun.MaxMultipartFormMemory
// are buffered in temporary files instead of memory:
//
//	handler.SetMaxRequestBodySize(10 << 20) // 10 MiB
//
// # Cross-Site Request Forgery Protection
//
// Form submissions are protected against cross-site request forgery (CSRF)
// with http.CrossOriginProtection: POST requests that browsers send from other origins
// are answered with 403 Forbidden without calling the function.
// Requests of non-browser clients without the Sec-Fetch-Site and Origin headers are accepted.
// Configure trusted origins or disable the protection with nil:
//
//	protection := http.NewCrossOriginProtection()
//	err := protection.AddTrustedOrigin("https://example.com")
//	if err != nil {
//	    panic(err)
//	}
//	handler.SetCrossOriginProtection(protection)
//
// # Error Handling
//
// The handler automatically:
//   - Catches panics and displays them as HTTP errors
//   - Validates form submissions
//   - Handles multipart form data (including file uploads)
//   - Returns appropriate HTTP status codes
//
// Errors from the wrapped function are passed to the result writer,
// which can handle them appropriately.
//
// # Styling
//
// The default template includes minimal CSS styling.
// Customize FormTemplate to add your own styles,
// the title is HTML escaped and can't be used to inject CSS:
//
//	htmlform.FormTemplate = strings.Replace(htmlform.FormTemplate,
//	    "</style>",
//	    "form { max-width: 600px; margin: 0 auto; }\n</style>",
//	    1,
//	)
//	handler := htmlform.MustNewHandler(wrapper, "My Form", resultWriter)
//
// # Best Practices
//
//   - Use context.Context as the first parameter for cancellation support
//   - Return error as the last result for proper error handling
//   - Set sensible default values for optional fields
//   - Use appropriate input types (email, password, url, tel, etc.)
//   - Provide clear labels through function documentation
//   - Add validators for complex validation logic
//   - Use dropdown options for fields with limited choices
//   - Test file upload handling with size limits
//   - Customize the success response to match your application style
//
// # Limitations
//
//   - Context.Context parameters are automatically excluded from the form
//   - Maps other than map[string]any are not supported,
//     struct and map[string]any arguments are entered as JSON in a text field
//   - Slices and arrays of files get a multiple file upload field,
//     other slices and arrays are entered as one text value like [1,2,3],
//     or as multiple values of the field in a custom template,
//     like of a select with the multiple attribute
//   - A variadic argument is not required, because it can have no values
//   - time.Time arguments get a text field unless their input type
//     is set to "datetime-local" with SetArgInputType
//   - The server only validates with the validators set by SetArgValidator
//     and by parsing the values, required fields are only enforced by browsers
//
// # Thread Safety
//
// Handler instances are safe for concurrent use after configuration.
// Do not modify handler settings (SetArgRequired, etc.) after the handler
// starts serving requests.
package htmlform
