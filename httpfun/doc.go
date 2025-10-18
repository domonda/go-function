// Package httpfun provides utilities for creating HTTP handlers from wrapped functions.
//
// # Overview
//
// The httpfun package enables you to convert any wrapped function into an HTTP handler.
// It handles request argument parsing, function execution, result formatting, and error handling.
//
// # Basic Usage
//
// Convert a function to an HTTP handler:
//
//	func Calculate(ctx context.Context, operation string, a, b int) (int, error) {
//	    switch operation {
//	    case "add":
//	        return a + b, nil
//	    case "multiply":
//	        return a * b, nil
//	    default:
//	        return 0, fmt.Errorf("unknown operation: %s", operation)
//	    }
//	}
//
//	func main() {
//	    wrapper := function.MustReflectWrapper("Calculate", Calculate)
//	    handler := httpfun.Handler(
//	        httpfun.RequestQueryArgs,     // Parse args from query params
//	        wrapper,
//	        httpfun.RespondJSON,          // Respond with JSON
//	    )
//
//	    http.Handle("/calculate", handler)
//	    http.ListenAndServe(":8080", nil)
//	}
//
// Usage:
//
//	GET /calculate?operation=add&a=5&b=3
//	Response: 8
//
//	POST /calculate with JSON: {"operation":"multiply","a":5,"b":3}
//	Response: 15
//
// # Request Argument Parsing
//
// Extract function arguments from various HTTP request sources:
//
//	// From query parameters
//	httpfun.RequestQueryArgs
//	httpfun.RequestQueryArg("argName")
//
//	// From JSON body
//	httpfun.RequestBodyJSONFieldsAsArgs
//
//	// From headers
//	httpfun.RequestHeaderArg("Authorization")
//	httpfun.RequestHeadersAsArgs(map[string]string{
//	    "Authorization": "token",
//	    "User-Agent": "userAgent",
//	})
//
//	// From form data
//	httpfun.RequestMultipartFormArgs
//
//	// From environment variables
//	httpfun.RequestArgFromEnvVar("API_KEY", "apiKey")
//
//	// Constant values
//	httpfun.ConstRequestArg("version", "1.0")
//
//	// Merge multiple sources (later sources override earlier ones)
//	httpfun.MergeRequestArgs(
//	    httpfun.ConstRequestArg("defaultValue", "abc"),
//	    httpfun.RequestQueryArgs,
//	    httpfun.RequestBodyJSONFieldsAsArgs,
//	)
//
// # Response Writers
//
// Format function results as HTTP responses:
//
//	// JSON response (default)
//	httpfun.RespondJSON
//
//	// JSON with named fields
//	httpfun.RespondJSONObject("result", "error")
//
//	// XML response
//	httpfun.RespondXML
//
//	// Plain text
//	httpfun.RespondPlaintext
//
//	// HTML
//	httpfun.RespondHTML
//
//	// Binary data with specific content type
//	httpfun.RespondBinary("application/pdf")
//
//	// Auto-detect content type
//	httpfun.RespondDetectContentType
//
//	// Static responses
//	httpfun.RespondStaticHTML("<html>...</html>")
//	httpfun.RespondStaticJSON(`{"status":"ok"}`)
//
//	// Redirects
//	httpfun.RespondRedirect("/success")
//	httpfun.RespondRedirectFunc(func(r *http.Request) (string, error) {
//	    return "/user/" + getUserID(r), nil
//	})
//
// # Custom Response Writers
//
// Create custom response formatting:
//
//	customWriter := httpfun.ResultsWriterFunc(func(
//	    results []any,
//	    err error,
//	    w http.ResponseWriter,
//	    r *http.Request,
//	) error {
//	    if err != nil {
//	        return err // Let error handler deal with it
//	    }
//
//	    // Custom formatting logic
//	    w.Header().Set("Content-Type", "text/plain")
//	    fmt.Fprintf(w, "Success: %v", results[0])
//	    return nil
//	})
//
// # Error Handling
//
// Customize error responses:
//
//	httpfun.HandleError = func(err error, w http.ResponseWriter, r *http.Request) {
//	    log.Printf("Error: %v", err)
//	    http.Error(w, "Internal Server Error", http.StatusInternalServerError)
//	}
//
// Or provide custom error handlers per endpoint:
//
//	handler := httpfun.Handler(
//	    httpfun.RequestQueryArgs,
//	    wrapper,
//	    httpfun.RespondJSON,
//	    customErrorHandler, // Handles errors for this endpoint only
//	)
//
// # Configuration
//
// Global configuration options:
//
//	// Pretty-print JSON responses (default: true)
//	httpfun.PrettyPrint = true
//	httpfun.PrettyPrintIndent = "  "
//
//	// Catch panics in handlers (default: true)
//	httpfun.CatchHandlerPanics = true
//
//	// Custom error handler (default uses go-httpx/httperr)
//	httpfun.HandleError = func(err error, w http.ResponseWriter, r *http.Request) {
//	    // Custom error handling
//	}
//
// # Advanced Examples
//
// ## REST API with Multiple Endpoints
//
//	type UserService struct{}
//
//	func (s *UserService) GetUser(ctx context.Context, id string) (*User, error) { ... }
//	func (s *UserService) CreateUser(ctx context.Context, name, email string) (*User, error) { ... }
//	func (s *UserService) DeleteUser(ctx context.Context, id string) error { ... }
//
//	func main() {
//	    svc := &UserService{}
//
//	    http.Handle("/user", httpfun.Handler(
//	        httpfun.MergeRequestArgs(
//	            httpfun.ConstRequestArg("id", ""),
//	            httpfun.RequestQueryArgs,
//	        ),
//	        function.MustReflectWrapper("GetUser", svc.GetUser),
//	        httpfun.RespondJSON,
//	    ))
//
//	    http.Handle("/user/create", httpfun.Handler(
//	        httpfun.RequestBodyJSONFieldsAsArgs,
//	        function.MustReflectWrapper("CreateUser", svc.CreateUser),
//	        httpfun.RespondJSON,
//	    ))
//
//	    http.ListenAndServe(":8080", nil)
//	}
//
// ## File Upload Handler
//
//	func ProcessFile(ctx context.Context, fileData string) (string, error) {
//	    // Process the file
//	    return "processed", nil
//	}
//
//	handler := httpfun.Handler(
//	    httpfun.RequestBodyAsArg("fileData"),
//	    function.MustReflectWrapper("ProcessFile", ProcessFile),
//	    httpfun.RespondPlaintext,
//	)
//
// ## Binary Response Handler
//
//	func GeneratePDF(ctx context.Context, title string) ([]byte, error) {
//	    // Generate PDF
//	    return pdfBytes, nil
//	}
//
//	handler := httpfun.Handler(
//	    httpfun.RequestQueryArgs,
//	    function.MustReflectWrapper("GeneratePDF", GeneratePDF),
//	    httpfun.RespondBinary("application/pdf"),
//	)
//
// # Best Practices
//
//   - Always include context.Context as the first parameter for cancellation support
//   - Return errors as the last result for proper error handling
//   - Use MergeRequestArgs to combine multiple argument sources
//   - Configure error handling before starting the server
//   - Enable panic recovery (CatchHandlerPanics) in production
//   - Use appropriate content types for your responses
//   - Validate inputs in your functions, not in request parsers
//   - Log errors in your HandleError function for debugging
//
// # Performance Considerations
//
//   - Request argument parsing happens on every request
//   - JSON marshaling can be expensive for large responses
//   - Consider disabling PrettyPrint in production for performance
//   - Use binary responses for large files instead of base64 encoding
//   - Cache wrapped functions instead of creating them per request
//
// # Thread Safety
//
// The package-level configuration variables (PrettyPrint, HandleError, etc.)
// are not thread-safe. Set them once during initialization before handling requests.
// Handler functions are safe for concurrent use.
package httpfun
