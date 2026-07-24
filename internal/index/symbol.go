package index

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

func ExtractSymbols(root, filePath string) ([]Symbol, string, error) {
	relPath, err := filepath.Rel(root, filePath)
	if err != nil {
		relPath = filePath
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, "", err
	}

	packageName := node.Name.Name
	var symbols []Symbol

	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		switch decl := n.(type) {
		case *ast.FuncDecl:
			sym := Symbol{
				Name: decl.Name.Name,
				File: relPath,
				Line: fset.Position(decl.Pos()).Line,
			}

			if decl.Doc != nil {
				sym.Doc = strings.TrimSpace(decl.Doc.Text())
			}

			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				sym.Kind = "method"
				var buf bytes.Buffer
				ast.Fprint(&buf, fset, decl.Recv.List[0].Type, ast.NotNilFilter)
				sym.Receiver = strings.TrimSpace(buf.String())
			} else {
				sym.Kind = "function"
			}

			symbols = append(symbols, sym)

		case *ast.GenDecl:
			var kind string
			switch decl.Tok {
			case token.IMPORT:
				return true
			case token.CONST:
				kind = "const"
			case token.VAR:
				kind = "var"
			case token.TYPE:
				kind = "type"
			}

			for _, spec := range decl.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := Symbol{
						Name: s.Name.Name,
						Kind: kind,
						File: relPath,
						Line: fset.Position(s.Pos()).Line,
					}
					if s.Doc != nil {
						sym.Doc = strings.TrimSpace(s.Doc.Text())
					} else if decl.Doc != nil {
						sym.Doc = strings.TrimSpace(decl.Doc.Text())
					}

					switch s.Type.(type) {
					case *ast.StructType:
						sym.Kind = "struct"
					case *ast.InterfaceType:
						sym.Kind = "interface"
					}
					symbols = append(symbols, sym)

				case *ast.ValueSpec:
					for _, name := range s.Names {
						sym := Symbol{
							Name: name.Name,
							Kind: kind,
							File: relPath,
							Line: fset.Position(name.Pos()).Line,
						}
						if s.Doc != nil {
							sym.Doc = strings.TrimSpace(s.Doc.Text())
						} else if decl.Doc != nil {
							sym.Doc = strings.TrimSpace(decl.Doc.Text())
						}
						symbols = append(symbols, sym)
					}
				}
			}
		}

		return true
	})

	return symbols, packageName, nil
}
