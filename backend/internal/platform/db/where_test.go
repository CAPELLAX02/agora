package db

import (
	"reflect"
	"testing"
)

func TestWhere(t *testing.T) {
	var w Where
	if w.SQL() != "" || len(w.Args()) != 0 {
		t.Fatalf("boş Where: SQL=%q args=%v", w.SQL(), w.Args())
	}

	w.Add("p.is_active")
	w.Add("p.degree_level = $1", "BACHELOR")
	w.Add("(p.code ILIKE $1 OR p.name_tr ILIKE $1)", "%bil%")
	w.Add("(p.name_tr, p.id) > ($1, $2::uuid)", "Bilgisayar", "01a1-...")
	limit := w.Arg(21)

	wantSQL := "WHERE p.is_active AND p.degree_level = $1 AND (p.code ILIKE $2 OR p.name_tr ILIKE $2)" +
		" AND (p.name_tr, p.id) > ($3, $4::uuid)"
	if got := w.SQL(); got != wantSQL {
		t.Errorf("SQL =\n  %s\nwant\n  %s", got, wantSQL)
	}
	if limit != "$5" {
		t.Errorf("Arg yer tutucusu = %q, want $5", limit)
	}

	wantArgs := []any{"BACHELOR", "%bil%", "Bilgisayar", "01a1-...", 21}
	if !reflect.DeepEqual(w.Args(), wantArgs) {
		t.Errorf("Args = %v, want %v", w.Args(), wantArgs)
	}
}
