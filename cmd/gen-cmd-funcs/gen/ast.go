package gen

import (
	"fmt"
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

func funcDeclArgTypes(funcDecl *ast.FuncDecl) (types []string) {
	for _, field := range funcDecl.Type.Params.List {
		for range field.Names {
			types = append(types, astvisit.ExprString(field.Type))
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

func funcDeclResultTypes(funcDecl *ast.FuncDecl) (types []string) {
	if funcDecl.Type.Results == nil {
		return nil
	}
	for _, field := range funcDecl.Type.Results.List {
		types = append(types, astvisit.ExprString(field.Type))
		for i := 1; i < len(field.Names); i++ {
			types = append(types, astvisit.ExprString(field.Type))
		}
	}
	return types
}

func recursiveExprSelectors(expr ast.Expr, setSelectors map[string]struct{}) {
	switch e := expr.(type) {
	case *ast.Ident:
		// Name without selector
	case *ast.SelectorExpr:
		setSelectors[e.X.(*ast.Ident).Name] = struct{}{}
	case *ast.StarExpr:
		recursiveExprSelectors(e.X, setSelectors)
	case *ast.Ellipsis:
		recursiveExprSelectors(e.Elt, setSelectors)
	case *ast.ArrayType:
		recursiveExprSelectors(e.Elt, setSelectors)
	case *ast.StructType:
		for _, f := range e.Fields.List {
			recursiveExprSelectors(f.Type, setSelectors)
		}
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			recursiveExprSelectors(elt, setSelectors)
		}
	case *ast.MapType:
		recursiveExprSelectors(e.Key, setSelectors)
		recursiveExprSelectors(e.Value, setSelectors)
	case *ast.ChanType:
		recursiveExprSelectors(e.Value, setSelectors)
	case *ast.FuncType:
		for _, p := range e.Params.List {
			recursiveExprSelectors(p.Type, setSelectors)
		}
		if e.Results != nil {
			for _, r := range e.Results.List {
				recursiveExprSelectors(r.Type, setSelectors)
			}
		}
	default:
		panic(fmt.Sprintf("UNSUPPORTED: %#v", expr))
	}
}
