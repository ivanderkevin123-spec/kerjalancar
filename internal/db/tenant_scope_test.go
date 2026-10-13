package db_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Every SQL statement that reads or writes a tenant table must mention
// workspace_id (ARCHITECTURE §5). This scans the Go source for string
// literals instead of relying on a line-based grep, because the SQL is
// multi-line. Statements that only reference a tenant table through an
// already-scoped join are still required to carry workspace_id.
func TestEverySQLOnTenantTablesIsScopedByWorkspace(t *testing.T) {
	tenant := regexp.MustCompile(`(?i)\b(FROM|UPDATE|INTO|JOIN)\s+(customers|leads|lead_activities|follow_ups|message_templates|quotations|quotation_items|invoices|invoice_items|payments|payment_reversals)\b`)
	root := filepath.Join("..", "..")
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil || !tenant.MatchString(s) {
				return true
			}
			if !strings.Contains(s, "workspace_id") {
				offenders = append(offenders, fset.Position(lit.Pos()).String()+": "+strings.Join(strings.Fields(s), " "))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range offenders {
		t.Errorf("tenant table queried without workspace_id: %s", o)
	}
}
