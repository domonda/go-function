package function

import (
	"errors"
	"reflect"
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

func (s *TypeStringScanners) ScanString(sourceStr string, destPtr interface{}) error {
	if destPtr == nil {
		return errors.New("destination pointer is nil")
	}
	if s == nil {
		return ErrTypeNotSupported
	}
	destType := reflect.ValueOf(destPtr).Type().Elem()
	if typeScanner, ok := s.Types[destType]; ok {
		err := typeScanner.ScanString(sourceStr, destPtr)
		if !errors.Is(err, ErrTypeNotSupported) {
			return err
		}
	}
	for interfaceType, interfaceScanner := range s.Interfaces {
		if destType.Implements(interfaceType) {
			err := interfaceScanner.ScanString(sourceStr, destPtr)
			if !errors.Is(err, ErrTypeNotSupported) {
				return err
			}
		}
	}
	if kindScanner, ok := s.Kinds[destType.Kind()]; ok {
		return kindScanner.ScanString(sourceStr, destPtr)
	}
	if s.Default != nil {
		return s.Default.ScanString(sourceStr, destPtr)
	}
	return ErrTypeNotSupported
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
	c := &TypeStringScanners{Default: s.Default}
	if len(s.Types) > 0 {
		c.Types = make(map[reflect.Type]StringScanner, len(s.Types))
		for key, val := range s.Types {
			c.Types[key] = val
		}
	}
	if len(s.Interfaces) > 0 {
		c.Interfaces = make(map[reflect.Type]StringScanner, len(s.Interfaces))
		for key, val := range s.Interfaces {
			c.Interfaces[key] = val
		}
	}
	if len(s.Kinds) > 0 {
		c.Kinds = make(map[reflect.Kind]StringScanner, len(s.Kinds))
		for key, val := range s.Kinds {
			c.Kinds[key] = val
		}
	}
	return c
}
