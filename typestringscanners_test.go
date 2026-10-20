package function

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypeStringScanners_ScanString_InvalidDestPtr(t *testing.T) {
	// Registered scanners rely on getting a non-nil pointer,
	// so invalid destinations must be returned as error
	// before any scanner is called instead of panicking.
	scanners := NewTypeStringScanners(StringScannerFunc(DefaultScanString)).
		WithTypeScanner(reflect.TypeFor[int](), StringScannerFunc(func(sourceStr string, destPtr any) error {
			*destPtr.(*int) = 1 // panics for a nil pointer
			return nil
		}))
	tests := []struct {
		name    string
		destPtr any
	}{
		{name: "untyped nil", destPtr: nil},
		{name: "non-pointer", destPtr: 5},
		{name: "typed nil pointer", destPtr: (*int)(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() { err = scanners.ScanString("1", tt.destPtr) })
			assert.Error(t, err)
		})
	}
}

// ptrReceiverTextUnmarshaler implements encoding.TextUnmarshaler
// with a pointer receiver like setter methods usually do
type ptrReceiverTextUnmarshaler struct{ text string }

func (p *ptrReceiverTextUnmarshaler) UnmarshalText(text []byte) error {
	p.text = string(text)
	return nil
}

// valueReceiverStringer implements fmt.Stringer with a value receiver
type valueReceiverStringer int

func (v valueReceiverStringer) String() string { return strconv.Itoa(int(v)) }

func TestTypeStringScanners_ScanString_InterfaceScanner(t *testing.T) {
	// Interface scanners are matched against the destination type,
	// the scanner gets the destination pointer which implements the interface
	// for value and pointer receiver methods.
	// Setter interfaces like encoding.TextUnmarshaler are typically implemented
	// with pointer receivers, so they must match too.
	// Interface typed destinations like error must keep matching.
	scanners := NewTypeStringScanners(StringScannerFunc(DefaultScanString)).
		WithInterfaceTypeScanner(reflect.TypeFor[encoding.TextUnmarshaler](), StringScannerFunc(func(sourceStr string, destPtr any) error {
			return destPtr.(encoding.TextUnmarshaler).UnmarshalText([]byte("scanned " + sourceStr))
		})).
		WithInterfaceTypeScanner(reflect.TypeFor[fmt.Stringer](), StringScannerFunc(func(sourceStr string, destPtr any) error {
			*destPtr.(*valueReceiverStringer) = 42
			return nil
		})).
		WithInterfaceTypeScanner(reflect.TypeFor[error](), StringScannerFunc(func(sourceStr string, destPtr any) error {
			*destPtr.(*error) = errors.New("scanned " + sourceStr)
			return nil
		}))

	t.Run("pointer receiver", func(t *testing.T) {
		var dest ptrReceiverTextUnmarshaler
		err := scanners.ScanString("x", &dest)
		require.NoError(t, err)
		assert.Equal(t, "scanned x", dest.text)
	})

	t.Run("value receiver", func(t *testing.T) {
		var dest valueReceiverStringer
		err := scanners.ScanString("1", &dest)
		require.NoError(t, err)
		assert.Equal(t, valueReceiverStringer(42), dest)
	})

	t.Run("interface type destination", func(t *testing.T) {
		var dest error
		err := scanners.ScanString("x", &dest)
		require.NoError(t, err)
		assert.EqualError(t, dest, "scanned x")
	})
}

func TestTypeStringScanners_ScanString_KindScannerFallback(t *testing.T) {
	// Like type and interface scanners, a kind scanner returning
	// ErrTypeNotSupported must fall back to the default scanner
	// so that kind scanners can handle only some types of a kind.
	scanners := NewTypeStringScanners(StringScannerFunc(DefaultScanString)).
		WithKindScanner(reflect.Int, StringScannerFunc(func(sourceStr string, destPtr any) error {
			return fmt.Errorf("wrapped: %w", ErrTypeNotSupported)
		}))

	var dest int
	err := scanners.ScanString("42", &dest)
	require.NoError(t, err)
	assert.Equal(t, 42, dest)

	t.Run("kind scanner error", func(t *testing.T) {
		// A kind scanner that rejects input, for example to enforce a range,
		// must not be bypassed by the default scanner accepting it
		errRejected := errors.New("rejected by kind scanner")
		scanners := NewTypeStringScanners(StringScannerFunc(func(sourceStr string, destPtr any) error {
			t.Error("default scanner must not be called")
			return DefaultScanString(sourceStr, destPtr)
		})).
			WithKindScanner(reflect.Int, StringScannerFunc(func(sourceStr string, destPtr any) error {
				return errRejected
			}))

		dest := -1
		err := scanners.ScanString("42", &dest)
		assert.ErrorIs(t, err, errRejected)
		assert.Equal(t, -1, dest, "destination must not be changed")
	})
}

type scannerTestA interface{ ScannerTestA() }
type scannerTestB interface{ ScannerTestB() }

// bothValueInterfaces implements scannerTestA and scannerTestB with value receivers
type bothValueInterfaces string

func (bothValueInterfaces) ScannerTestA() {}
func (bothValueInterfaces) ScannerTestB() {}

// valueAndPointerInterfaces implements scannerTestB with a value receiver
// and scannerTestA only with a pointer receiver
type valueAndPointerInterfaces string

func (valueAndPointerInterfaces) ScannerTestB()  {}
func (*valueAndPointerInterfaces) ScannerTestA() {}

func TestTypeStringScanners_ScanString_InterfaceScannerOrder(t *testing.T) {
	// When several registered interface scanners match a destination type,
	// the same input must always be scanned the same way.
	// Ranging over the Interfaces map chose a random scanner per call.
	scanTo := func(name string) StringScanner {
		return StringScannerFunc(func(sourceStr string, destPtr any) error {
			reflect.ValueOf(destPtr).Elem().SetString(name)
			return nil
		})
	}
	scanners := NewTypeStringScanners(nil).
		WithInterfaceTypeScanner(reflect.TypeFor[scannerTestB](), scanTo("B")).
		WithInterfaceTypeScanner(reflect.TypeFor[scannerTestA](), scanTo("A"))

	for range 100 {
		var both bothValueInterfaces
		err := scanners.ScanString("x", &both)
		require.NoError(t, err)
		require.Equal(t, bothValueInterfaces("A"), both, "first interface sorted by type name wins")

		var valueAndPtr valueAndPointerInterfaces
		err = scanners.ScanString("x", &valueAndPtr)
		require.NoError(t, err)
		require.Equal(t, valueAndPointerInterfaces("B"), valueAndPtr, "interface implemented by the type itself wins over pointer type")
	}
}

func TestTypeStringScanners_ScanString_InterfaceScannerFallthrough(t *testing.T) {
	// An interface scanner returning ErrTypeNotSupported passes the destination
	// to the next matching interface scanner in the documented order:
	// the interfaces of the type, then the ones of its pointer type,
	// then the kind scanner
	notSupported := StringScannerFunc(func(string, any) error {
		return fmt.Errorf("wrapped: %w", ErrTypeNotSupported)
	})
	scanTo := func(name string) StringScanner {
		return StringScannerFunc(func(_ string, destPtr any) error {
			reflect.ValueOf(destPtr).Elem().SetString(name)
			return nil
		})
	}

	t.Run("next interface of the type", func(t *testing.T) {
		scanners := NewTypeStringScanners(nil).
			WithInterfaceTypeScanner(reflect.TypeFor[scannerTestA](), notSupported).
			WithInterfaceTypeScanner(reflect.TypeFor[scannerTestB](), scanTo("B"))
		var both bothValueInterfaces
		require.NoError(t, scanners.ScanString("x", &both))
		assert.Equal(t, bothValueInterfaces("B"), both)
	})

	t.Run("interface of the pointer type", func(t *testing.T) {
		scanners := NewTypeStringScanners(nil).
			WithInterfaceTypeScanner(reflect.TypeFor[scannerTestB](), notSupported).
			WithInterfaceTypeScanner(reflect.TypeFor[scannerTestA](), scanTo("A"))
		var valueAndPtr valueAndPointerInterfaces
		require.NoError(t, scanners.ScanString("x", &valueAndPtr))
		assert.Equal(t, valueAndPointerInterfaces("A"), valueAndPtr)
	})

	t.Run("kind scanner", func(t *testing.T) {
		scanners := NewTypeStringScanners(nil).
			WithInterfaceTypeScanner(reflect.TypeFor[scannerTestA](), notSupported).
			WithInterfaceTypeScanner(reflect.TypeFor[scannerTestB](), notSupported).
			WithKindScanner(reflect.String, scanTo("kind"))
		var both bothValueInterfaces
		require.NoError(t, scanners.ScanString("x", &both))
		assert.Equal(t, bothValueInterfaces("kind"), both)
	})
}

func TestTypeStringScanners_With_DoesNotModifyReceiver(t *testing.T) {
	// The global StringScanners is read concurrently and changed
	// by assigning the result of a With method to it,
	// so With methods must return a copy and not modify the receiver's maps.
	scanner := StringScannerFunc(DefaultScanString)
	base := (*TypeStringScanners)(nil).
		WithTypeScanner(reflect.TypeFor[int](), scanner).
		WithInterfaceTypeScanner(reflect.TypeFor[error](), scanner).
		WithKindScanner(reflect.Int, scanner)

	mod := base.
		WithTypeScanner(reflect.TypeFor[string](), scanner).
		WithInterfaceTypeScanner(reflect.TypeFor[fmt.Stringer](), scanner).
		WithKindScanner(reflect.String, scanner).
		WithDefaultScanner(scanner)

	assert.Len(t, base.types, 1)
	assert.Len(t, base.interfaces, 1)
	assert.Len(t, base.kinds, 1)
	assert.Nil(t, base.defaultScanner)
	assert.Len(t, mod.types, 2)
	assert.Len(t, mod.interfaces, 2)
	assert.Len(t, mod.kinds, 2)
	assert.NotNil(t, mod.defaultScanner)
}

func TestTypeStringScanners_With_PanicsForInvalidScanners(t *testing.T) {
	// An invalid registration must fail where it is made,
	// because a non-interface type would make every later ScanString
	// panic in reflect.Type.Implements and a nil scanner would panic
	// for every destination it matches
	scanner := StringScannerFunc(DefaultScanString)
	s := NewTypeStringScanners(scanner)
	assert.PanicsWithValue(t, "TypeStringScanners.WithInterfaceTypeScanner(int): not an interface type", func() {
		s.WithInterfaceTypeScanner(reflect.TypeFor[int](), scanner)
	})
	assert.Panics(t, func() { s.WithInterfaceTypeScanner(nil, scanner) })
	assert.Panics(t, func() { s.WithInterfaceTypeScanner(reflect.TypeFor[fmt.Stringer](), nil) })
	assert.Panics(t, func() { s.WithTypeScanner(nil, scanner) })
	assert.Panics(t, func() { s.WithTypeScanner(reflect.TypeFor[int](), nil) })
	assert.Panics(t, func() { s.WithKindScanner(reflect.Int, nil) })
	assert.NotPanics(t, func() { s.WithDefaultScanner(nil) }, "nil default scanner returns ErrTypeNotSupported")

	var x string
	assert.ErrorIs(t, s.WithDefaultScanner(nil).ScanString("x", &x), ErrTypeNotSupported)
}
