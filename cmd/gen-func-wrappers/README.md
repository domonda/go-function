# gen-func-wrappers

Install:

```sh
go install github.com/domonda/go-function/cmd/gen-func-wrappers@latest
```

Test with:

```sh
go run gen-func-wrappers.go -verbose -replaceForJSON=fs.FileReader:fs.File ../../htmlform/examples/
```