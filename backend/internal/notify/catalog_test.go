package notify

import (
	"context"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// Katalogdaki (migration) her türün şablonu, her şablonun da katalogda türü olmalı.
func TestCatalogMatchesTexts(t *testing.T) {
	pool := dbtest.New(t)
	rows, err := pool.Query(context.Background(), `SELECT code FROM communication.notification_types`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[Type]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatal(err)
		}
		seen[Type(code)] = true
		if _, ok := texts[Type(code)]; !ok {
			t.Errorf("%s türünün şablonu yok", code)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for typ, text := range texts {
		if !seen[typ] {
			t.Errorf("%s şablonunun katalogda türü yok", typ)
		}
		if text.TitleTR == "" || text.TitleEN == "" || text.BodyTR == "" || text.BodyEN == "" {
			t.Errorf("%s şablonunda boş alan var", typ)
		}
	}
}
