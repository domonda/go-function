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
//	    wrapper := function.MustReflectWrapper("CreateUser", CreateUser)
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
//	int, float     → <input type="number">
//	bool           → <input type="checkbox">
//	fs.FileReader  → <input type="file">
//	time.Time      → <input type="datetime-local"> (commented out in current version)
//	date.Date      → <input type="date"> (commented out in current version)
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
// Add custom validators for form fields:
//
//	handler.SetArgValidator("email", func(value any) error {
//	    email, ok := value.(string)
//	    if !ok || !strings.Contains(email, "@") {
//	        return errors.New("invalid email address")
//	    }
//	    return nil
//	})
//
//	handler.SetArgValidator("age", func(value any) error {
//	    age, ok := value.(int)
//	    if !ok || age < 18 {
//	        return errors.New("must be 18 or older")
//	    }
//	    return nil
//	})
//
// # Required Fields
//
// Fields are automatically marked as required based on their type:
//   - Non-pointer types (int, string, etc.) → Required by default
//   - Pointer types (*int, *string) → Optional
//   - String type → Optional (empty string is valid)
//   - Types implementing IsNull() → Optional
//
// Override automatic detection:
//
//	handler.SetArgRequired("email", true)   // Force required
//	handler.SetArgRequired("age", false)    // Make optional
//
// # Custom Form Templates
//
// The default form template can be customized by modifying FormTemplate
// or by creating a new template:
//
//	handler, _ := htmlform.NewHandler(wrapper, "My Form", resultWriter)
//	customTemplate := `<!DOCTYPE html>
//	<html>
//	<head><title>{{.Title}}</title></head>
//	<body>
//	    <h1>{{.Title}}</h1>
//	    <form method="post">
//	        {{range .Fields}}
//	            <label>{{.Label}}</label>
//	            <input type="{{.Type}}" name="{{.Name}}" value="{{.Value}}">
//	        {{end}}
//	        <button>{{.SubmitButtonText}}</button>
//	    </form>
//	</body>
//	</html>`
//	handler.template, _ = template.New("form").Parse(customTemplate)
//
// # File Uploads
//
// Functions accepting fs.FileReader automatically get file upload fields:
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
//	    function.MustReflectWrapper("ProcessDocument", ProcessDocument),
//	    "Upload Document",
//	    httpfun.RespondStaticHTML("<p>Document processed!</p>"),
//	)
//
// The form will include a file input field for the document parameter.
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
//	    wrapper := function.MustReflectWrapper("RegisterUser", RegisterUser)
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
// The default template includes minimal CSS styling. Customize the template
// to add your own styles, or inject CSS through the form title:
//
//	title := `My Form
//	<style>
//	    form { max-width: 600px; margin: 0 auto; }
//	    input, select, textarea { width: 100%; padding: 8px; }
//	</style>`
//	handler := htmlform.MustNewHandler(wrapper, title, resultWriter)
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
//   - Complex types (nested structs, maps) are not supported
//   - Array/slice parameters need manual handling
//   - Time.Time support is currently commented out (awaiting implementation)
//   - Form validation happens server-side only
//
// # Thread Safety
//
// Handler instances are safe for concurrent use after configuration.
// Do not modify handler settings (SetArgRequired, etc.) after the handler
// starts serving requests.
package htmlform
