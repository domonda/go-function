package function

import (
	"context"
	"strings"
	"testing"
)

func TestPrintlnTo(t *testing.T) {
	type structA struct {
		A string
		B bool
	}

	type args struct {
		results   []any
		resultErr error
	}
	tests := []struct {
		name        string
		args        args
		wantErr     bool
		wantPrinted string
	}{
		{
			name: `string "Hello World!"`,
			args: args{
				results:   []any{"Hello World!"},
				resultErr: nil,
			},
			wantPrinted: `Hello World!` + "\n",
		},
		{
			name: `13 decimal places float64 0.0000000001234"`,
			args: args{
				results:   []any{float64(0.0000000001234)},
				resultErr: nil,
			},
			wantPrinted: `0.000000000123` + "\n",
		},
		{
			name: `8 zero bytes"`,
			args: args{
				results:   []any{make([]byte, 8)},
				resultErr: nil,
			},
			wantPrinted: `0x0000000000000000` + "\n",
		},
		{
			name: `struct"`,
			args: args{
				results:   []any{structA{A: "Hello World!", B: true}},
				resultErr: nil,
			},
			wantPrinted: `{
  "A": "Hello World!",
  "B": true
}` + "\n",
		},
		{
			name: `map"`,
			args: args{
				results:   []any{map[int]string{1: "A", 2: "B"}},
				resultErr: nil,
			},
			wantPrinted: `{
  "1": "A",
  "2": "B"
}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			err := PrintlnTo(&buf)(context.Background(), tt.args.results, tt.args.resultErr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Println() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if buf.String() != tt.wantPrinted {
				t.Errorf("Println() output:\n%s\nwant:\n%s", buf.String(), tt.wantPrinted)
			}
		})
	}
}
