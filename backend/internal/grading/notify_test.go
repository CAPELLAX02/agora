package grading_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// studentAccounts, şubedeki öğrencilere hesap açar ve kullanıcı kimliklerini öğrenci numarası
// sırasıyla (Ali, Ece, Can) döndürür.
func studentAccounts(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		INSERT INTO iam.users (person_id, username, email, password_hash)
		SELECT person_id, student_no, student_no || '@ogrenci.test', 'x' FROM people.students ORDER BY student_no
		RETURNING id`)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// notificationsOf, kullanıcının bildirimleridir: tür ve ayrıntı (aşama, karar ya da bileşen),
// eskiden yeniye.
func notificationsOf(t *testing.T, pool *pgxpool.Pool, userID string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT type_code || coalesce(':' || coalesce(payload->>'phase', payload->>'decision', payload->>'component_tr'), '')
		FROM communication.notifications WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}
