package function

import (
	"reflect"
	"testing"
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
	intMap := make(map[int]int)
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
			args:     args{sourceStr: "666", destPtr: new(int)},
			wantDest: int(666),
		},
		{
			name:     "empty string map[int]int",
			args:     args{sourceStr: "", destPtr: &intMap},
			wantDest: map[int]int(nil),
		},
		{
			name:     "nil map[int]int",
			args:     args{sourceStr: "nil", destPtr: &intMap},
			wantDest: map[int]int(nil),
		},
		{
			name:     "null map[int]int",
			args:     args{sourceStr: "null", destPtr: &intMap},
			wantDest: map[int]int(nil),
		},
		// wantErr
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
