package org

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// TestRepository, gerçek bir PostgreSQL'e karşı çalışan entegrasyon testidir.
// Konteyner bir kez başlatılır, alt testler aynı veriyi paylaşır.
func TestRepository(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	seedFaculties(t, ctx, pool)

	codes := func(fs []Faculty) []string {
		out := make([]string, 0, len(fs))
		for _, f := range fs {
			out = append(out, f.Code)
		}
		return out
	}

	t.Run("varsayılan: sadece aktifler, Türkçe alfabetik sıra", func(t *testing.T) {
		got, err := NewRepository(pool).ListFaculties(ctx, FacultyFilter{})
		if err != nil {
			t.Fatal(err)
		}
		// Çevre < İletişim < Mühendislik < Ziraat. Varsayılan (C) sıralamada
		// Ç ve İ, Z'den sonra gelirdi.
		want := []string{"CEVRE", "ILT", "MUH", "ZRT"}
		if !slices.Equal(codes(got), want) {
			t.Errorf("sıra = %v, want %v", codes(got), want)
		}
	})

	t.Run("pasifler dahil", func(t *testing.T) {
		got, err := NewRepository(pool).ListFaculties(ctx, FacultyFilter{IncludeInactive: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 5 {
			t.Errorf("birim sayısı = %d, want 5: %v", len(got), codes(got))
		}
	})

	t.Run("türe göre filtre", func(t *testing.T) {
		got, err := NewRepository(pool).ListFaculties(ctx, FacultyFilter{UnitType: UnitVocationalSchool})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(codes(got), []string{"CEVRE"}) {
			t.Errorf("birimler = %v, want [CEVRE]", codes(got))
		}
	})

	t.Run("arama büyük/küçük harf duyarsız", func(t *testing.T) {
		got, err := NewRepository(pool).ListFaculties(ctx, FacultyFilter{Query: "ENGINEERING"})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(codes(got), []string{"MUH"}) {
			t.Errorf("birimler = %v, want [MUH]", codes(got))
		}
	})

	t.Run("yerleşke bilgisi JOIN ile gelir", func(t *testing.T) {
		got, err := NewRepository(pool).ListFaculties(ctx, FacultyFilter{Query: "MUH"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Campus == nil || got[0].Campus.Code != "GOLBASI" {
			t.Fatalf("yerleşke bilgisi eksik: %+v", got)
		}

		f, err := NewRepository(pool).GetFaculty(ctx, got[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if f.Code != "MUH" || f.Campus == nil || f.Campus.Name != "50. Yıl Yerleşkesi" {
			t.Errorf("GetFaculty = %+v", f)
		}
	})

	t.Run("yerleşkesi olmayan birimde Campus nil", func(t *testing.T) {
		got, err := NewRepository(pool).ListFaculties(ctx, FacultyFilter{Query: "ZRT"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Campus != nil {
			t.Errorf("Campus nil olmalıydı: %+v", got)
		}
	})

	t.Run("olmayan kimlik ErrNotFound döner", func(t *testing.T) {
		_, err := NewRepository(pool).GetFaculty(ctx, "01a1168a-ffff-7fff-bfff-ffffffffffff")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}

func seedFaculties(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO org.campuses (code, name) VALUES ('GOLBASI', '50. Yıl Yerleşkesi');

		INSERT INTO org.faculties (code, name_tr, name_en, unit_type, campus_id, is_active) VALUES
			('MUH',   'Mühendislik Fakültesi',          'Faculty of Engineering',            'FACULTY',
				(SELECT id FROM org.campuses WHERE code = 'GOLBASI'), true),
			('ZRT',   'Ziraat Fakültesi',               'Faculty of Agriculture',            'FACULTY', NULL, true),
			('ILT',   'İletişim Fakültesi',             'Faculty of Communication',          'FACULTY', NULL, true),
			('CEVRE', 'Çevre Meslek Yüksekokulu',       'Vocational School of Environment',  'VOCATIONAL_SCHOOL', NULL, true),
			('ESKI',  'Kapatılmış Yüksekokul',          'Closed School',                     'SCHOOL', NULL, false);
	`)
	if err != nil {
		t.Fatalf("test verisi eklenemedi: %v", err)
	}
}
