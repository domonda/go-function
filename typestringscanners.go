package function

import (
	"errors"
	"reflect"
)

var _ StringScanner = new(TypeStringScanners)

type TypeStringScanners struct {
	Types          map[reflect.Type]StringScanner
	InterfaceTypes map[reflect.Type]StringScanner
	Kinds          map[reflect.Kind]StringScanner
	Default        StringScanner
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
	for interfaceType, interfaceScanner := range s.InterfaceTypes {
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

func (s *TypeStringScanners) WithTypeScanner(typ reflect.Type, scanner StringScanner) *TypeStringScanners {
	mod := s.cloneOrNew()
	if mod.Types == nil {
		mod.Types = make(map[reflect.Type]StringScanner)
	}
	mod.Types[typ] = scanner
	return mod
}

func (s *TypeStringScanners) WithInterfaceTypeScanner(typ reflect.Type, scanner StringScanner) *TypeStringScanners {
	mod := s.cloneOrNew()
	if mod.InterfaceTypes == nil {
		mod.InterfaceTypes = make(map[reflect.Type]StringScanner)
	}
	mod.InterfaceTypes[typ] = scanner
	return mod
}

func (s *TypeStringScanners) WithKindScanner(kind reflect.Kind, scanner StringScanner) *TypeStringScanners {
	mod := s.cloneOrNew()
	if mod.Kinds == nil {
		mod.Kinds = make(map[reflect.Kind]StringScanner)
	}
	mod.Kinds[kind] = scanner
	return mod
}

func (s *TypeStringScanners) WithOtherScanner(scanner StringScanner) *TypeStringScanners {
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
	if len(s.InterfaceTypes) > 0 {
		c.InterfaceTypes = make(map[reflect.Type]StringScanner, len(s.InterfaceTypes))
		for key, val := range s.InterfaceTypes {
			c.InterfaceTypes[key] = val
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
