package function

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
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

		// // wantErr
		{
			name:    "nil destPtr",
			args:    args{sourceStr: "nil", destPtr: nil},
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
			name:     "string slice",
			args:     args{sourceStr: `["a","b","c"]`, destVal: reflect.ValueOf(new([]string)).Elem()},
			wantDest: []string{`"a"`, `"b"`, `"c"`},
		},
		{
			name:     "empty slice",
			args:     args{sourceStr: "[]", destVal: reflect.ValueOf(new([]int)).Elem()},
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
			wantDest: [2]string{`"a"`, `"b"`},
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
			name:    "map type not supported via scanString",
			args:    args{sourceStr: "nil", destVal: reflect.ValueOf(new(map[string]int)).Elem()},
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
			err := scanString(tt.args.sourceStr, tt.args.destVal)
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
