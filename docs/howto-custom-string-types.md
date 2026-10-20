# How to make your own types scannable from strings

This guide makes a type you use as a function argument parse from strings, so it works as a CLI argument, URL query parameter, form field, and JSON request body field.

## Prerequisites

- go-function v1.0.0 or later
- A wrapped function with an argument of a type that `ScanString` doesn't handle the way you want. Calls fail with errors like `can't parse string value of argument 'level' ...` or `type not supported`.

Pick the step that matches your type:

| Situation | Step |
|---|---|
| You own the type | [1. Implement `encoding.TextUnmarshaler`](#1-implement-encodingtextunmarshaler-on-a-type-you-own) |
| The type is from another package | [2. Register a type scanner](#2-register-a-type-scanner-for-a-type-you-dont-own) |
| You want different parsing for every type of a kind, like all bools | [3. Register a kind scanner](#3-register-a-kind-scanner-to-change-parsing-for-a-whole-kind) |

## Steps

### 1. Implement encoding.TextUnmarshaler on a type you own

`ScanString` passes the string unchanged to `UnmarshalText` of any type that implements it. `encoding/json` uses the same method for JSON strings, so one method covers `CallWithJSON` too.

```go
// Level is a log level that is scanned from its name.
type Level int

const (
	Debug Level = iota
	Info
	Warn
)

// UnmarshalText implements encoding.TextUnmarshaler.
func (l *Level) UnmarshalText(text []byte) error {
	switch strings.ToLower(strings.TrimSpace(string(text))) {
	case "debug", "":
		*l = Debug
	case "info":
		*l = Info
	case "warn":
		*l = Warn
	default:
		return fmt.Errorf("unknown level %q", text)
	}
	return nil
}
```

`UnmarshalText` gets all input as it is, including surrounding whitespace, the empty string, `nil`, and `null`. Decide what those mean for your type, like the `""` case above.

Every convention now accepts level names, also as elements of slices and variadic arguments:

```go
func SetLevel(level Level, extra ...Level) string { return fmt.Sprint(level, extra) }

setLevel := function.MustReflectWrapper(SetLevel, "level", "extra")

setLevel.CallWithStrings(ctx, "warn", "info", "debug")
// [2 [1 0]] <nil>
setLevel.CallWithNamedStrings(ctx, map[string]string{"level": "info", "extra": "[warn, debug]"})
// [1 [2 0]] <nil>
setLevel.CallWithJSON(ctx, []byte(`{"level": "warn", "extra": ["info"]}`))
// [2 [1]] <nil>
setLevel.CallWithStrings(ctx, "loud")
// [] can't parse string value of argument 'level' of function main.SetLevel(level main.Level, extra ...main.Level) string: unknown level "loud"
```

### 2. Register a type scanner for a type you don't own

You can't add methods to `url.URL`, so register a scanner in `function.StringScanners` at program start. The `With` methods return a modified copy, so assign the result back:

```go
func init() {
	function.StringScanners = function.StringScanners.WithTypeScanner(
		reflect.TypeFor[url.URL](),
		function.StringScannerFunc(func(sourceStr string, destPtr any) error {
			u, err := url.Parse(sourceStr)
			if err != nil {
				return err
			}
			*destPtr.(*url.URL) = *u
			return nil
		}),
	)
}
```

`destPtr` is always a non-nil pointer to the destination, here a `*url.URL`.

Register the scanner for the value type `url.URL`, not for `*url.URL`. The default scanner then handles pointers: `""`, `nil`, and `null` become a nil `*url.URL`, and any other string is scanned by your scanner into a new `url.URL`. Slices like `[]*url.URL` use your scanner for each element:

```go
func Fetch(u *url.URL, mirrors []*url.URL) string {
	return fmt.Sprint(u.Host, " ", len(mirrors))
}

fetch := function.MustReflectWrapper(Fetch, "u", "mirrors")
fetch.CallWithStrings(ctx, "https://example.com/x", "[https://a.example.com, https://b.example.com]")
// [example.com 2] <nil>
```

A scanner registered for `*url.URL` itself would also get `"nil"` and turn it into a URL whose path is `nil`.

### 3. Register a kind scanner to change parsing for a whole kind

A kind scanner applies to every type of a `reflect.Kind` that no type or interface scanner matched. Return `function.ErrTypeNotSupported` for input you don't handle, and `ScanString` passes it on to the default scanner:

```go
func init() {
	function.StringScanners = function.StringScanners.WithKindScanner(
		reflect.Bool,
		function.StringScannerFunc(func(sourceStr string, destPtr any) error {
			v := reflect.ValueOf(destPtr).Elem()
			switch strings.ToLower(strings.TrimSpace(sourceStr)) {
			case "yes", "on":
				v.SetBool(true)
			case "no", "off":
				v.SetBool(false)
			default:
				// Let the default scanner parse "true", "1", "", ...
				return function.ErrTypeNotSupported
			}
			return nil
		}),
	)
}
```

Use `reflect.Value.SetBool` instead of `*destPtr.(*bool)`, because the destination can be a named type like `type Flag bool`.

For `func Enabled(on bool) bool`:

| String | Result |
|---|---|
| `yes` | `true` |
| `off` | `false` |
| `true` | `true`, from the default scanner |
| `0` | `false`, from the default scanner |
| `""` | `false`, from the default scanner |
| `maybe` | error: `can't parse "maybe" as bool because of: invalid syntax` |

To match every type that implements an interface, use `WithInterfaceTypeScanner(reflect.TypeFor[YourInterface](), scanner)` the same way.

## Verification

Test the conversion directly with `function.ScanString`, which uses the same scanners as the wrappers:

```go
func TestScanLevel(t *testing.T) {
	var level Level
	err := function.ScanString("warn", &level)
	if err != nil || level != Warn {
		t.Fatalf("got %v, %v", level, err)
	}
}

func TestScanURL(t *testing.T) {
	var u *url.URL
	err := function.ScanString("https://example.com/path", &u)
	if err != nil || u.Host != "example.com" {
		t.Fatalf("got %v, %v", u, err)
	}
	err = function.ScanString("nil", &u)
	if err != nil || u != nil {
		t.Fatalf("got %v, %v", u, err)
	}
}
```

Scanners registered in `init` of the package under test are active in its tests.

## Troubleshooting

**The scanner is never called.**
- The result of `WithTypeScanner` wasn't assigned to `function.StringScanners`. The method returns a copy and leaves the original unchanged.
- The type scanner is registered for a different type than the argument. Type scanners match the exact type: one for `url.URL` also covers `*url.URL` and `[]url.URL` arguments, but one for `*url.URL` doesn't cover `url.URL`.
- A kind scanner is never called for a type that a type or interface scanner already handled, unless that scanner returned `ErrTypeNotSupported`.

**A kind scanner changed how a type with `UnmarshalText` parses.**
Kind scanners run before the default scanner, and the default scanner is what calls `UnmarshalText`. A kind scanner for `reflect.Int` therefore also gets `Level` from step 1. Skip such types in the kind scanner:

```go
if _, ok := destPtr.(encoding.TextUnmarshaler); ok {
	return function.ErrTypeNotSupported
}
```

**Strings work, but `CallWithJSON` fails with `cannot unmarshal string into Go value of type url.URL`.**
`CallWithJSON` uses `json.Unmarshal`, not `ScanString`. Registered scanners don't apply to it. Use a type that implements `encoding.TextUnmarshaler` or `json.Unmarshaler`. Note that `httpfun.RequestBodyJSONFieldsAsArgs` passes JSON body fields through `ScanString`, so registered scanners do apply there.

**`type not supported: map[string]int`.**
`ScanString` only handles `map[string]any` maps. Use a struct type, which is scanned from a JSON object, or register a type scanner for the map type.

**Empty input or `null` produces a value instead of a nil pointer or zero value.**
Type scanners, interface scanners, and `UnmarshalText` receive those strings like any other. Check for them in the scanner, or register for the value type as in step 2.

**The scanner panics with a type assertion error.**
`destPtr` is a pointer to the destination. For a scanner registered for `T`, assert `destPtr.(*T)`. For kind and interface scanners, the concrete type varies: use `reflect.ValueOf(destPtr).Elem()`.

## Related

- [String conversion reference](reference-string-conversion.md): the lookup order and what each type accepts by default
- [Calling conventions reference](reference-calling-conventions.md): which conventions use `ScanString`
- [Security model](explanation-security-model.md#types-that-name-resources): types you should not make scannable from untrusted input
