package gen

import "testing"

// TestImplFromString_RoundTrip tests that every interface name written
// by Impl.String into the comments of generated code and every name
// of a function.*TODO function is parsed back to the same Impl,
// because a name that can't be parsed makes the generator skip the wrapper.
func TestImplFromString_RoundTrip(t *testing.T) {
	for _, impl := range []Impl{
		ImplDescription,
		ImplCallWrapper,
		ImplCallWithStringsWrapper,
		ImplCallWithNamedStringsWrapper,
		ImplCallWithJSONWrapper,
		ImplWrapper,
	} {
		t.Run(impl.String(), func(t *testing.T) {
			got, err := ImplFromString(impl.String())
			if err != nil {
				t.Fatalf("ImplFromString(%q) error: %s", impl, err)
			}
			if got != impl {
				t.Errorf("ImplFromString(%q) = %s, want %s", impl, got, impl)
			}
		})
	}
}

// Test_remapPackageQualifiers tests that only package qualifiers are renamed,
// because a substring replacement would also rename other packages
// whose name ends with the renamed one, and the contents of struct tags.
func Test_remapPackageQualifiers(t *testing.T) {
	remap := map[string]string{"token": "token2"}
	tests := map[string]string{
		"token.Pos":                       "token2.Pos",
		"...token.Pos":                    "...token2.Pos",
		"[]*token.Pos":                    "[]*token2.Pos",
		"map[token.Pos]mytoken.Pos":       "map[token2.Pos]mytoken.Pos",
		"mytoken.Pos":                     "mytoken.Pos",
		"func(token.Pos) token.Token":     "func(token2.Pos) token2.Token",
		"struct{ token.Pos \"token.x\" }": "struct{ token2.Pos \"token.x\" }",
	}
	for typeName, want := range tests {
		t.Run(typeName, func(t *testing.T) {
			if got := remapPackageQualifiers(typeName, remap); got != want {
				t.Errorf("remapPackageQualifiers(%q) = %q, want %q", typeName, got, want)
			}
		})
	}
}

// Test_typeAssertionIsAssignability tests for which argument types
// the generated Call omits the reflection that accepts assignable values
// of other types, because omitting it for a type that a value of another
// type is assignable to makes Call reject arguments that ReflectWrapper accepts,
// while generating it for every type bloats the generated code.
func Test_typeAssertionIsAssignability(t *testing.T) {
	tests := map[string]bool{
		// Only values of the predeclared type itself are assignable
		"int":    true,
		"string": true,
		"byte":   true,
		// Values implementing the interface are assignable and pass the type assertion
		"error":            true,
		"interface{ M() }": true,
		"interface{ any }": true,
		"MyInts":           false, // type MyInts []int is assignable from []int
		"time.Duration":    false, // could be an interface or have a type literal as underlying type
		"io.Reader":        false,
		"[]int":            false, // assignable from MyInts
		"*int":             false, // assignable from type IntPtr *int
		"map[string]int":   false,
		"func()":           false,
		"<-chan int":       false, // assignable from chan int
		"struct{ X int }":  false,
		"[2]int":           false,
		"List[int]":        false,
		"not a type ][":    false,
	}
	for typeName, want := range tests {
		t.Run(typeName, func(t *testing.T) {
			if got := typeAssertionIsAssignability(typeName); got != want {
				t.Errorf("typeAssertionIsAssignability(%q) = %t, want %t", typeName, got, want)
			}
		})
	}
}
