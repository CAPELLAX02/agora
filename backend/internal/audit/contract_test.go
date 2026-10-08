package audit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestSecurityEventTypesMatchContract, koddaki güvenlik olayı türlerinin sözleşmedeki
// SecurityEventType listesiyle aynı olduğunu doğrular. Yeni bir olay eklenip sözleşme
// unutulursa web istemcisi o türü tanımaz ve filtrede seçilemez.
func TestSecurityEventTypesMatchContract(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "audit.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var code []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Event") || i >= len(vs.Values) {
					continue
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
					v, _ := strconv.Unquote(lit.Value)
					code = append(code, v)
				}
			}
		}
	}

	spec, err := os.ReadFile("../../../contracts/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\n    SecurityEventType:\n      type: string\n      enum:\n(.*?)\n\n`).FindSubmatch(spec)
	if block == nil {
		t.Fatal("sözleşmede SecurityEventType bulunamadı")
	}
	var contract []string
	for _, m := range regexp.MustCompile(`- ([A-Z_]+)`).FindAllSubmatch(block[1], -1) {
		contract = append(contract, string(m[1]))
	}

	slices.Sort(code)
	slices.Sort(contract)
	if !slices.Equal(code, contract) {
		t.Errorf("kod ve sözleşme farklı:\n  kod:       %v\n  sözleşme:  %v", code, contract)
	}
}
