# String conversion reference

[`function.ScanString(sourceStr, destPtr)`](https://pkg.go.dev/github.com/domonda/go-function#ScanString) converts a string into the value `destPtr` points to. It is the one conversion engine behind every string-based way to call a function, so a type that parses on the command line parses the same way in a URL query or a form field. This page lists which scanner handles which type, what input each type accepts, and the variables that configure it.

## Where it applies

| Caller | Uses `ScanString` |
|---|---|
| `CallWithStrings`, `CallWithNamedStrings` of `ReflectWrapper` and generated wrappers | yes, for every argument except type `any`, which gets the string unparsed |
| `cli` dispatchers | yes, through `CallWithStrings` |
| `httpfun.Handler` | yes, through `CallWithNamedStrings`, also for `RequestBodyJSONFieldsAsArgs`, which turns JSON strings into their text and other JSON values into their JSON text like `42` or `[1,2]` |
| `htmlform.Handler` | yes, for every text field except arguments of type `any`, which get the text unparsed |
| `ScanStrings(sourceStrings, destPtrs...)` | yes, pairwise, an error if the counts differ |
| `CallWithJSON` | **no**, it uses `json.Unmarshal` |

## Scanner lookup

`ScanString` calls `StringScanners.ScanString`. A [`TypeStringScanners`](https://pkg.go.dev/github.com/domonda/go-function#TypeStringScanners) tries its scanners in this order and uses the first that matches the destination type:

1. The type scanner registered for exactly the destination type.
2. Interface scanners for interfaces the destination type implements, then for interfaces only its pointer type implements (like `encoding.TextUnmarshaler` with a pointer receiver). Within each group, interfaces are sorted by `reflect.Type.String()` and `PkgPath()`, so the result doesn't depend on map order.
3. The kind scanner registered for the kind of the destination type, like `reflect.Bool`.
4. The default scanner, which is `DefaultScanString` unless replaced.

A scanner that returns an error wrapping `ErrTypeNotSupported` passes the string on to the next matching scanner. Any other error, or `nil`, ends the lookup.

`DefaultScanString` is what calls `UnmarshalText` and `UnmarshalJSON`, so a registered kind scanner also gets types of its kind that implement `encoding.TextUnmarshaler` or `json.Unmarshaler`, before their own methods would run.

Elements of slices and arrays and targets of pointers are scanned with the package-level `StringScanners` again, so a scanner registered for `T` also applies to `*T`, `[]T`, `[]*T`, and `[N]T`.

## DefaultScanString

`DefaultScanString` checks the destination in this order. The first matching row handles the string.

| Order | Destination | Accepted input | Whitespace | `""`, `nil`, `null` |
|---|---|---|---|---|
| 1 | type with a `SetNull()` method | | | calls `SetNull()` |
| 2 | `string` | anything, unchanged | kept | kept as text |
| 3 | `error` | anything | trimmed | `nil` |
| 4 | `time.Time` | layouts in `TimeFormats`, tried in order, at most 256 bytes | trimmed | zero time |
| 5 | type with a `Set(time.Time)` method | like `time.Time`, then calls `Set` | trimmed | `Set(time.Time{})` |
| 6 | `time.Duration` | `time.ParseDuration`: `1h30m`, `250ms`; `90` without unit is an error, at most 256 bytes | trimmed | `0` |
| 7 | `encoding.TextUnmarshaler` | anything, passed unchanged to `UnmarshalText` | kept | passed to `UnmarshalText` |
| 8 | `json.Unmarshaler` | valid JSON as is, else the string encoded as a JSON string; valid non-string JSON that is rejected is retried as a JSON string | trimmed if valid JSON, kept if encoded as a JSON string | `null` as JSON `null`, `""` and `nil` as JSON strings |
| 9 | `map[string]any` | a JSON object | trimmed | `nil` |
| 10 | `[]any` | a JSON array | trimmed | `nil` |
| 11 | `[]byte` | anything, unchanged bytes | kept | kept as text |
| 12 | pointer `*T` | the input for `T` | as for `T` | `nil` |
| 13 | other kinds of `string` | anything, unchanged | kept | kept as text |
| 14 | `bool` kinds | `strconv.ParseBool`: `1 t T TRUE true True 0 f F FALSE false False` | trimmed | `false` |
| 15 | `int` kinds | base-10 integer within the type's bit size | trimmed | `0` |
| 16 | `uint` kinds | base-10 unsigned integer within the type's bit size | trimmed | `0` |
| 17 | `float` kinds | `strconv.ParseFloat`: `3.14`, `1e3`, `NaN`, `Inf` | trimmed | `0` |
| 18 | struct | a JSON object for `json.Unmarshal` | trimmed | zero struct |
| 19 | slice `[]T` | a [slice literal](#slice-literals) `[a,b]`, or a single value without brackets as one element | trimmed | `nil` |
| 20 | array `[N]T` | a slice literal with exactly `N` elements | trimmed | zero array |
| 21 | other maps, chan, func | not supported, returns `ErrTypeNotSupported` | | zero value |
| 22 | everything else, like `complex128` | `fmt.Sscan` | | |

Notes:

- `""`, `nil`, and `null` are recognized after trimming and without regard to case, so `NULL` and ` nil ` count too.
- `time.Time` is matched before `encoding.TextUnmarshaler`, which it implements, so `TimeFormats` decides how times parse.
- A type implementing `encoding.TextUnmarshaler` owns all of its input, including empty strings and the nil words. Handle them in `UnmarshalText` if the type has an empty or null value.
- Only `map[string]any` maps are supported. `map[string]int` returns `type not supported: map[string]int`. Use a struct type for structured arguments.
- `ScanString` can't fill a plain `any` destination. The wrappers avoid this by passing the string itself to arguments of type `any`.

## Examples

Output of `ScanString` for the input in quotes:

| Destination | Input | Result |
|---|---|---|
| `int` | `" 42 "` | `42` |
| `int` | `"NULL"` | `0` |
| `int` | `"4.2"` | error: `can't parse "4.2" as int because of: invalid syntax` |
| `int8` | `"300"` | error: `can't parse "300" as int because of: value out of range` |
| `uint` | `"-1"` | error: `can't parse "-1" as uint because of: invalid syntax` |
| `float64` | `"1e3"` | `1000` |
| `bool` | `"T"` | `true` |
| `bool` | `"yes"` | error: `can't parse "yes" as bool because of: invalid syntax` |
| `string` | `" nil "` | `" nil "` |
| `*string` | `"nil"` | `nil` |
| `*int` | `"7"` | pointer to `7` |
| `[]byte` | `"null"` | `[]byte("null")` |
| `time.Duration` | `"1h30m"` | `90 * time.Minute` |
| `time.Time` | `"2024-01-15"` | 2024-01-15 00:00 in `TimeLocation` |
| `time.Time` | `"2024-01-15T10:30"` | 2024-01-15 10:30 in `TimeLocation` |
| `time.Time` | `"2024-01-15T10:30:00Z"` | 2024-01-15 10:30 UTC |
| `time.Time` | `"15.01.2024"` | error listing the tried `TimeFormats` |
| `struct{ A int }` | `{"A":1}` | `{A:1}` |
| `map[string]any` | `{"a":1}` | `map[a:1]` |
| `net.IP` | `"10.0.0.1"` | via `UnmarshalText` |
| `complex128` | `"(1+2i)"` | via `fmt.Sscan` |
| `error` | `"  boom "` | `errors.New("boom")` |

## Slice literals

A slice or array value is written like a JSON array whose elements can be unquoted:

| Input | `[]int` | `[]string` | Rule |
|---|---|---|---|
| `[1, 2, 3]` | `[1 2 3]` | `["1" "2" "3"]` | elements are split at top-level commas and trimmed |
| `1` | `[1]` | `["1"]` | no brackets: one element |
| `[]` or `[ ]` | `[]` (empty, not nil) | `[]` | |
| `[1, null, 3]` | `[1 0 3]` | `["1" "null" "3"]` | unquoted nil words are zero values, except for string elements, which keep the text |
| `[a, "b"]` | error | `["a" "b"]` | quoted elements are JSON strings and get unquoted |
| `["null", ""]` | error | `["null" ""]` | quoted elements are explicit strings, never nil words |
| `[[1,2],[3]]` as `[][]int` | `[[1 2] [3]]` | | nested `[]` and `{}` stay together |
| `[1,2` | error: `malformed slice literal` | | brackets must match |
| `[1,,2]` | error: `has an empty element` | | |
| `[1,2,3]` as `[2]int` | error: `needs to have 2 elements, but has 3` | | arrays need the exact count |

For `[]*string`, `[null, "null"]` scans as a nil pointer followed by a pointer to `"null"`.

`httpfun.RequestQueryArgs`, `RequestQueryArg`, `RequestQueryAsArg`, and `RequestMultipartFormArgs` pass a parameter with multiple values, like `?x=1&x=2`, as the literal `["1","2"]`. Because the elements are quoted, they are explicit strings: an empty value in `?x=1&x=` is an error for a `[]int` argument instead of a `0`, and `?x=null&x=a` gives a `[]*string` argument a pointer to `"null"` instead of a nil pointer.

A literal may hold at most `MaxSliceLiteralElements` elements. Longer literals fail with `slice value has more than 1048576 elements (MaxSliceLiteralElements)` before memory for the elements is allocated. The limit doesn't apply to `[]any` destinations, like the slice of a `...any` argument, and to arrays in JSON objects of struct or map destinations, which `encoding/json` decodes.

## Configuration

Set these once at program start, before any string is scanned. They are process-wide and not safe to change while other goroutines scan.

| Variable | Default | Effect |
|---|---|---|
| `StringScanners *TypeStringScanners` | `NewTypeStringScanners(StringScannerFunc(DefaultScanString))` | the scanners `ScanString` uses |
| `TimeFormats []string` | see below | layouts tried in order for `time.Time` and `Set(time.Time)` types |
| `TimeLocation *time.Location` | `time.Local` | zone for parsed times whose input has no zone |
| `MaxSliceLiteralElements int` | `1 << 20` | maximum elements per slice or array literal, `<= 0` disables the limit |

Default `TimeFormats`:

```go
time.RFC3339Nano,
time.RFC3339,
time.DateOnly + " 15:04:05.999999999 -0700 MST", // format of time.Time.String()
time.DateTime,
time.DateOnly + " 15:04",
time.DateOnly + "T15:04:05.999999999", // browser datetime-local inputs with seconds
time.DateOnly + "T15:04",              // browser datetime-local inputs
time.DateOnly,
```

## TypeStringScanners

| Method | Returns a copy that |
|---|---|
| `NewTypeStringScanners(defaultScanner)` | (constructor) has only a default scanner, `nil` returns `ErrTypeNotSupported` for everything |
| `WithTypeScanner(destType, scanner)` | scans exactly `destType` with `scanner` |
| `WithInterfaceTypeScanner(iface, scanner)` | scans types whose value or pointer implements the interface `iface` |
| `WithKindScanner(kind, scanner)` | scans types of `kind` that no type or interface scanner matched |
| `WithDefaultScanner(scanner)` | uses `scanner` when nothing else matches, `nil` returns `ErrTypeNotSupported` |

A `TypeStringScanners` never changes after creation, so a value can be shared between goroutines. `WithTypeScanner`, `WithInterfaceTypeScanner`, and `WithKindScanner` panic for a nil type or scanner, and `WithInterfaceTypeScanner` panics for a type that is not an interface. A nil `*TypeStringScanners` returns `ErrTypeNotSupported` for every destination.

A scanner always receives a non-nil pointer to the destination as `destPtr`.

## Related

- [How to make your own types scannable from strings](howto-custom-string-types.md)
- [Calling conventions reference](reference-calling-conventions.md): how each convention passes strings to `ScanString`
- [Security model](explanation-security-model.md): why a scanned string can be dangerous for some types
