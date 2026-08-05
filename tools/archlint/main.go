package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	violations := []string{}
	err := filepath.Walk("services", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		slashPath := filepath.ToSlash(path)
		for _, imported := range file.Imports {
			name, _ := strconv.Unquote(imported.Path.Value)
			if strings.Contains(slashPath, "/domain/") && (name == "github.com/gin-gonic/gin" || name == "gorm.io/gorm" || name == "net/http") {
				violations = append(violations, fmt.Sprintf("L-2 %s imports %s", path, name))
			}
			if strings.Contains(slashPath, "/controllers/") && filepath.Base(path) != "service.go" && strings.Contains(name, "/db/") {
				violations = append(violations, fmt.Sprintf("L-3 %s imports %s", path, name))
			}
		}
		if strings.Contains(slashPath, "/domain/") || strings.Contains(slashPath, "/entities/domain/") {
			ast.Inspect(file, func(node ast.Node) bool {
				ident, ok := node.(*ast.Ident)
				if ok && (ident.Name == "float32" || ident.Name == "float64") {
					violations = append(violations, fmt.Sprintf("M-1 %s declares %s", path, ident.Name))
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(violations) > 0 {
		for _, violation := range violations {
			fmt.Fprintln(os.Stderr, violation)
		}
		os.Exit(1)
	}
	fmt.Println("architecture boundaries OK")
}
