package function

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

var _ StringScanner = new(TypeStringScanners)

// TypeStringScanners is a StringScanner that selects the scanner
// for a destination by its type, the interfaces it implements, or its kind,
// with a default scanner for all other destinations, see ScanString.
//
// A TypeStringScanners is not changed after its creation:
// the With methods return a modified copy, so a TypeStringScanners
// can be used concurrently and passed around without being changed.
// Configure the package-level StringScanners used by ScanString
// by assigning a copy before scanning strings, for example in an init function:
//
//	function.StringScanners = function.StringScanners.WithTypeScanner(
//	    reflect.TypeFor[MyType](),
//	    function.StringScannerFunc(scanMyType),
//	)
//
// A nil *TypeStringScanners has no scanners
// and returns ErrTypeNotSupported for all destinations.
type TypeStringScanners struct {
	types          map[reflect.Type]StringScanner
	interfaces     map[reflect.Type]StringScanner
	kinds          map[reflect.Kind]StringScanner
	defaultScanner StringScanner
}

// NewTypeStringScanners returns a TypeStringScanners without
// type, interface, or kind scanners that uses defaultScanner
// for all destinations. A nil defaultScanner returns ErrTypeNotSupported.
func NewTypeStringScanners(defaultScanner StringScanner) *TypeStringScanners {
	return &TypeStringScanners{defaultScanner: defaultScanner}
}

// ScanString scans sourceStr into destPtr using the first registered scanner
// matching the destination type in the following order. A scanner returning
// an error wrapping ErrTypeNotSupported falls through to the next matching one.
//  1. Type scanner for the destination type
//  2. Interface scanners for interfaces implemented by the destination type,
//     then for interfaces only implemented by its pointer type
//     (like encoding.TextUnmarshaler with a pointer receiver).
//     Within both groups the interfaces are sorted by their
//     reflect.Type.String() and PkgPath(), so the first match in that order
//     wins and the result does not depend on the map iteration order.
//  3. Kind scanner for the kind of the destination type
//  4. Default scanner
func (s *TypeStringScanners) ScanString(sourceStr string, destPtr any) error {
	if destPtr == nil {
		return errors.New("destination pointer is nil")
	}
	if s == nil {
		return ErrTypeNotSupported
	}
	destPtrVal := reflect.ValueOf(destPtr)
	if destPtrVal.Kind() != reflect.Pointer {
		return fmt.Errorf("expected destination pointer type but got: %s", destPtrVal.Type())
	}
	if destPtrVal.IsNil() {
		return errors.New("destination pointer is nil")
	}
	destType := destPtrVal.Type().Elem()
	if typeScanner, ok := s.types[destType]; ok {
		err := typeScanner.ScanString(sourceStr, destPtr)
		if !errors.Is(err, ErrTypeNotSupported) {
			return err
		}
	}
	if len(s.interfaces) > 0 {
		// Filter before sorting to not allocate if no interface matches
		var implementedByType, implementedByPtr []reflect.Type
		for interfaceType := range s.interfaces {
			switch {
			case destType.Implements(interfaceType):
				implementedByType = append(implementedByType, interfaceType)
			case destPtrVal.Type().Implements(interfaceType):
				// Setter interfaces like encoding.TextUnmarshaler are usually
				// implemented with pointer receivers, so also check the pointer type.
				// The scanner gets destPtr which implements the interface in both cases.
				implementedByPtr = append(implementedByPtr, interfaceType)
			}
		}
		// Sort to not depend on the random map iteration order
		// if multiple interfaces match
		for _, interfaceTypes := range [2][]reflect.Type{implementedByType, implementedByPtr} {
			if len(interfaceTypes) > 1 {
				slices.SortFunc(interfaceTypes, compareInterfaceTypes)
			}
			for _, interfaceType := range interfaceTypes {
				err := s.interfaces[interfaceType].ScanString(sourceStr, destPtr)
				if !errors.Is(err, ErrTypeNotSupported) {
					return err
				}
			}
		}
	}
	if kindScanner, ok := s.kinds[destType.Kind()]; ok {
		err := kindScanner.ScanString(sourceStr, destPtr)
		if !errors.Is(err, ErrTypeNotSupported) {
			return err
		}
	}
	if s.defaultScanner != nil {
		return s.defaultScanner.ScanString(sourceStr, destPtr)
	}
	return ErrTypeNotSupported
}

// compareInterfaceTypes orders interface types
// by their reflect.Type.String() and PkgPath().
func compareInterfaceTypes(a, b reflect.Type) int {
	return cmp.Or(
		strings.Compare(a.String(), b.String()),
		strings.Compare(a.PkgPath(), b.PkgPath()),
	)
}

// WithTypeScanner returns a copy of s that scans destinations
// of exactly the type destType with scanner.
// It panics if destType or scanner is nil.
func (s *TypeStringScanners) WithTypeScanner(destType reflect.Type, scanner StringScanner) *TypeStringScanners {
	if destType == nil {
		panic("TypeStringScanners.WithTypeScanner: nil destType")
	}
	if scanner == nil {
		panic(fmt.Sprintf("TypeStringScanners.WithTypeScanner(%s): nil scanner", destType))
	}
	mod := s.cloneOrNew()
	if mod.types == nil {
		mod.types = make(map[reflect.Type]StringScanner)
	}
	mod.types[destType] = scanner
	return mod
}

// WithInterfaceTypeScanner returns a copy of s that scans destinations
// whose type or pointer type implements the interface type
// destImplsInterface with scanner, which gets a pointer to the destination.
// It panics if destImplsInterface is not an interface type or scanner is nil.
func (s *TypeStringScanners) WithInterfaceTypeScanner(destImplsInterface reflect.Type, scanner StringScanner) *TypeStringScanners {
	if destImplsInterface == nil || destImplsInterface.Kind() != reflect.Interface {
		panic(fmt.Sprintf("TypeStringScanners.WithInterfaceTypeScanner(%s): not an interface type", destImplsInterface))
	}
	if scanner == nil {
		panic(fmt.Sprintf("TypeStringScanners.WithInterfaceTypeScanner(%s): nil scanner", destImplsInterface))
	}
	mod := s.cloneOrNew()
	if mod.interfaces == nil {
		mod.interfaces = make(map[reflect.Type]StringScanner)
	}
	mod.interfaces[destImplsInterface] = scanner
	return mod
}

// WithKindScanner returns a copy of s that scans destinations
// of the kind destKind with scanner, if no type or interface scanner matches.
// It panics if scanner is nil.
func (s *TypeStringScanners) WithKindScanner(destKind reflect.Kind, scanner StringScanner) *TypeStringScanners {
	if scanner == nil {
		panic(fmt.Sprintf("TypeStringScanners.WithKindScanner(%s): nil scanner", destKind))
	}
	mod := s.cloneOrNew()
	if mod.kinds == nil {
		mod.kinds = make(map[reflect.Kind]StringScanner)
	}
	mod.kinds[destKind] = scanner
	return mod
}

// WithDefaultScanner returns a copy of s that scans destinations
// with scanner if no other scanner matches.
// A nil scanner returns ErrTypeNotSupported for such destinations.
func (s *TypeStringScanners) WithDefaultScanner(scanner StringScanner) *TypeStringScanners {
	mod := s.cloneOrNew()
	mod.defaultScanner = scanner
	return mod
}

func (s *TypeStringScanners) cloneOrNew() *TypeStringScanners {
	if s == nil {
		return new(TypeStringScanners)
	}
	return &TypeStringScanners{
		types:          maps.Clone(s.types),
		interfaces:     maps.Clone(s.interfaces),
		kinds:          maps.Clone(s.kinds),
		defaultScanner: s.defaultScanner,
	}
}
