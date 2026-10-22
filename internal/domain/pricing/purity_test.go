package pricing

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/coste360/arenile"

// TestPricing_NoForbiddenImports enforces INV-11 (ADR 0004): the pricing
// engine is a pure function. The module packages it builds on (pricing,
// money, calendar: the only ones allowed, checked transitively) must not
// import database, network, filesystem or randomness packages, nor call the
// wall clock. Standard-library internals (fmt importing os, ...) are not
// considered; calendar may implement driver.Valuer.
func TestPricing_NoForbiddenImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{join .GoFiles \",\"}}\t{{join .Imports \",\"}}", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	forbidden := []string{
		"database/sql", "net/http", "net", "os", "os/exec", "math/rand", "math/rand/v2", "crypto/rand",
		"github.com/uptrace/bun",
	}
	allowedInternal := map[string]bool{
		modulePath + "/internal/domain/pricing": true,
		modulePath + "/internal/money":          true,
		modulePath + "/internal/calendar":       true,
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 4)
		path, dir, files, imports := parts[0], parts[1], parts[2], parts[3]
		if path == "github.com/uptrace/bun" || strings.HasPrefix(path, "github.com/uptrace/bun/") {
			t.Errorf("pricing depends on bun")
		}
		if !strings.HasPrefix(path, modulePath) {
			continue
		}
		if !allowedInternal[path] {
			t.Errorf("pricing depends on %s: only money and calendar are allowed", path)
			continue
		}
		for _, imp := range strings.Split(imports, ",") {
			for _, f := range forbidden {
				if imp == f || (strings.HasPrefix(imp, f+"/") && imp != "database/sql/driver") {
					t.Errorf("%s imports forbidden package %s", path, imp)
					break
				}
			}
		}
		for _, name := range strings.Split(files, ",") {
			checkNoWallClock(t, filepath.Join(dir, name))
		}
	}
}

// checkNoWallClock fails on any call to time.Now, time.Since or time.Until.
func checkNoWallClock(t *testing.T, file string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	timeName := ""
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "time" {
			timeName = "time"
			if imp.Name != nil {
				timeName = imp.Name.Name
			}
		}
	}
	if timeName == "" {
		return
	}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == timeName {
			switch sel.Sel.Name {
			case "Now", "Since", "Until":
				t.Errorf("%s: uses time.%s (the engine must receive dates as input)", fset.Position(sel.Pos()), sel.Sel.Name)
			}
		}
		return true
	})
}
