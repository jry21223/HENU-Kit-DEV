// Command rowserrcheck reports `for rows.Next()` loops whose enclosing function
// never consults rows.Err().
//
// database/sql and pgx both surface a mid-iteration failure — a dropped
// connection, a statement timeout, a decode error — only through Err(). A bare
// Next() loop simply stops, so the function returns a partial result with a nil
// error and its caller serves a truncated read as a success.
//
// This exists because the off-the-shelf options do not currently work here:
// upstream rowserrcheck is unmaintained and cannot parse Go 1.26, and
// golangci-lint's released binary is built with an older Go than these modules
// target and refuses to run. It is deliberately small and stdlib-only, matching
// how the repository already pins its other Go tooling.
//
// Usage: go run ./scripts/ops/rowserrcheck <dir>...
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}

	fileSet := token.NewFileSet()
	var findings []string

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "node_modules", "vendor", ".git", "testdata":
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			parsed, parseErr := parser.ParseFile(fileSet, path, nil, 0)
			if parseErr != nil {
				return fmt.Errorf("parse %s: %w", path, parseErr)
			}
			findings = append(findings, checkFile(fileSet, parsed)...)
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "rowserrcheck:", err)
			os.Exit(2)
		}
	}

	for _, finding := range findings {
		fmt.Println(finding)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "\nrowserrcheck: %d loop(s) never consult rows.Err()\n", len(findings))
		os.Exit(1)
	}
}

// checkFile reports every `for <recv>.Next()` loop in fn whose enclosing
// function never calls <recv>.Err().
func checkFile(fileSet *token.FileSet, file *ast.File) []string {
	var findings []string

	ast.Inspect(file, func(node ast.Node) bool {
		var body *ast.BlockStmt
		switch declaration := node.(type) {
		case *ast.FuncDecl:
			body = declaration.Body
		case *ast.FuncLit:
			body = declaration.Body
		default:
			return true
		}
		if body == nil {
			return true
		}

		for _, receiver := range nextLoopReceivers(body) {
			if !callsMethod(body, receiver.name, "Err") {
				findings = append(findings, fmt.Sprintf(
					"%s: for %s.Next() loop never checks %s.Err()",
					fileSet.Position(receiver.pos), receiver.name, receiver.name,
				))
			}
		}
		return true
	})

	return findings
}

type receiver struct {
	name string
	pos  token.Pos
}

// nextLoopReceivers finds `for x.Next() { ... }` loops directly inside body,
// excluding loops nested in a function literal, which is checked on its own.
func nextLoopReceivers(body *ast.BlockStmt) []receiver {
	var found []receiver
	seen := map[string]bool{}

	ast.Inspect(body, func(node ast.Node) bool {
		if _, isFunc := node.(*ast.FuncLit); isFunc {
			return false
		}
		loop, isFor := node.(*ast.ForStmt)
		if !isFor || loop.Cond == nil {
			return true
		}
		name, ok := methodCallReceiver(loop.Cond, "Next")
		if !ok || seen[name] {
			return true
		}
		seen[name] = true
		found = append(found, receiver{name: name, pos: loop.Pos()})
		return true
	})

	return found
}

// callsMethod reports whether body contains a call to <name>.<method>().
func callsMethod(body *ast.BlockStmt, name, method string) bool {
	called := false
	ast.Inspect(body, func(node ast.Node) bool {
		if called {
			return false
		}
		if receiverName, ok := methodCallReceiver(node, method); ok && receiverName == name {
			called = true
			return false
		}
		return true
	})
	return called
}

// methodCallReceiver returns the identifier x from an expression `x.method()`.
func methodCallReceiver(node ast.Node, method string) (string, bool) {
	call, isCall := node.(*ast.CallExpr)
	if !isCall {
		return "", false
	}
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != method {
		return "", false
	}
	ident, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return "", false
	}
	return ident.Name, true
}
