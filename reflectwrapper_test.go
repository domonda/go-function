package function

import (
	"context"
	"reflect"
	"testing"
)

func TestReflectWrapper(t *testing.T) {
	f0 := func() {}
	f1 := func(i int) {}
	f1r := func(i int) int { return i * 2 }

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
		},
		// TODO test errors
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newReflectWrapper(tt.args.function, tt.args.argNames)
			if (err != nil) != tt.wantErr {
				t.Errorf("newReflectWrapper() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newReflectWrapper() = %v, want %v", got, tt.want)
			}

			gotResults, gotErr := got.Call(context.Background(), tt.call.args)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.Call() error = %v, call.wantErr %v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.Call() = %v, want %v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithStrings(context.Background(), tt.call.argsStrings...)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithStrings() error = %v, call.wantErr %v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithStrings() = %v, want %v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithNamedStrings(context.Background(), tt.call.argsNamedStr)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithNamedStrings() error = %v, call.wantErr %v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithNamedStrings() = %v, want %v", gotResults, tt.call.results)
			}

			gotResults, gotErr = got.CallWithJSON(context.Background(), tt.call.argsJSON)
			if (gotErr != nil) != tt.call.wantErr {
				t.Errorf("reflectWrapper.CallWithJSON() error = %v, call.wantErr %v", gotErr, tt.call.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResults, tt.call.results) {
				t.Errorf("reflectWrapper.CallWithJSON() = %v, want %v", gotResults, tt.call.results)
			}
		})
	}
}
