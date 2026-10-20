package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_sliceLiteralFields(t *testing.T) {
	tests := []struct {
		name       string
		sourceStr  string
		wantFields []string
		wantErr    bool
	}{
		{
			name:      "empty",
			sourceStr: ``,
			wantErr:   true,
		},
		{
			name:       "empty[]",
			sourceStr:  `[]`,
			wantFields: nil,
		},
		{
			name:       `[a]`,
			sourceStr:  `[a]`,
			wantFields: []string{`a`},
		},
		{
			name:       `[a,b]`,
			sourceStr:  `[a,b]`,
			wantFields: []string{`a`, `b`},
		},
		{
			name:       `[a, b]`,
			sourceStr:  `[a, b]`,
			wantFields: []string{`a`, `b`},
		},
		{
			name:       `[a, "b,c"]`,
			sourceStr:  `[a, "b,c"]`,
			wantFields: []string{`a`, `"b,c"`},
		},
		{
			name:       `["[quoted", "{", "comma,string", "}"]`,
			sourceStr:  `["[quoted", "{", "comma,string", "}"]`,
			wantFields: []string{`"[quoted"`, `"{"`, `"comma,string"`, `"}"`},
		},
		{
			name:       `[[1,2,3], {"key": "comma,string"}, null]`,
			sourceStr:  `[[1,2,3], {"key": "comma,string"}, null]`,
			wantFields: []string{`[1,2,3]`, `{"key": "comma,string"}`, `null`},
		},

		// JSON escape handling — without proper escape tracking these inputs
		// caused the splitter to silently drop trailing fields when partner
		// names contained quotes (idwell-finance-service masterdata import bug).
		{
			name:       `escaped quote inside string`,
			sourceStr:  `["a\"b","c"]`,
			wantFields: []string{`"a\"b"`, `"c"`},
		},
		{
			name:       `escaped quote at start of string`,
			sourceStr:  `["\"Die Familienunternehmer e.V.","next"]`,
			wantFields: []string{`"\"Die Familienunternehmer e.V."`, `"next"`},
		},
		{
			name:       `escaped quote in middle of string`,
			sourceStr:  `["Abwasserzweckverband \"Chemnitz","next"]`,
			wantFields: []string{`"Abwasserzweckverband \"Chemnitz"`, `"next"`},
		},
		{
			name:       `escaped backslash before closing quote`,
			sourceStr:  `["path\\","next"]`,
			wantFields: []string{`"path\\"`, `"next"`},
		},
		{
			name:       `escaped backslash followed by escaped quote`,
			sourceStr:  `["path\\\"end","next"]`,
			wantFields: []string{`"path\\\"end"`, `"next"`},
		},
		{
			name: `partner-companies reproducer with escaped quotes in names`,
			sourceStr: `[` +
				`{"Name":"Abwasserzweckverband \"Chemnitz","VendorAccountNumber":"77226"},` +
				`{"Name":"\"Die Familienunternehmer e.V.","VendorAccountNumber":"77483"},` +
				`{"Name":"CANARY","VendorAccountNumber":"99999"}` +
				`]`,
			wantFields: []string{
				`{"Name":"Abwasserzweckverband \"Chemnitz","VendorAccountNumber":"77226"}`,
				`{"Name":"\"Die Familienunternehmer e.V.","VendorAccountNumber":"77483"}`,
				`{"Name":"CANARY","VendorAccountNumber":"99999"}`,
			},
		},
		{
			// Invalid UTF-8 (raw 0xE4 from Latin-1 ä) inside a quoted string
			// must not trip the splitter — the bad byte is the JSON decoder's
			// problem, not the array splitter's.
			name:      `invalid utf8 byte inside quoted string`,
			sourceStr: "[{\"Name\":\"Karli Geb\xe4udereinigung GbR\",\"VendorAccountNumber\":\"81230\"},{\"Name\":\"CANARY\",\"VendorAccountNumber\":\"99999\"}]",
			wantFields: []string{
				"{\"Name\":\"Karli Geb\xe4udereinigung GbR\",\"VendorAccountNumber\":\"81230\"}",
				`{"Name":"CANARY","VendorAccountNumber":"99999"}`,
			},
		},

		// Validation of unbalanced state — previously the function silently
		// returned a partial prefix, which let bad inputs corrupt the batch.
		{
			name:      `unterminated string literal`,
			sourceStr: `["abc, "next"]`,
			wantErr:   true,
		},
		{
			name:      `unbalanced opening brace`,
			sourceStr: `[{"a":1]`,
			wantErr:   true,
		},
		{
			name:      `unbalanced opening bracket`,
			sourceStr: `[[1,2]`,
			wantErr:   true,
		},
		{
			name:      `unescaped quote inside object value desyncs and is rejected`,
			sourceStr: `[{"Name":"Abwasserzweckverband "Chemnitz","V":"77226"}]`,
			wantErr:   true,
		},

		// Malformed literals must be rejected instead of producing
		// merged fields or phantom zero-value elements.
		{
			name:       `brackets with only whitespace are an empty slice like []`,
			sourceStr:  "[ \t\n]",
			wantFields: nil,
		},
		{
			name:      `content after closing bracket`,
			sourceStr: `[1][2]`,
			wantErr:   true,
		},
		{
			name:      `content after closing bracket with too many closing brackets`,
			sourceStr: `[1]]`,
			wantErr:   true,
		},
		{
			name:      `only a comma`,
			sourceStr: `[,]`,
			wantErr:   true,
		},
		{
			name:      `empty first element`,
			sourceStr: `[,1]`,
			wantErr:   true,
		},
		{
			name:      `empty middle element`,
			sourceStr: `[1, ,2]`,
			wantErr:   true,
		},
		{
			name:      `trailing comma`,
			sourceStr: `[1,]`,
			wantErr:   true,
		},
		{
			name:      `trailing comma with whitespace`,
			sourceStr: `[1, ]`,
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFields, err := sliceLiteralFields(tt.sourceStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("sliceLiteralFields() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotFields, tt.wantFields) {
				t.Errorf("sliceLiteralFields() = %v, want %v", gotFields, tt.wantFields)
			}
		})
	}
}

func Test_unquoteSliceElement(t *testing.T) {
	// unquoteSliceElement skips json.Unmarshal for simple JSON strings
	// for performance, so its result must be identical to json.Unmarshal
	// with and without escapes, else the same slice literal would
	// be scanned differently depending on which path it takes.
	fields := []string{
		// Fast path
		`""`,
		`"a"`,
		`" a b "`,
		`"comma,string"`,
		`"[1,2]"`,
		`"null"`,
		`"ä€😀"`,
		"\"\x7f\"",
		// json.Unmarshal path
		`"a\"b"`,
		`"c\\d"`,
		`"ä"`,
		`"tab\tnewline\n"`,
		`"\/"`,
		"\"invalid utf8 \xe4\"",
		// Invalid JSON strings are not unquoted
		"\"raw\ttab\"",
		"\"raw\nnewline\"",
		`"a"b"`,
		`"bad escape \x"`,
		`"`,
		`"a`,
		`a"`,
		`a`,
		`1`,
		`null`,
		`{"a":"b"}`,
	}
	for _, field := range fields {
		// Reference implementation without fast path
		var want string
		wantOK := strings.HasPrefix(field, `"`) && json.Unmarshal([]byte(field), &want) == nil
		if !wantOK {
			want = field
		}

		got, gotOK := unquoteSliceElement(field)
		assert.Equal(t, wantOK, gotOK, "field %q", field)
		assert.Equal(t, want, got, "field %q", field)
	}
}

func TestScanString(t *testing.T) {
	var boolPtr *bool
	intMap := make(map[int]int)
	type TestStruct struct {
		Int int
		Str string
	}
	type args struct {
		sourceStr string
		destPtr   any
	}
	tests := []struct {
		name     string
		args     args
		wantDest any
		wantErr  bool
	}{
		{
			name:     "int(666)",
			args:     args{sourceStr: " \t666\n", destPtr: new(int)},
			wantDest: int(666),
		},
		{
			name:     "empty string as int",
			args:     args{sourceStr: "", destPtr: new(int)},
			wantDest: int(0),
		},
		{
			name:     "empty string map[int]int",
			args:     args{sourceStr: "", destPtr: &intMap},
			wantDest: map[int]int(nil),
		},
		{
			name:     "nil map[int]int",
			args:     args{sourceStr: " nil ", destPtr: &intMap},
			wantDest: map[int]int(nil),
		},
		{
			name:     "null map[int]int",
			args:     args{sourceStr: "null", destPtr: &intMap},
			wantDest: map[int]int(nil),
		},
		{
			name:     "nil bool",
			args:     args{sourceStr: "false", destPtr: &boolPtr},
			wantDest: new(bool), // ptr to false
		},
		{
			name:     "empty string scanned as false",
			args:     args{sourceStr: "    ", destPtr: new(bool)},
			wantDest: false,
		},
		{
			name:     "struct",
			args:     args{sourceStr: `{"Int": 1, "Str": "test"}`, destPtr: &TestStruct{}},
			wantDest: TestStruct{Int: 1, Str: "test"},
		},
		{
			name:     "struct slice",
			args:     args{sourceStr: `[{"Int": 1, "Str": "test"}, {"Int": 2, "Str": "test2"}]`, destPtr: &[]*TestStruct{}},
			wantDest: []*TestStruct{{Int: 1, Str: "test"}, {Int: 2, Str: "test2"}},
		},
		{
			name:     "struct slice, non trimmed string",
			args:     args{sourceStr: ` [{"Int": 1,"Str":"test"},{"Int": 2,    "Str": "test2"}]` + "\n", destPtr: &[]*TestStruct{}},
			wantDest: []*TestStruct{{Int: 1, Str: "test"}, {Int: 2, Str: "test2"}},
		},

		// nilSrc handling — empty/null inputs become the zero/nil value of
		// the destination type instead of bubbling up JSON parse errors.
		// scanString is intentionally tolerant for CLI/HTTP arg use.
		{
			name:     "empty string into struct",
			args:     args{sourceStr: "", destPtr: &TestStruct{Int: 99, Str: "stale"}},
			wantDest: TestStruct{},
		},
		{
			name:     "null into struct",
			args:     args{sourceStr: "null", destPtr: &TestStruct{Int: 99, Str: "stale"}},
			wantDest: TestStruct{},
		},
		{
			name:     "whitespace into struct",
			args:     args{sourceStr: "   \t\n", destPtr: &TestStruct{Int: 99, Str: "stale"}},
			wantDest: TestStruct{},
		},
		{
			name:     "empty string into *map[string]any",
			args:     args{sourceStr: "", destPtr: &map[string]any{"stale": 1}},
			wantDest: map[string]any(nil),
		},
		{
			name:     "null into *map[string]any",
			args:     args{sourceStr: "null", destPtr: &map[string]any{"stale": 1}},
			wantDest: map[string]any(nil),
		},
		{
			name:     "JSON null into *map[string]any (already worked, lock it in)",
			args:     args{sourceStr: `{"a":1}`, destPtr: &map[string]any{}},
			wantDest: map[string]any{"a": float64(1)},
		},
		{
			name:     "empty string into *[]any",
			args:     args{sourceStr: "", destPtr: &[]any{"stale"}},
			wantDest: []any(nil),
		},
		{
			name:     "null into *[]any",
			args:     args{sourceStr: "null", destPtr: &[]any{"stale"}},
			wantDest: []any(nil),
		},

		// Malformed slice literal — a stray '[' or ']' should error explicitly
		// rather than silently fall through to single-element interpretation,
		// which used to surface as misleading downstream parse errors.
		{
			name:    "truncated slice literal missing close bracket",
			args:    args{sourceStr: "[1,2,3", destPtr: &[]int{}},
			wantErr: true,
		},
		{
			name:    "stray close bracket without open",
			args:    args{sourceStr: "1,2,3]", destPtr: &[]int{}},
			wantErr: true,
		},
		{
			name:    "truncated slice literal into array",
			args:    args{sourceStr: "[1,2", destPtr: &[2]int{}},
			wantErr: true,
		},
		{
			name:    "stray close bracket into array",
			args:    args{sourceStr: "1,2]", destPtr: &[2]int{}},
			wantErr: true,
		},
		{
			// The single-element fallback still works when there is neither
			// '[' nor ']' — required for CLI scalar-as-slice convenience.
			name:     "scalar coerced to single-element slice",
			args:     args{sourceStr: "42", destPtr: &[]int{}},
			wantDest: []int{42},
		},

		// // wantErr
		{
			name:    "nil destPtr",
			args:    args{sourceStr: "nil", destPtr: nil},
			wantErr: true,
		},
		{
			// A non-pointer destination is a caller error that
			// must be returned as error instead of panicking
			name:    "non-pointer destPtr",
			args:    args{sourceStr: "1", destPtr: 5},
			wantErr: true,
		},
		{
			name:    "typed nil destPtr",
			args:    args{sourceStr: "1", destPtr: (*int)(nil)},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ScanString(tt.args.sourceStr, tt.args.destPtr); (err != nil) != tt.wantErr {
				t.Errorf("ScanString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			gotDest := reflect.ValueOf(tt.args.destPtr).Elem().Interface()
			if !reflect.DeepEqual(gotDest, tt.wantDest) {
				t.Errorf("ScanString() set %#v, want %#v", gotDest, tt.wantDest)
			}
		})
	}
}

// timeSetter is a minimal stub for the `interface{ Set(time.Time) }` branch
// in scanString — exercised by the second TimeLocation case below.
type timeSetter struct{ t time.Time }

func (s *timeSetter) Set(t time.Time) { s.t = t }

func TestScanString_TimeLocation(t *testing.T) {
	originalLoc := TimeLocation
	t.Cleanup(func() { TimeLocation = originalLoc })

	// given — a naive timestamp string with no timezone component
	naive := "2024-01-15 10:00:00"

	t.Run("default time.Local interprets naive timestamp in local zone", func(t *testing.T) {
		// given
		TimeLocation = time.Local

		// when
		var got time.Time
		err := ScanString(naive, &got)

		// then
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want, _ := time.ParseInLocation(time.DateTime, naive, time.Local)
		if !got.Equal(want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("override to UTC interprets naive timestamp in UTC", func(t *testing.T) {
		// given
		TimeLocation = time.UTC

		// when
		var got time.Time
		err := ScanString(naive, &got)

		// then
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want, _ := time.ParseInLocation(time.DateTime, naive, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got %v (loc=%v), want %v (loc=%v)", got, got.Location(), want, want.Location())
		}
		if got.Location() != time.UTC {
			t.Errorf("got location %v, want UTC", got.Location())
		}
	})

	t.Run("override to fixed zone applies to interface{ Set(time.Time) } branch too", func(t *testing.T) {
		// given
		paris, err := time.LoadLocation("Europe/Paris")
		if err != nil {
			t.Skipf("zoneinfo unavailable: %v", err)
		}
		TimeLocation = paris

		// when
		var dest timeSetter
		err = ScanString(naive, &dest)

		// then
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want, _ := time.ParseInLocation(time.DateTime, naive, paris)
		if !dest.t.Equal(want) {
			t.Errorf("got %v, want %v", dest.t, want)
		}
	})

	t.Run("source string with explicit timezone ignores TimeLocation", func(t *testing.T) {
		// given
		TimeLocation = time.UTC

		// when
		var got time.Time
		err := ScanString("2024-01-15T10:00:00+02:00", &got)

		// then
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The +02:00 offset must be preserved, not replaced by UTC.
		_, offset := got.Zone()
		if offset != 2*60*60 {
			t.Errorf("got offset %d seconds, want 7200 (+02:00)", offset)
		}
	})
}

func TestStringScannerFunc_ScanString(t *testing.T) {
	type args struct {
		sourceStr string
		destPtr   any
	}
	tests := []struct {
		name     string
		f        StringScannerFunc
		args     args
		wantDest any
		wantErr  bool
	}{
		{
			name: "custom scanner always sets 42",
			f: func(sourceStr string, destPtr any) error {
				if ptr, ok := destPtr.(*int); ok {
					*ptr = 42
					return nil
				}
				return nil
			},
			args:     args{sourceStr: "100", destPtr: new(int)},
			wantDest: 42,
		},
		{
			name: "custom scanner with error",
			f: func(sourceStr string, destPtr any) error {
				return fmt.Errorf("custom error")
			},
			args:    args{sourceStr: "test", destPtr: new(string)},
			wantErr: true,
		},
		{
			name: "custom scanner uppercases strings",
			f: func(sourceStr string, destPtr any) error {
				if ptr, ok := destPtr.(*string); ok {
					*ptr = strings.ToUpper(sourceStr)
					return nil
				}
				return fmt.Errorf("expected *string")
			},
			args:     args{sourceStr: "hello", destPtr: new(string)},
			wantDest: "HELLO",
		},
		{
			name:     "default scanner as StringScannerFunc",
			f:        StringScannerFunc(DefaultScanString),
			args:     args{sourceStr: "true", destPtr: new(bool)},
			wantDest: true,
		},
		{
			name:     "chained scanner - default implementation",
			f:        StringScannerFunc(DefaultScanString),
			args:     args{sourceStr: "123", destPtr: new(int)},
			wantDest: 123,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.f.ScanString(tt.args.sourceStr, tt.args.destPtr)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringScannerFunc.ScanString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			gotDest := reflect.ValueOf(tt.args.destPtr).Elem().Interface()
			if !reflect.DeepEqual(gotDest, tt.wantDest) {
				t.Errorf("StringScannerFunc.ScanString() set %#v, want %#v", gotDest, tt.wantDest)
			}
		})
	}
}

// jsonStringValue is a json.Unmarshaler that only accepts JSON strings
type jsonStringValue string

func (s *jsonStringValue) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, (*string)(s))
}

// jsonIntValue is a json.Unmarshaler that only accepts JSON numbers
type jsonIntValue int

func (i *jsonIntValue) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, (*int)(i))
}

func Test_scanString(t *testing.T) {
	type args struct {
		sourceStr string
		destVal   reflect.Value
	}
	tests := []struct {
		name     string
		args     args
		wantDest any
		wantErr  bool
	}{
		// String types
		{
			name:     "string with whitespace",
			args:     args{sourceStr: "  hello world  ", destVal: reflect.ValueOf(new(string)).Elem()},
			wantDest: "  hello world  ",
		},
		{
			name:     "empty string",
			args:     args{sourceStr: "", destVal: reflect.ValueOf(new(string)).Elem()},
			wantDest: "",
		},

		// Boolean types
		{
			name:     "bool true",
			args:     args{sourceStr: "true", destVal: reflect.ValueOf(new(bool)).Elem()},
			wantDest: true,
		},
		{
			name:     "bool false",
			args:     args{sourceStr: "false", destVal: reflect.ValueOf(new(bool)).Elem()},
			wantDest: false,
		},
		{
			name:     "bool 1",
			args:     args{sourceStr: "1", destVal: reflect.ValueOf(new(bool)).Elem()},
			wantDest: true,
		},
		{
			name:     "bool empty string becomes false",
			args:     args{sourceStr: "", destVal: reflect.ValueOf(new(bool)).Elem()},
			wantDest: false,
		},

		// Integer types
		{
			name:     "int positive",
			args:     args{sourceStr: "42", destVal: reflect.ValueOf(new(int)).Elem()},
			wantDest: 42,
		},
		{
			name:     "int negative",
			args:     args{sourceStr: "-123", destVal: reflect.ValueOf(new(int)).Elem()},
			wantDest: -123,
		},
		{
			name:     "int8",
			args:     args{sourceStr: "127", destVal: reflect.ValueOf(new(int8)).Elem()},
			wantDest: int8(127),
		},
		{
			name:     "int16",
			args:     args{sourceStr: "32000", destVal: reflect.ValueOf(new(int16)).Elem()},
			wantDest: int16(32000),
		},
		{
			name:     "int32",
			args:     args{sourceStr: "2147483647", destVal: reflect.ValueOf(new(int32)).Elem()},
			wantDest: int32(2147483647),
		},
		{
			name:     "int64",
			args:     args{sourceStr: "9223372036854775807", destVal: reflect.ValueOf(new(int64)).Elem()},
			wantDest: int64(9223372036854775807),
		},
		{
			name:     "int nil value",
			args:     args{sourceStr: "nil", destVal: reflect.ValueOf(new(int)).Elem()},
			wantDest: 0,
		},

		// Unsigned integer types
		{
			name:     "uint",
			args:     args{sourceStr: "42", destVal: reflect.ValueOf(new(uint)).Elem()},
			wantDest: uint(42),
		},
		{
			name:     "uint8",
			args:     args{sourceStr: "255", destVal: reflect.ValueOf(new(uint8)).Elem()},
			wantDest: uint8(255),
		},
		{
			name:     "uint16",
			args:     args{sourceStr: "65535", destVal: reflect.ValueOf(new(uint16)).Elem()},
			wantDest: uint16(65535),
		},
		{
			name:     "uint32",
			args:     args{sourceStr: "4294967295", destVal: reflect.ValueOf(new(uint32)).Elem()},
			wantDest: uint32(4294967295),
		},
		{
			name:     "uint64",
			args:     args{sourceStr: "18446744073709551615", destVal: reflect.ValueOf(new(uint64)).Elem()},
			wantDest: uint64(18446744073709551615),
		},

		// Float types
		{
			name:     "float32",
			args:     args{sourceStr: "3.14", destVal: reflect.ValueOf(new(float32)).Elem()},
			wantDest: float32(3.14),
		},
		{
			name:     "float64",
			args:     args{sourceStr: "3.141592653589793", destVal: reflect.ValueOf(new(float64)).Elem()},
			wantDest: 3.141592653589793,
		},
		{
			name:     "float64 scientific notation",
			args:     args{sourceStr: "1.23e10", destVal: reflect.ValueOf(new(float64)).Elem()},
			wantDest: 1.23e10,
		},
		{
			name:     "float nil value",
			args:     args{sourceStr: "null", destVal: reflect.ValueOf(new(float64)).Elem()},
			wantDest: float64(0),
		},

		// Pointer types
		{
			name: "pointer to int",
			args: args{sourceStr: "42", destVal: reflect.ValueOf(new(*int)).Elem()},
			wantDest: func() *int {
				v := 42
				return &v
			}(),
		},
		{
			name:     "pointer nil",
			args:     args{sourceStr: "nil", destVal: reflect.ValueOf(new(*int)).Elem()},
			wantDest: (*int)(nil),
		},
		{
			name: "pointer to string",
			args: args{sourceStr: "hello", destVal: reflect.ValueOf(new(*string)).Elem()},
			wantDest: func() *string {
				v := "hello"
				return &v
			}(),
		},

		// Slice types
		{
			name:     "int slice",
			args:     args{sourceStr: "[1,2,3]", destVal: reflect.ValueOf(new([]int)).Elem()},
			wantDest: []int{1, 2, 3},
		},
		{
			// Elements that are JSON strings are unquoted,
			// so the documented JSON-like syntax works
			name:     "string slice",
			args:     args{sourceStr: `["a","b","c"]`, destVal: reflect.ValueOf(new([]string)).Elem()},
			wantDest: []string{"a", "b", "c"},
		},
		{
			name:     "string slice with JSON escapes",
			args:     args{sourceStr: `["a\"b", "c\\d", "\u00e4", " e "]`, destVal: reflect.ValueOf(new([]string)).Elem()},
			wantDest: []string{`a"b`, `c\d`, "ä", " e "},
		},
		{
			// Unquoted elements keep working for CLI convenience
			name:     "string slice without quotes",
			args:     args{sourceStr: `[a, b]`, destVal: reflect.ValueOf(new([]string)).Elem()},
			wantDest: []string{"a", "b"},
		},
		{
			name:     "string slice with quoted and unquoted elements",
			args:     args{sourceStr: `[a, "b,c"]`, destVal: reflect.ValueOf(new([]string)).Elem()},
			wantDest: []string{"a", "b,c"},
		},
		{
			name:     "quoted elements of non-string type",
			args:     args{sourceStr: `["5s", "1m"]`, destVal: reflect.ValueOf(new([]time.Duration)).Elem()},
			wantDest: []time.Duration{5 * time.Second, time.Minute},
		},

		// A quoted element is an explicit string value, so "" and "null"
		// must not get the nil/zero shortcut of unquoted empty/null values.
		// Else [1, "", 3] silently became [1 0 3] and ["null", true]
		// became [false true] where json.Unmarshal rejects the string.
		{
			name:    "quoted empty element of int slice",
			args:    args{sourceStr: `[1, "", 3]`, destVal: reflect.ValueOf(new([]int)).Elem()},
			wantErr: true,
		},
		{
			name:    "quoted null element of int slice",
			args:    args{sourceStr: `[1, "null"]`, destVal: reflect.ValueOf(new([]int)).Elem()},
			wantErr: true,
		},
		{
			name:    "quoted empty element of float array",
			args:    args{sourceStr: `[""]`, destVal: reflect.ValueOf(new([1]float64)).Elem()},
			wantErr: true,
		},
		{
			name:    "quoted empty element of bool slice",
			args:    args{sourceStr: `["", true]`, destVal: reflect.ValueOf(new([]bool)).Elem()},
			wantErr: true,
		},
		{
			name:    "quoted null element of bool slice",
			args:    args{sourceStr: `["null", true]`, destVal: reflect.ValueOf(new([]bool)).Elem()},
			wantErr: true,
		},
		{
			name:    "quoted null element of struct slice",
			args:    args{sourceStr: `[{"A": 1}, "null"]`, destVal: reflect.ValueOf(new([]struct{ A int })).Elem()},
			wantErr: true,
		},
		{
			// The whole quoted element is an explicit string also when it
			// is scanned as single-element slice of a nested slice
			name:    "quoted empty element of nested int slice",
			args:    args{sourceStr: `[[1], ""]`, destVal: reflect.ValueOf(new([][]int)).Elem()},
			wantErr: true,
		},
		{
			name:     "quoted empty and null elements of string pointer slice",
			args:     args{sourceStr: `["", "null", " nil "]`, destVal: reflect.ValueOf(new([]*string)).Elem()},
			wantDest: []*string{new(""), new("null"), new(" nil ")},
		},
		{
			name:     "quoted empty and null elements of string slice",
			args:     args{sourceStr: `["", "null", " "]`, destVal: reflect.ValueOf(new([]string)).Elem()},
			wantDest: []string{"", "null", " "},
		},
		{
			// A quoted "null" is passed to UnmarshalJSON as JSON string, not as JSON null
			name:     "quoted null element of json.Unmarshaler slice",
			args:     args{sourceStr: `["null", null]`, destVal: reflect.ValueOf(new([]jsonStringValue)).Elem()},
			wantDest: []jsonStringValue{"null", ""},
		},
		{
			// Like the error arguments of CallWithJSON,
			// an empty error message is no error
			name:     "quoted empty and null elements of error slice",
			args:     args{sourceStr: `["", "null", null]`, destVal: reflect.ValueOf(new([]error)).Elem()},
			wantDest: []error{nil, errors.New("null"), nil},
		},
		{
			// Unquoted null/nil elements keep meaning the zero/nil value
			name:     "unquoted null and nil elements of int slice",
			args:     args{sourceStr: `[1, null, nil, 3]`, destVal: reflect.ValueOf(new([]int)).Elem()},
			wantDest: []int{1, 0, 0, 3},
		},
		{
			name:     "unquoted null elements of string pointer slice",
			args:     args{sourceStr: `[null, a, NULL]`, destVal: reflect.ValueOf(new([]*string)).Elem()},
			wantDest: []*string{nil, new("a"), nil},
		},
		{
			name:     "unquoted null element of struct slice",
			args:     args{sourceStr: `[{"A": 1}, null]`, destVal: reflect.ValueOf(new([]struct{ A int })).Elem()},
			wantDest: []struct{ A int }{{A: 1}, {}},
		},

		{
			// A single value without brackets is not a JSON-like literal
			// and is scanned verbatim like a string argument
			name:     "single quoted string element without brackets",
			args:     args{sourceStr: `"a"`, destVal: reflect.ValueOf(new([]string)).Elem()},
			wantDest: []string{`"a"`},
		},
		{
			name:     "empty slice",
			args:     args{sourceStr: "[]", destVal: reflect.ValueOf(new([]int)).Elem()},
			wantDest: []int{},
		},
		{
			// Whitespace between the brackets must not
			// produce a phantom zero-value element
			name:     "empty slice with whitespace",
			args:     args{sourceStr: "[ ]", destVal: reflect.ValueOf(new([]int)).Elem()},
			wantDest: []int{},
		},
		{
			name:     "slice nil",
			args:     args{sourceStr: "nil", destVal: reflect.ValueOf(new([]int)).Elem()},
			wantDest: []int(nil),
		},
		{
			name:     "nested slice",
			args:     args{sourceStr: "[[1,2],[3,4]]", destVal: reflect.ValueOf(new([][]int)).Elem()},
			wantDest: [][]int{{1, 2}, {3, 4}},
		},

		// Array types
		{
			name:     "int array",
			args:     args{sourceStr: "[1,2,3]", destVal: reflect.ValueOf(new([3]int)).Elem()},
			wantDest: [3]int{1, 2, 3},
		},
		{
			name:     "string array",
			args:     args{sourceStr: `["a","b"]`, destVal: reflect.ValueOf(new([2]string)).Elem()},
			wantDest: [2]string{"a", "b"},
		},
		{
			// Arrays must treat empty/nil/null input as zero value
			// like every other kind, also overwriting previous values
			name:     "array null",
			args:     args{sourceStr: "null", destVal: reflect.ValueOf(&[2]int{1, 2}).Elem()},
			wantDest: [2]int{},
		},
		{
			name:     "array nil",
			args:     args{sourceStr: " nil ", destVal: reflect.ValueOf(&[3]string{"a", "b", "c"}).Elem()},
			wantDest: [3]string{},
		},
		{
			name:     "array empty string",
			args:     args{sourceStr: "", destVal: reflect.ValueOf(&[2]int{1, 2}).Elem()},
			wantDest: [2]int{},
		},

		// Duration
		{
			name:     "duration seconds",
			args:     args{sourceStr: "5s", destVal: reflect.ValueOf(new(time.Duration)).Elem()},
			wantDest: 5 * time.Second,
		},
		{
			name:     "duration minutes",
			args:     args{sourceStr: "10m", destVal: reflect.ValueOf(new(time.Duration)).Elem()},
			wantDest: 10 * time.Minute,
		},
		{
			name:     "duration hours",
			args:     args{sourceStr: "2h", destVal: reflect.ValueOf(new(time.Duration)).Elem()},
			wantDest: 2 * time.Hour,
		},
		{
			name:     "duration nil",
			args:     args{sourceStr: "nil", destVal: reflect.ValueOf(new(time.Duration)).Elem()},
			wantDest: time.Duration(0),
		},

		// Byte slice
		{
			name:     "byte slice",
			args:     args{sourceStr: "hello", destVal: reflect.ValueOf(new([]byte)).Elem()},
			wantDest: []byte("hello"),
		},

		// encoding.TextUnmarshaler gets the unmodified string,
		// so netip.Addr decides itself what an empty string means
		// and rejects whitespace and "null" like netip.ParseAddr does
		{
			name:     "TextUnmarshaler",
			args:     args{sourceStr: "1.2.3.4", destVal: reflect.ValueOf(new(netip.Addr)).Elem()},
			wantDest: netip.MustParseAddr("1.2.3.4"),
		},
		{
			name:     "TextUnmarshaler empty string",
			args:     args{sourceStr: "", destVal: reflect.ValueOf(new(netip.MustParseAddr("1.2.3.4"))).Elem()},
			wantDest: netip.Addr{},
		},

		// A string valued json.Unmarshaler must accept input that happens
		// to be valid non-string JSON like a number or bool as string
		{
			name:     "string json.Unmarshaler with text",
			args:     args{sourceStr: "abc", destVal: reflect.ValueOf(new(jsonStringValue)).Elem()},
			wantDest: jsonStringValue("abc"),
		},
		{
			name:     "string json.Unmarshaler with JSON string",
			args:     args{sourceStr: `"abc"`, destVal: reflect.ValueOf(new(jsonStringValue)).Elem()},
			wantDest: jsonStringValue("abc"),
		},
		{
			name:     "string json.Unmarshaler with number",
			args:     args{sourceStr: "123", destVal: reflect.ValueOf(new(jsonStringValue)).Elem()},
			wantDest: jsonStringValue("123"),
		},
		{
			name:     "string json.Unmarshaler with bool",
			args:     args{sourceStr: "true", destVal: reflect.ValueOf(new(jsonStringValue)).Elem()},
			wantDest: jsonStringValue("true"),
		},
		{
			name:     "string json.Unmarshaler with object",
			args:     args{sourceStr: `{"a":1}`, destVal: reflect.ValueOf(new(jsonStringValue)).Elem()},
			wantDest: jsonStringValue(`{"a":1}`),
		},
		{
			name:     "number json.Unmarshaler with number",
			args:     args{sourceStr: "123", destVal: reflect.ValueOf(new(jsonIntValue)).Elem()},
			wantDest: jsonIntValue(123),
		},

		// Error cases
		{
			name:    "invalid int",
			args:    args{sourceStr: "not-a-number", destVal: reflect.ValueOf(new(int)).Elem()},
			wantErr: true,
		},
		{
			name:    "int overflow int8",
			args:    args{sourceStr: "256", destVal: reflect.ValueOf(new(int8)).Elem()},
			wantErr: true,
		},
		{
			name:    "invalid uint",
			args:    args{sourceStr: "-1", destVal: reflect.ValueOf(new(uint)).Elem()},
			wantErr: true,
		},
		{
			name:    "invalid float",
			args:    args{sourceStr: "not-a-float", destVal: reflect.ValueOf(new(float64)).Elem()},
			wantErr: true,
		},
		{
			name:    "invalid bool",
			args:    args{sourceStr: "not-a-bool", destVal: reflect.ValueOf(new(bool)).Elem()},
			wantErr: true,
		},
		{
			name:    "invalid duration",
			args:    args{sourceStr: "invalid", destVal: reflect.ValueOf(new(time.Duration)).Elem()},
			wantErr: true,
		},
		{
			name:    "array wrong size",
			args:    args{sourceStr: "[1,2]", destVal: reflect.ValueOf(new([3]int)).Elem()},
			wantErr: true,
		},
		{
			name:    "invalid slice literal",
			args:    args{sourceStr: "[1,2,3", destVal: reflect.ValueOf(new([]int)).Elem()},
			wantErr: true,
		},
		{
			name:    "empty slice element",
			args:    args{sourceStr: "[,]", destVal: reflect.ValueOf(new([]int)).Elem()},
			wantErr: true,
		},
		{
			name:    "content after slice literal",
			args:    args{sourceStr: "[1][2]", destVal: reflect.ValueOf(new([]string)).Elem()},
			wantErr: true,
		},
		{
			name:    "TextUnmarshaler with whitespace",
			args:    args{sourceStr: " 1.2.3.4\n", destVal: reflect.ValueOf(new(netip.Addr)).Elem()},
			wantErr: true,
		},
		{
			name:    "TextUnmarshaler null",
			args:    args{sourceStr: "null", destVal: reflect.ValueOf(new(netip.MustParseAddr("1.2.3.4"))).Elem()},
			wantErr: true,
		},
		{
			name:    "TextUnmarshaler only whitespace",
			args:    args{sourceStr: "  ", destVal: reflect.ValueOf(new(netip.MustParseAddr("1.2.3.4"))).Elem()},
			wantErr: true,
		},
		{
			// The original error of UnmarshalJSON is returned
			// if the input also can't be unmarshalled as JSON string
			name:    "number json.Unmarshaler with text",
			args:    args{sourceStr: "abc", destVal: reflect.ValueOf(new(jsonIntValue)).Elem()},
			wantErr: true,
		},
		{
			name:     "map type not supported via scanString",
			args:     args{sourceStr: "nil", destVal: reflect.ValueOf(new(map[string]int)).Elem()},
			wantDest: map[string]int(nil),
		},
		{
			name:    "chan type not supported",
			args:    args{sourceStr: "not-nil", destVal: reflect.ValueOf(new(chan int)).Elem()},
			wantErr: true,
		},
		{
			name:    "func type not supported",
			args:    args{sourceStr: "not-nil", destVal: reflect.ValueOf(new(func())).Elem()},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := scanString(tt.args.sourceStr, tt.args.destVal, false)
			if (err != nil) != tt.wantErr {
				t.Errorf("scanString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			gotDest := tt.args.destVal.Interface()
			if !reflect.DeepEqual(gotDest, tt.wantDest) {
				t.Errorf("scanString() set %#v, want %#v", gotDest, tt.wantDest)
			}
		})
	}
}

func TestScanString_JSONUnmarshalerRetryError(t *testing.T) {
	// Valid non-string JSON rejected by a json.Unmarshaler is retried
	// as JSON string for string valued types. If the retry fails too,
	// the error for the JSON as passed must be returned, not the error
	// for the JSON string the scanner made of it, which would tell
	// the caller that a string was passed where they passed a bool or object.
	for _, source := range []string{"true", `{"a":1}`} {
		wantErr := json.Unmarshal([]byte(source), new(int))
		require.Error(t, wantErr, "source %q", source)
		retryErr := json.Unmarshal([]byte(strconv.Quote(source)), new(int))
		require.Error(t, retryErr, "source %q", source)
		require.NotEqual(t, wantErr.Error(), retryErr.Error(), "source %q", source)

		var dest jsonIntValue
		err := ScanString(source, &dest)
		assert.EqualError(t, err, wantErr.Error(), "source %q", source)
	}
}

// validatedName is a string type with a scanner
// registered in StringScanners that rejects invalid names
type validatedName string

var errInvalidName = errors.New("invalid name")

// setValidatedNameScanner registers a scanner for validatedName in StringScanners
// that rejects "bad" and "" and upper cases valid names, so tests can tell
// whether the registered scanner or the default scanner handled a value.
func setValidatedNameScanner(t *testing.T) {
	t.Helper()
	orig := StringScanners
	t.Cleanup(func() { StringScanners = orig })
	StringScanners = StringScanners.WithTypeScanner(
		reflect.TypeFor[validatedName](),
		StringScannerFunc(func(sourceStr string, destPtr any) error {
			if sourceStr == "bad" || sourceStr == "" {
				return fmt.Errorf("%w: %q", errInvalidName, sourceStr)
			}
			*destPtr.(*validatedName) = validatedName(strings.ToUpper(sourceStr))
			return nil
		}),
	)
}

func TestScanString_RegisteredScannerForNestedValues(t *testing.T) {
	// A scanner registered for a type validates every value of that type.
	// Slice and array elements and pointer targets are scanned by DefaultScanString,
	// which used to skip registered scanners for them, so the same argument was
	// rejected by CallWithStrings (variadic elements are scanned one by one)
	// but accepted by CallWithNamedStrings (the whole slice is scanned at once).
	setValidatedNameScanner(t)

	t.Run("variadic elements via CallWithNamedStrings", func(t *testing.T) {
		f := MustReflectWrapper(func(names ...validatedName) []validatedName { return names }, "names")

		_, err := f.CallWithNamedStrings(context.Background(), map[string]string{"names": `[ok, bad]`})
		assert.ErrorIs(t, err, errInvalidName)

		results, err := f.CallWithNamedStrings(context.Background(), map[string]string{"names": `[a, "b"]`})
		require.NoError(t, err)
		assert.Equal(t, []any{[]validatedName{"A", "B"}}, results)

		// Same validation as for the variadic elements passed one by one
		_, err = f.CallWithStrings(context.Background(), "ok", "bad")
		assert.ErrorIs(t, err, errInvalidName)
	})

	t.Run("pointer target via CallWithStrings", func(t *testing.T) {
		f := MustReflectWrapper(func(name *validatedName) *validatedName { return name }, "name")

		_, err := f.CallWithStrings(context.Background(), "bad")
		assert.ErrorIs(t, err, errInvalidName)

		results, err := f.CallWithStrings(context.Background(), "ok")
		require.NoError(t, err)
		assert.Equal(t, []any{new(validatedName("OK"))}, results)

		// An unquoted null is still the nil pointer
		results, err = f.CallWithStrings(context.Background(), "null")
		require.NoError(t, err)
		assert.Equal(t, []any{(*validatedName)(nil)}, results)
	})

	t.Run("array elements and nested pointers", func(t *testing.T) {
		var arr [2]validatedName
		assert.ErrorIs(t, ScanString(`[ok, bad]`, &arr), errInvalidName)

		var ptrs []*validatedName
		assert.ErrorIs(t, ScanString(`[ok, bad]`, &ptrs), errInvalidName)
		require.NoError(t, ScanString(`[ok, null]`, &ptrs))
		assert.Equal(t, []*validatedName{new(validatedName("OK")), nil}, ptrs)
	})

	t.Run("quoted empty element", func(t *testing.T) {
		// A quoted "" is an explicit string that is not interpreted
		// as nil/zero value, but it must still be validated
		// by the registered scanner instead of the default scanner
		var names []validatedName
		assert.ErrorIs(t, ScanString(`[ok, ""]`, &names), errInvalidName)

		var ptrs []*validatedName
		assert.ErrorIs(t, ScanString(`[ok, ""]`, &ptrs), errInvalidName)
	})
}

func TestScanString_customDefaultScannerForNestedValues(t *testing.T) {
	// Without registered scanners, nested values are scanned directly
	// by DefaultScanString only if it is the default scanner of StringScanners,
	// another default scanner still gets the slice elements
	orig := StringScanners
	t.Cleanup(func() { StringScanners = orig })
	var scanned []string
	StringScanners = NewTypeStringScanners(StringScannerFunc(func(sourceStr string, destPtr any) error {
		scanned = append(scanned, sourceStr)
		return DefaultScanString(sourceStr, destPtr)
	}))

	var ints []int
	require.NoError(t, ScanString("[1,2]", &ints))
	assert.Equal(t, []int{1, 2}, ints)
	assert.Equal(t, []string{"[1,2]", "1", "2"}, scanned)
}

// BenchmarkScanString_intSlice measures scanning the elements of a slice literal,
// which the default StringScanners scans without looking up scanners per element
func BenchmarkScanString_intSlice(b *testing.B) {
	var sb strings.Builder
	sb.WriteByte('[')
	for i := range 1000 {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.Itoa(i))
	}
	sb.WriteByte(']')
	src := sb.String()
	b.ReportAllocs()
	for b.Loop() {
		var v []int
		if err := ScanString(src, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDefaultScanString_WithoutStringScanners(t *testing.T) {
	// DefaultScanString scans nested values with StringScanners,
	// but must keep working on its own if StringScanners is nil
	// instead of failing for every slice element and pointer target.
	orig := StringScanners
	t.Cleanup(func() { StringScanners = orig })
	StringScanners = nil

	var ints []int
	require.NoError(t, DefaultScanString(`[1, null, 3]`, &ints))
	assert.Equal(t, []int{1, 0, 3}, ints)
	assert.Error(t, DefaultScanString(`[1, ""]`, &ints), "quoted empty element is no int")

	var strs []*string
	require.NoError(t, DefaultScanString(`["", null]`, &strs))
	assert.Equal(t, []*string{new(""), nil}, strs)
}

// minLenPassword is an encoding.TextUnmarshaler that validates its input
// like a password type rejecting too short passwords
type minLenPassword string

func (p *minLenPassword) UnmarshalText(text []byte) error {
	if len(text) < 8 {
		return fmt.Errorf("password needs at least 8 characters, got %d", len(text))
	}
	*p = minLenPassword(text)
	return nil
}

// nullableText is an encoding.TextUnmarshaler with a SetNull method
type nullableText struct {
	text   string
	isNull bool
}

func (n *nullableText) UnmarshalText(text []byte) error {
	*n = nullableText{text: string(text)}
	return nil
}

func (n *nullableText) SetNull() { *n = nullableText{isNull: true} }

func TestScanString_TextUnmarshaler(t *testing.T) {
	// An encoding.TextUnmarshaler owns its parsing and validation,
	// so it must get the unmodified string. Trimming whitespace or setting
	// the zero value for "", "nil", or "null" without calling UnmarshalText
	// let input like an empty password bypass the validation of the type.
	t.Run("validating type rejects short input", func(t *testing.T) {
		for _, source := range []string{"", "  ", "nil", "null", "NULL", "short"} {
			var dest minLenPassword
			err := ScanString(source, &dest)
			assert.Error(t, err, "source %q", source)
		}
	})

	t.Run("whitespace is not trimmed", func(t *testing.T) {
		var dest minLenPassword
		err := ScanString("  secret12  ", &dest)
		require.NoError(t, err)
		assert.Equal(t, minLenPassword("  secret12  "), dest)
	})

	t.Run("SetNull takes precedence", func(t *testing.T) {
		// A SetNull method is how a type defines its own null value,
		// so it is still used for "", "nil", and "null" instead of UnmarshalText
		for _, source := range []string{"", " null ", "nil"} {
			dest := nullableText{text: "previous"}
			err := ScanString(source, &dest)
			require.NoError(t, err, "source %q", source)
			assert.Equal(t, nullableText{isNull: true}, dest, "source %q", source)
		}
		var dest nullableText
		err := ScanString(" x ", &dest)
		require.NoError(t, err)
		assert.Equal(t, nullableText{text: " x "}, dest)
	})
}

func TestScanString_QuotedElementsStayStrings(t *testing.T) {
	// A quoted element of a slice literal is an explicit string value,
	// so destinations that decode JSON must get a JSON string,
	// not the number, bool or object that the unquoted text also is
	var raw []json.RawMessage
	err := ScanString(`["123", "true", "{\"a\":1}", "x"]`, &raw)
	require.NoError(t, err)
	assert.Equal(t, []json.RawMessage{json.RawMessage(`"123"`), json.RawMessage(`"true"`), json.RawMessage(`"{\"a\":1}"`), json.RawMessage(`"x"`)}, raw)

	var anys []any
	err = ScanString(`["123", 123]`, &anys)
	require.NoError(t, err)
	assert.Equal(t, []any{"123", float64(123)}, anys, "only the unquoted element is a number")
}

func TestScanString_noAllocationsPerSliceElement(t *testing.T) {
	// Request bodies can contain slice literals with millions of elements,
	// so scanning must not allocate memory per element,
	// only for the slices that grow with the number of elements
	allocs := func(numElems int) float64 {
		src := "[" + strings.Repeat("1,", numElems-1) + "1]"
		return testing.AllocsPerRun(10, func() {
			var dest []int8
			err := ScanString(src, &dest)
			require.NoError(t, err)
		})
	}
	assert.Less(t, allocs(1000)-allocs(10), 50.0)
}

func TestScanString_maxSliceLiteralElements(t *testing.T) {
	// Scanning a slice literal allocates memory for every element,
	// so the number of elements of untrusted input like HTTP request bodies
	// is limited, and the error doesn't repeat the possibly huge input
	orig := MaxSliceLiteralElements
	t.Cleanup(func() { MaxSliceLiteralElements = orig })
	MaxSliceLiteralElements = 3

	var ints []int
	err := ScanString("[1,2,3]", &ints)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, ints)

	err = ScanString("[1,2,3,4]", &ints)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "1,2,3,4")

	var array [4]int
	err = ScanString("[1,2,3,4]", &array)
	assert.Error(t, err, "arrays are limited too")

	var nested [][]int
	err = ScanString("[[1],[1,2,3,4]]", &nested)
	assert.Error(t, err, "nested literals are limited too")

	MaxSliceLiteralElements = 0
	err = ScanString("[1,2,3,4]", &ints)
	require.NoError(t, err, "no limit")
}

func TestScanStrings(t *testing.T) {
	var (
		a int
		b string
	)
	require.NoError(t, ScanStrings([]string{"42", "hello"}, &a, &b))
	assert.Equal(t, 42, a)
	assert.Equal(t, "hello", b)

	// Different numbers of strings and destinations are an error
	// instead of silently leaving destinations or strings unscanned
	a, b = 0, ""
	assert.EqualError(t, ScanStrings([]string{"1"}, &a, &b), "can't scan 1 strings into 2 destinations")
	assert.EqualError(t, ScanStrings([]string{"1", "x", "y"}, &a, &b), "can't scan 3 strings into 2 destinations")
	assert.Zero(t, a, "nothing is scanned for different numbers")
}

func TestScanString_numberErrorsQuoteStringOnce(t *testing.T) {
	// The strconv errors contain the parsed string,
	// which the error of ScanString already quotes
	var n int
	err := ScanString("abc", &n)
	assert.EqualError(t, err, `can't parse "abc" as int because of: invalid syntax`)
	assert.ErrorIs(t, err, strconv.ErrSyntax)

	var u uint8
	err = ScanString("300", &u)
	assert.EqualError(t, err, `can't parse "300" as uint because of: value out of range`)
	assert.ErrorIs(t, err, strconv.ErrRange)

	var d time.Duration
	err = ScanString("abc", &d)
	assert.EqualError(t, err, `can't parse time.Duration: time: invalid duration "abc"`)
}

func TestScanString_errorsQuoteBoundedValue(t *testing.T) {
	// Errors quote the scanned string, which can be a large untrusted input
	// like an HTTP request body up to httpfun.MaxRequestBodySize,
	// so they only quote its beginning instead of a copy that needs
	// several times the memory of the input
	long := strings.Repeat("\x01", 1<<20)
	tests := []struct {
		name string
		src  string
		dest any
	}{
		{name: "int", src: long, dest: new(int)},
		{name: "float", src: long, dest: new(float64)},
		{name: "time", src: long, dest: new(time.Time)},
		// The errors of the time package quote the whole string
		{name: "Set(time.Time)", src: long, dest: new(settableTime)},
		{name: "duration", src: long, dest: new(time.Duration)},
		{name: "duration with number", src: "1" + long, dest: new(time.Duration)},
		// Scanned like the other unsigned integers instead of with fmt.Sscan
		{name: "uintptr", src: long, dest: new(uintptr)},
		{name: "malformed slice literal", src: "[" + long, dest: new([]int)},
		{name: "empty element", src: "[," + long + "]", dest: new([]int)},
		{name: "content after closing bracket", src: "[1]" + long + "]", dest: new([]int)},
		{name: "unterminated string", src: `["` + long + "]", dest: new([]int)},
		{name: "array length", src: "[" + long + "]", dest: new([2]string)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ScanString(tt.src, tt.dest)
			require.Error(t, err)
			assert.Less(t, len(err.Error()), 2000, "error must not quote the whole value")
			assert.Contains(t, err.Error(), fmt.Sprintf("... (%d bytes)", len(tt.src)), "error names the length of the value")
		})
	}

	// Short values are quoted completely
	err := ScanString("[1,,2]", new([]int))
	assert.EqualError(t, err, `slice value "[1,,2]" has an empty element`)
}

func TestScanString_uintptr(t *testing.T) {
	var u uintptr
	require.NoError(t, ScanString(" 42 ", &u))
	assert.Equal(t, uintptr(42), u)
	assert.EqualError(t, ScanString("-1", &u), `can't parse "-1" as uint because of: invalid syntax`)
}

func Test_quoteForError(t *testing.T) {
	assert.Equal(t, `"abc"`, quoteForError("abc"))
	assert.Equal(t, strconv.Quote(strings.Repeat("a", maxErrorValueLen)), quoteForError(strings.Repeat("a", maxErrorValueLen)))
	// A multi-byte character at the limit is not cut into invalid bytes
	long := strings.Repeat("a", maxErrorValueLen-1) + "ä" + "b"
	assert.Equal(t, `"`+strings.Repeat("a", maxErrorValueLen-1)+`"... (102 bytes)`, quoteForError(long))
	// Invalid UTF-8 of continuation bytes still quotes a prefix
	invalid := strings.Repeat("\x80", 200)
	assert.Equal(t, strconv.Quote(invalid[:maxErrorValueLen-utf8.UTFMax+1])+"... (200 bytes)", quoteForError(invalid))
}

// settableTime has a Set(time.Time) method like nullable.Time of go-types
// and an UnmarshalText method that only accepts one format
type settableTime struct {
	t   time.Time
	set bool
}

func (s *settableTime) Set(t time.Time) { s.t, s.set = t, true }

func (s *settableTime) UnmarshalText(text []byte) error {
	return errors.New("UnmarshalText must not be called")
}

func TestScanString_SetTime(t *testing.T) {
	// A type with a Set(time.Time) method gets the time parsed like time.Time,
	// as documented also if it implements encoding.TextUnmarshaler
	var dest settableTime
	require.NoError(t, ScanString("2026-01-02 15:04", &dest))
	assert.True(t, dest.set)
	assert.Equal(t, time.Date(2026, 1, 2, 15, 4, 0, 0, TimeLocation), dest.t)

	dest = settableTime{}
	require.NoError(t, ScanString("null", &dest))
	assert.True(t, dest.set)
	assert.True(t, dest.t.IsZero())

	assert.Error(t, ScanString("not a time", &dest))
}

func TestScanString_datetimeLocalWithSeconds(t *testing.T) {
	// Browsers submit datetime-local values with seconds if the value has seconds
	var got time.Time
	require.NoError(t, ScanString("2026-10-20T14:30:05", &got))
	assert.Equal(t, time.Date(2026, 10, 20, 14, 30, 5, 0, TimeLocation), got)
	require.NoError(t, ScanString("2026-10-20T14:30", &got))
	assert.Equal(t, time.Date(2026, 10, 20, 14, 30, 0, 0, TimeLocation), got)
}
