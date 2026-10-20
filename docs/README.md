# go-function documentation

Start with the tutorial if you are new to go-function. The API of every package is documented on [pkg.go.dev](https://pkg.go.dev/github.com/domonda/go-function), and the code generator in its own [README](../cmd/gen-func-wrappers/README.md).

## Tutorial

Learn by building something that works.

- [One function, four interfaces](tutorial-one-function-four-ways.md): wrap a Go function and call it from Go, the command line, a JSON API, and an HTML form

## How-to guides

Steps for a specific task, assuming you know the basics.

- [How to switch from ReflectWrapper to generated wrappers](howto-generated-wrappers.md)
- [How to make your own types scannable from strings](howto-custom-string-types.md)
- [How to customize HTTP error responses](howto-http-errors.md)

## Reference

Exact behavior, for looking things up.

- [Calling conventions](reference-calling-conventions.md): `Call`, `CallWithStrings`, `CallWithNamedStrings`, `CallWithJSON`, `Description`, results handlers
- [String conversion](reference-string-conversion.md): what `ScanString` accepts for each type, slice literals, scanner lookup, configuration
- [HTTP status codes](reference-http-status-codes.md): every status of `httpfun.Handler` and its cause

## Explanation

Background and design decisions.

- [Architecture](explanation-architecture.md): wrappers, adapters, reflection vs. code generation
- [Security model](explanation-security-model.md): trust boundaries, resource types, limits, CSRF, error disclosure

## Package documentation

- [`function`](https://pkg.go.dev/github.com/domonda/go-function): wrappers, descriptions, string conversion
- [`httpfun`](https://pkg.go.dev/github.com/domonda/go-function/httpfun): HTTP handlers
- [`cli`](https://pkg.go.dev/github.com/domonda/go-function/cli): command dispatchers and shell completion
- [`htmlform`](https://pkg.go.dev/github.com/domonda/go-htmlform): HTML forms for quick internal dashboards, the separate module [go-htmlform](https://github.com/domonda/go-htmlform)
- [`gen-func-wrappers`](../cmd/gen-func-wrappers/README.md): code generator
