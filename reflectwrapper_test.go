package function

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestReflectWrapper(t *testing.T) {
	type description struct {
		NumArgs     int
		ContextArg  bool
		NumResults  int
		ErrorResult bool
		ArgNames    []string
		ArgTypes    []reflect.Type
		ResultTypes []reflect.Type
	}

	f0 := func() {}
	f0desc := description{
		NumArgs:     0,
		ContextArg:  false,
		NumResults:  0,
		ErrorResult: false,
		ArgNames:    nil,
		ArgTypes:    nil,
		ResultTypes: nil,
	}

	f1 := func(i int) {}
	f1desc := description{
		NumArgs:     1,
		ContextArg:  false,
		NumResults:  0,
		ErrorResult: false,
		ArgNames:    []string{"i"},
		ArgTypes:    []reflect.Type{reflect.TypeOf(0)},
		ResultTypes: nil,
	}

	f1r := func(i int) int { return i * 2 }
	f1rdesc := description{
		NumArgs:     1,
		ContextArg:  false,
		NumResults:  1,
		ErrorResult: false,
		ArgNames:    []string{"i"},
		ArgTypes:    []reflect.Type{reflect.TypeOf(0)},
		ResultTypes: []reflect.Type{reflect.TypeOf(0)},
	}

	ferr := func(i int, e error) (int, error) { return i * 2, e }
	ferrdesc := description{
		NumArgs:     2,
		ContextArg:  false,
		NumResults:  2,
		ErrorResult: true,
		ArgNames:    []string{"i", "e"},
		ArgTypes:    []reflect.Type{reflect.TypeOf(0), typeOfError},
		ResultTypes: []reflect.Type{reflect.TypeOf(0), typeOfError},
	}

	type args struct {
		function interface{}
		argNames []string
	}
	type call struct {
		args         []interface{}
		argsStrings  []string
		argsNamedStr map[string]string
		argsJSON     []byte
		results      []interface{}
		wantErr      bool
	}
	tests := []struct {
		name    string
		args    args
		want    *reflectWrapper
		wantErr bool
		call    call
		desc    description
	}{
		{
			name: "func() {}",
			args: args{
				function: f0,
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(f0),
				funcType: reflect.TypeOf(f0),
				argNames: nil,
			},
			wantErr: false,
			call: call{
				args:         nil,
				argsStrings:  nil,
				argsNamedStr: nil,
				argsJSON:     []byte(`{}`),
				results:      []interface{}{},
				wantErr:      false,
			},
			desc: f0desc,
		},
		{
			name: "func(i int) {}",
			args: args{
				function: f1,
				argNames: []string{"i"},
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(f1),
				funcType: reflect.TypeOf(f1),
				argNames: []string{"i"},
			},
			wantErr: false,
			call: call{
				args:         []interface{}{666},
				argsStrings:  []string{"666"},
				argsNamedStr: map[string]string{"i": "666"},
				argsJSON:     []byte(`{"i":666}`),
				results:      []interface{}{},
				wantErr:      false,
			},
			desc: f1desc,
		},
		{
			name: "func(i int) int",
			args: args{
				function: f1r,
				argNames: []string{"i"},
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(f1r),
				funcType: reflect.TypeOf(f1r),
				argNames: []string{"i"},
			},
			wantErr: false,
			call: call{
				args:         []interface{}{666},
				argsStrings:  []string{"666"},
				argsNamedStr: map[string]string{"i": "666"},
				argsJSON:     []byte(`{"i":666}`),
				results:      []interface{}{666 * 2},
				wantErr:      false,
			},
			desc: f1rdesc,
		},
		{
			name: "func(i int, e error = nil) (int, error)",
			args: args{
				function: ferr,
				argNames: []string{"i", "e"},
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(ferr),
				funcType: reflect.TypeOf(ferr),
				argNames: []string{"i", "e"},
			},
			wantErr: false,
			call: call{
				args:         []interface{}{666, nil},
				argsStrings:  []string{"666", ""},
				argsNamedStr: map[string]string{"i": "666", "e": ""},
				argsJSON:     []byte(`{"i":666,"e":null}`),
				results:      []interface{}{666 * 2},
				wantErr:      false,
			},
			desc: ferrdesc,
		},
		{
			name: "func(i int, e error = ERROR) (int, error)",
			args: args{
				function: ferr,
				argNames: []string{"i", "e"},
			},
			want: &reflectWrapper{
				funcVal:  reflect.ValueOf(ferr),
				funcType: reflect.TypeOf(ferr),
				argNames: []string{"i", "e"},
			},
			wantErr: false,
			call: call{
				args:         []interface{}{666, errors.New("ERROR")},
				argsStrings:  []string{"666", "ERROR"},
				argsNamedStr: map[string]string{"i": "666", "e": "ERROR"},
				argsJSON:     []byte(`{"i":666,"e":"ERROR"}`),
				results:      []interface{}{666 * 2},
				wantErr:      true,
			},
			desc: ferrdesc,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newReflectWrapper(tt.args.function, tt.args.argNames)
			if (err != nil) != tt.wantErr {
				t.Errorf("newReflectWrapper() error = %#v, wantErr = %#v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newReflectWrapper() = %#v, want = %#v", got, tt.want)
			}

			if got.NumArgs() != tt.desc.NumArgs {
				t.Errorf("NumArgs(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.NumArgs(), tt.desc.NumArgs)
			}
			if got.ContextArg() != tt.desc.ContextArg {
				t.Errorf("ContextArg(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ContextArg(), tt.desc.ContextArg)
			}
			if got.NumResults() != tt.desc.NumResults {
				t.Errorf("NumResults(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.NumResults(), tt.desc.NumResults)
			}
			if got.ErrorResult() != tt.desc.ErrorResult {
				t.Errorf("ErrorResult(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ErrorResult(), tt.desc.ErrorResult)
			}
			if !reflect.DeepEqual(got.ArgNames(), tt.desc.ArgNames) {
				t.Errorf("ArgNames(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ArgNames(), tt.desc.ArgNames)
			}
			if !reflect.DeepEqual(got.ArgTypes(), tt.desc.ArgTypes) {
				t.Errorf("ArgTypes(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ArgTypes(), tt.desc.ArgTypes)
			}
			if !reflect.DeepEqual(got.ResultTypes(), tt.desc.ResultTypes) {
				t.Errorf("ResultTypes(%s) = %#v, want = %#v", reflect.TypeOf(tt.args.function), got.ResultTypes(), tt.desc.ResultTypes)
			}

			gotResults, gotErr := got.Call(context.Background(), tt.call.args)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.Call() error = %#v, call.wantErr = %#v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.Call() = %#v, want %#v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithStrings(context.Background(), tt.call.argsStrings...)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithStrings() error = %#v, call.wantErr = %#v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithStrings() = %#v, want %#v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithNamedStrings(context.Background(), tt.call.argsNamedStr)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithNamedStrings() error = %#v, call.wantErr = %#v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithNamedStrings() = %#v, want %#v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithJSON(context.Background(), tt.call.argsJSON)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithJSON() error = %#v, call.wantErr = %#v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithJSON() = %#v, want %#v", gotResults, tt.call.results)
			}
		})
	}
}
