package people_test

import (
	"context"
	"errors"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/people"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

func TestRepository(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := people.NewRepository(pool)

	newPerson := func(t *testing.T) string {
		t.Helper()
		id, err := repo.CreatePerson(ctx, people.NewPerson{FirstName: "Deniz", LastName: "Aksoy"})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	t.Run("öğrenci numarası benzersiz", func(t *testing.T) {
		if err := repo.CreateStudent(ctx, newPerson(t), "22290001"); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateStudent(ctx, newPerson(t), "22290001"); !errors.Is(err, people.ErrConflict) {
			t.Errorf("err = %v, want ErrConflict", err)
		}
	})

	t.Run("öğrenci numarası biçimi veritabanında denetlenir", func(t *testing.T) {
		if err := repo.CreateStudent(ctx, newPerson(t), "OGR-1"); err == nil {
			t.Error("rakam olmayan öğrenci numarası kabul edildi")
		}
	})

	t.Run("akademik unvan sadece akademik personelde", func(t *testing.T) {
		err := repo.CreateStaff(ctx, people.NewStaff{
			PersonID: newPerson(t), StaffNo: "P1", Type: people.StaffAcademic, AcademicTitle: "PROF",
		})
		if err != nil {
			t.Fatalf("akademik personel: %v", err)
		}

		err = repo.CreateStaff(ctx, people.NewStaff{
			PersonID: newPerson(t), StaffNo: "P2", Type: people.StaffAdministrative, AcademicTitle: "PROF",
		})
		if err == nil {
			t.Error("idari personele akademik unvan verilebildi")
		}
	})

	t.Run("boş ad kabul edilmez", func(t *testing.T) {
		if _, err := repo.CreatePerson(ctx, people.NewPerson{FirstName: "  ", LastName: "X"}); err == nil {
			t.Error("boşluktan oluşan ad kabul edildi")
		}
	})
}
