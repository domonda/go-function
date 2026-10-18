package function

import (
	"bytes"
	"context"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			// Whole-number floats must not print with a dangling decimal point like "1."
			name: `whole-number float64 1`,
			args: args{
				results: []any{float64(1)},
			},
			wantPrinted: `1` + "\n",
		},
		{
			// Only the decimal point may be trimmed after the trailing zeros,
			// never the zeros of the integer part
			name: `whole-number float64 100`,
			args: args{
				results: []any{float64(100)},
			},
			wantPrinted: `100` + "\n",
		},
		{
			name: `float64 zero`,
			args: args{
				results: []any{float64(0)},
			},
			wantPrinted: `0` + "\n",
		},
		{
			name: `negative whole-number float64 -2`,
			args: args{
				results: []any{float64(-2)},
			},
			wantPrinted: `-2` + "\n",
		},
		{
			name: `whole-number float32 3`,
			args: args{
				results: []any{float32(3)},
			},
			wantPrinted: `3` + "\n",
		},
		{
			name: `float32 1.5`,
			args: args{
				results: []any{float32(1.5)},
			},
			wantPrinted: `1.5` + "\n",
		},
		{
			name: `float64 NaN`,
			args: args{
				results: []any{math.NaN()},
			},
			wantPrinted: `NaN` + "\n",
		},
		{
			name: `float64 +Inf`,
			args: args{
				results: []any{math.Inf(1)},
			},
			wantPrinted: `+Inf` + "\n",
		},
		{
			// Text with newlines or tabs is still human readable text
			// and must not be printed as hex
			name: `bytes with whitespace`,
			args: args{
				results: []any{[]byte("hello\n\tworld\r\n")},
			},
			wantPrinted: "hello\n\tworld\r\n" + "\n",
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

func TestLogTo(t *testing.T) {
	// Results are arbitrary text and must be logged verbatim,
	// a '%' in a result must not be interpreted as a Printf verb.
	results := func() []any { return []any{"100% done"} }

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	err := LogTo(logger)(context.Background(), results(), nil)
	require.NoError(t, err)
	assert.Equal(t, "100% done\n", buf.String())

	buf.Reset()
	err = LogWithPrefixTo("prefix:", logger)(context.Background(), results(), nil)
	require.NoError(t, err)
	assert.Equal(t, "prefix: 100% done\n", buf.String())
}

// captureStdout returns what f writes to os.Stdout
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	stdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = stdout }()
	f()
	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestPrintStructSliceAsTable(t *testing.T) {
	// Results can be any user type, so structs with unexported fields
	// and results that are not slices of structs must not panic:
	// unexported fields are left out of the table
	// and unsupported results are reported as error.
	type row struct {
		Name   string
		secret int
		Value  int
	}
	tests := []struct {
		name        string
		results     []any
		wantPrinted string
		wantErr     bool
	}{
		{
			name:        "unexported field is skipped",
			results:     []any{[]row{{Name: "a", secret: 1, Value: 2}, {Name: "bcdef", secret: 3, Value: 4}}},
			wantPrinted: "| Name  | Value |\n| a     | 2     |\n| bcdef | 4     |\n",
		},
		{
			name:        "unexported field is skipped for struct pointers",
			results:     []any{[]*row{{Name: "a", secret: 1, Value: 2}, nil}},
			wantPrinted: "| Name | Value |\n| a    | 2     |\n",
		},
		// wantErr
		{
			name:    "slice of non-struct pointers",
			results: []any{[]*int{new(int)}},
			wantErr: true,
		},
		{
			name:    "untyped nil result",
			results: []any{nil},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			printed := captureStdout(t, func() {
				require.NotPanics(t, func() {
					err = PrintStructSliceAsTable(context.Background(), tt.results, nil)
				})
			})
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPrinted, printed)
		})
	}
}
