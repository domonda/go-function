package gen

import (
	"go/ast"
	"strconv"
	"strings"

	"github.com/ungerik/go-astvisit"
)

// funcTypeArgNames extracts argument names from a function type AST node.
// Returns the names in order, as they appear in the function signature.
// Unnamed arguments get the names arg0, arg1, ... by their index.
//
// Example:
//
//	func(ctx context.Context, name string, age int) -> ["ctx", "name", "age"]
//	func(string, int) -> ["arg0", "arg1"]
func funcTypeArgNames(funcType *ast.FuncType) (names []string) {
	for _, field := range funcType.Params.List {
		if len(field.Names) == 0 {
			names = append(names, "arg"+strconv.Itoa(len(names)))
			continue
		}
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

// funcTypeArgTypes extracts argument types from a function type AST node.
// Returns type names as strings, with optional package qualification.
//
// Parameters:
//   - funcType: The function type AST node
//   - exportedNameQualifyer: Package name to use for qualifying exported types
//     (typically empty string for same-package functions)
//
// Returns:
//   - Slice of type names in declaration order
//
// Example:
//
//	func(ctx context.Context, name string, count int) with exportedNameQualifyer=""
//	-> ["context.Context", "string", "int"]
func funcTypeArgTypes(funcType *ast.FuncType, exportedNameQualifyer string) (types []string) {
	for _, field := range funcType.Params.List {
		typeName := astvisit.ExprStringWithExportedNameQualifyer(field.Type, exportedNameQualifyer)
		// An unnamed argument is a field without names
		for range max(len(field.Names), 1) {
			types = append(types, typeName)
		}
	}
	return types
}

// funcDeclArgDescriptions extracts argument descriptions from function documentation comments.
// Descriptions are extracted from lines of the comment text that start
// with the argument name followed by a colon, optionally as list item:
//
//	// MyFunc does something.
//	//   argName: Description of this argument
//	//   - otherArg: Description of another argument
//
// The description is the rest of the line.
// Leading spaces and tabs are ignored, so the lines are also found
// after gofmt reformatted indented lines as code block.
//
// Parameters:
//   - funcDecl: The function declaration with documentation
//
// Returns:
//   - Slice of description strings, one per argument (empty string if no description)
//
// These descriptions are used in the generated ArgDescriptions() method and by
// CLI and HTML form generators to provide helpful labels.
func funcDeclArgDescriptions(funcDecl *ast.FuncDecl) (descriptions []string) {
	argNames := funcTypeArgNames(funcDecl.Type)
	if len(argNames) == 0 {
		return nil
	}
	descriptions = make([]string, len(argNames))
	var lines []string
	if funcDecl.Doc != nil {
		lines = strings.Split(funcDecl.Doc.Text(), "\n")
	}
	for i, name := range argNames {
		for _, line := range lines {
			line = trimListItemMarker(strings.TrimLeft(line, " \t"))
			if description, ok := strings.CutPrefix(line, name+":"); ok {
				descriptions[i] = strings.TrimSpace(description)
				break
			}
		}
	}
	return descriptions
}

// trimListItemMarker removes the marker of a list item
// like "- ", "* ", "+ ", "1. " or "1) " from the start of line.
func trimListItemMarker(line string) string {
	for _, marker := range []string{"- ", "* ", "+ "} {
		if rest, ok := strings.CutPrefix(line, marker); ok {
			return strings.TrimLeft(rest, " \t")
		}
	}
	digits := len(line) - len(strings.TrimLeft(line, "0123456789"))
	if digits > 0 && len(line) > digits+1 && (line[digits] == '.' || line[digits] == ')') && line[digits+1] == ' ' {
		return strings.TrimLeft(line[digits+2:], " \t")
	}
	return line
}

// funcTypeResultTypes extracts result types from a function type AST node.
// Returns type names as strings, with optional package qualification.
//
// Parameters:
//   - funcType: The function type AST node
//   - exportedNameQualifyer: Package name to use for qualifying exported types
//     (typically empty string for same-package functions)
//
// Returns:
//   - Slice of result type names in declaration order, or nil if no results
//
// Handles both named and unnamed results:
//   - func() (string, error) -> ["string", "error"]
//   - func() (result string, err error) -> ["string", "error"]
//   - func() -> nil
func funcTypeResultTypes(funcType *ast.FuncType, exportedNameQualifyer string) (types []string) {
	if funcType.Results == nil {
		return nil
	}
	for _, field := range funcType.Results.List {
		types = append(types, astvisit.ExprStringWithExportedNameQualifyer(field.Type, exportedNameQualifyer))
		for i := 1; i < len(field.Names); i++ {
			types = append(types, astvisit.ExprStringWithExportedNameQualifyer(field.Type, exportedNameQualifyer))
		}
	}
	return types
}

// unqualifiedIdents adds the identifiers that the type expression expr
// refers to without package qualifier to idents,
// like the names of types and constants declared in the package of the function.
// The names of struct fields, parameters, results and interface methods
// are not added, because they don't refer to declarations.
func unqualifiedIdents(expr ast.Expr, idents map[string]struct{}) {
	ast.Inspect(expr, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.SelectorExpr:
			// Qualified identifier
			return false
		case *ast.Field:
			unqualifiedIdents(node.Type, idents)
			return false
		case *ast.Ident:
			idents[node.Name] = struct{}{}
		}
		return true
	})
}

// isContextType reports if the type expression expr of a parameter
// declared in file is context.Context, also if file imports
// the context package with another name.
func isContextType(expr ast.Expr, file *ast.File) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Context" {
		return false
	}
	qualifier, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "context" {
			continue
		}
		name := "context"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == qualifier.Name {
			return true
		}
	}
	return false
}
