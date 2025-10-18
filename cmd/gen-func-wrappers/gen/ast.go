package gen

import (
	"go/ast"
	"strings"

	"github.com/ungerik/go-astvisit"
)

// funcTypeArgNames extracts argument names from a function type AST node.
// Returns the names in order, as they appear in the function signature.
//
// Example:
//
//	func(ctx context.Context, name string, age int) -> ["ctx", "name", "age"]
func funcTypeArgNames(funcType *ast.FuncType) (names []string) {
	for _, field := range funcType.Params.List {
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
		for range field.Names {
			types = append(types, astvisit.ExprStringWithExportedNameQualifyer(field.Type, exportedNameQualifyer))
		}
	}
	return types
}

// funcDeclArgDescriptions extracts argument descriptions from function documentation comments.
// Descriptions are extracted from comments using the format:
//
//	// MyFunc does something.
//	//   argName: Description of this argument
//	//   otherArg: Description of another argument
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
	for _, field := range funcDecl.Type.Params.List {
		for _, name := range field.Names {
			description := ""
			if funcDecl.Doc != nil {
				label := " " + name.Name + ": "
				for _, comment := range funcDecl.Doc.List {
					if labelPos := strings.Index(comment.Text, label); labelPos != -1 {
						description = strings.TrimSpace(comment.Text[labelPos+len(label):])
						break
					}
				}
			}
			descriptions = append(descriptions, description)
		}
	}
	return descriptions
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
