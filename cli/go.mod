module github.com/domonda/go-function/cli

go 1.24.0

replace github.com/domonda/go-function => ../

require github.com/domonda/go-function v0.0.0-00010101000000-000000000000 // replaced

require (
	github.com/fatih/color v1.18.0
	github.com/posener/complete/v2 v2.1.0
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/posener/script v1.2.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
)
