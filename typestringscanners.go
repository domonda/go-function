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

type TypeStringScanners struct {
	Types      map[reflect.Type]StringScanner
	Interfaces map[reflect.Type]StringScanner
	Kinds      map[reflect.Kind]StringScanner
	Default    StringScanner
}

func NewTypeStringScanners(defaultScanner StringScanner) *TypeStringScanners {
	return &TypeStringScanners{Default: defaultScanner}
}

// ScanString scans sourceStr into destPtr using the first registered scanner
// matching the destination type in the following order. A scanner returning
// an error wrapping ErrTypeNotSupported falls through to the next matching one.
//  1. Types scanner for the destination type
//  2. Interfaces scanners for interfaces implemented by the destination type,
//     then for interfaces only implemented by its pointer type
//     (like encoding.TextUnmarshaler with a pointer receiver).
//     Within both groups the interfaces are sorted by their
//     reflect.Type.String() and PkgPath(), so the first match in that order
//     wins and the result does not depend on the map iteration order.
//  3. Kinds scanner for the kind of the destination type
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
	if typeScanner, ok := s.Types[destType]; ok {
		err := typeScanner.ScanString(sourceStr, destPtr)
		if !errors.Is(err, ErrTypeNotSupported) {
			return err
		}
	}
	if len(s.Interfaces) > 0 {
		// Filter before sorting to not allocate if no interface matches
		var implementedByType, implementedByPtr []reflect.Type
		for interfaceType := range s.Interfaces {
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
				err := s.Interfaces[interfaceType].ScanString(sourceStr, destPtr)
				if !errors.Is(err, ErrTypeNotSupported) {
					return err
				}
			}
		}
	}
	if kindScanner, ok := s.Kinds[destType.Kind()]; ok {
		err := kindScanner.ScanString(sourceStr, destPtr)
		if !errors.Is(err, ErrTypeNotSupported) {
			return err
		}
	}
	if s.Default != nil {
		return s.Default.ScanString(sourceStr, destPtr)
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

func (s *TypeStringScanners) WithTypeScanner(destType reflect.Type, scanner StringScanner) *TypeStringScanners {
	mod := s.cloneOrNew()
	if mod.Types == nil {
		mod.Types = make(map[reflect.Type]StringScanner)
	}
	mod.Types[destType] = scanner
	return mod
}

func (s *TypeStringScanners) WithInterfaceTypeScanner(destImplsInterface reflect.Type, scanner StringScanner) *TypeStringScanners {
	mod := s.cloneOrNew()
	if mod.Interfaces == nil {
		mod.Interfaces = make(map[reflect.Type]StringScanner)
	}
	mod.Interfaces[destImplsInterface] = scanner
	return mod
}

func (s *TypeStringScanners) WithKindScanner(destKind reflect.Kind, scanner StringScanner) *TypeStringScanners {
	mod := s.cloneOrNew()
	if mod.Kinds == nil {
		mod.Kinds = make(map[reflect.Kind]StringScanner)
	}
	mod.Kinds[destKind] = scanner
	return mod
}

func (s *TypeStringScanners) WithDefaultScanner(scanner StringScanner) *TypeStringScanners {
	mod := s.cloneOrNew()
	mod.Default = scanner
	return mod
}

func (s *TypeStringScanners) cloneOrNew() *TypeStringScanners {
	if s == nil {
		return new(TypeStringScanners)
	}
	return &TypeStringScanners{
		Types:      maps.Clone(s.Types),
		Interfaces: maps.Clone(s.Interfaces),
		Kinds:      maps.Clone(s.Kinds),
		Default:    s.Default,
	}
}
