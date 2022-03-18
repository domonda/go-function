package function

import (
	"context"
	"strings"
	"testing"
)

func TestPrintlnTo(t *testing.T) {
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
		// TODO more tests
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
