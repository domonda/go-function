package gen

import (
	"go/ast"
	"strings"

	"github.com/ungerik/go-astvisit"
)

func funcDeclArgNames(funcDecl *ast.FuncDecl) (names []string) {
	for _, field := range funcDecl.Type.Params.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

func funcDeclArgTypes(funcDecl *ast.FuncDecl, exportedNameQualifyer string) (types []string) {
	for _, field := range funcDecl.Type.Params.List {
		for range field.Names {
			types = append(types, astvisit.ExprStringWithExportedNameQualifyer(field.Type, exportedNameQualifyer))
		}
	}
	return types
}

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

func funcDeclResultTypes(funcDecl *ast.FuncDecl, exportedNameQualifyer string) (types []string) {
	if funcDecl.Type.Results == nil {
		return nil
	}
	for _, field := range funcDecl.Type.Results.List {
		types = append(types, astvisit.ExprStringWithExportedNameQualifyer(field.Type, exportedNameQualifyer))
		for i := 1; i < len(field.Names); i++ {
			types = append(types, astvisit.ExprStringWithExportedNameQualifyer(field.Type, exportedNameQualifyer))
		}
	}
	return types
}
