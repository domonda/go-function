# htmlform

Package `htmlform` generates an HTML form for a Go function wrapped with
[go-function](https://github.com/domonda/go-function) and calls the function
with the submitted values.

## Scope

`htmlform` is meant for quick internal dashboards and admin tools, like a form
for triggering a maintenance task or entering test data. It is not meant for
polished production websites: the generated forms have a plain default layout,
and the package doesn't have the same quality criteria as the root package
`github.com/domonda/go-function`.

For public websites, write the forms yourself and call your functions through
`httpfun` or the wrappers of the root package.

## Usage

```go
func CreateUser(ctx context.Context, name, email string, age int) error {
	// ...
	return nil
}

handler := htmlform.MustNewHandler(
	function.MustReflectWrapper(CreateUser, "ctx", "name", "email", "age"),
	"Create User",
	httpfun.RespondStaticHTML("<h1>User created</h1>"),
)
http.Handle("/create-user", handler)
```

A GET request shows the form, a POST request calls `CreateUser` with the parsed
values. Submissions are protected against cross-site request forgery and limited
in size. Arguments of file types like `fs.File` of
[go-fs](https://github.com/ungerik/go-fs) get upload fields.

See the [package documentation](https://pkg.go.dev/github.com/domonda/go-function/htmlform)
for field types, validators, options, custom templates and the security notes,
and [examples/main.go](examples/main.go) for a runnable example.
