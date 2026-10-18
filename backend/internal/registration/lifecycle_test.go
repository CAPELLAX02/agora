package registration_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/academic"
	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
	"github.com/CAPELLAX02/agora/backend/internal/registration"
)

func (w *world) dispatch(consumers ...outbox.Consumer) {
	w.t.Helper()
	d := outbox.NewDispatcher(w.pool, consumers, outbox.DefaultConfig, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if _, err := d.ProcessBatch(context.Background()); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) count(sql string, args ...any) int {
	w.t.Helper()
	var n int
	if err := w.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		w.t.Fatal(err)
	}
	return n
}

// notifications, kullanıcının bildirimleridir: tür ve (varsa) karar ya da aşama, eskiden yeniye.
func (w *world) notifications(userID string) []string {
	w.t.Helper()
	rows, err := w.pool.Query(context.Background(), `
		SELECT type_code || coalesce(':' || coalesce(payload->>'decision', payload->>'stage'), '')
		FROM communication.notifications WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		w.t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		w.t.Fatal(err)
	}
	return out
}

// Gönderilen kayıt danışmana, danışman kararı öğrenciye bildirilir; karar e-postayla da gider
// (türün varsayılan kanalları).
func TestDecisionNotification(t *testing.T) {
	w := newWorld(t)
	if r := w.add(w.ali, w.aliSP, "MAT111"); r.status != http.StatusCreated {
		t.Fatalf("ekleme: %d %s", r.status, r.raw)
	}
	r := w.do("POST", "/api/v1/student-programs/"+w.aliSP+"/registration/submit", w.ali, nil, "If-Match", etag(w.registration(w.ali, w.aliSP)))
	if got := w.notifications(w.advisor); !slices.Equal(got, []string{"registration.submitted"}) {
		t.Errorf("danışmanın bildirimleri: %v", got)
	}
	r = w.do("POST", "/api/v1/registrations/"+r.body["id"].(string)+"/approve", w.advisor, map[string]string{"note": "Başarılar"}, "If-Match", etag(r))
	if r.status != 200 {
		t.Fatalf("onay: %d %s", r.status, r.raw)
	}
	if got := w.notifications(w.ali); !slices.Equal(got, []string{"registration.decision:APPROVED"}) {
		t.Errorf("öğrencinin bildirimleri: %v", got)
	}
	var to, title, body, link string
	if err := w.pool.QueryRow(context.Background(), `
		SELECT to_address, payload->>'title', payload->>'body', payload->>'link' FROM communication.email_outbox
		WHERE template_code = 'notification'`).Scan(&to, &title, &body, &link); err != nil {
		t.Fatal(err)
	}
	if to != "24000001@agora.test" || title != "Ders kaydınız onaylandı" || !strings.Contains(body, "Danışman notu: Başarılar") ||
		!strings.Contains(body, "1 ders") || link != "/ders-secimi" {
		t.Errorf("e-posta = %s %q %q %s", to, title, body, link)
	}
	if n := w.count(`SELECT count(*) FROM communication.email_outbox`); n != 1 {
		t.Errorf("%d e-posta kuyruğa girdi, 1 olmalı (danışmana e-posta gitmez)", n)
	}
}

// Bahar dönemi aktif yapılınca aktif öğrencilerin yarıyılı bir ilerler; aynı ya da daha
// eski bir dönem yeniden aktif yapılırsa ilerlemez.
func TestSemesterAdvance(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	w.exec(`
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on)
		SELECT id, 'SPRING', '2026-SPRING', '2027-02-08', '2027-07-02' FROM academic.academic_years WHERE start_year = 2026;
		INSERT INTO enrollment.semester_advances (term_id, advanced) SELECT id, 0 FROM academic.terms WHERE code = '2026-FALL';
		UPDATE enrollment.student_programs SET status = 'FROZEN' WHERE id = '` + w.berkSP + `';`)
	cal := academic.NewRepository(w.pool)
	var spring, fall string
	_ = w.pool.QueryRow(ctx, `SELECT id FROM academic.terms WHERE code = '2026-SPRING'`).Scan(&spring)
	_ = w.pool.QueryRow(ctx, `SELECT id FROM academic.terms WHERE code = '2026-FALL'`).Scan(&fall)

	semester := func(sp string) (int, int) {
		var s, c int
		_ = w.pool.QueryRow(ctx, `SELECT current_semester, class_level FROM enrollment.student_programs WHERE id = $1`, sp).Scan(&s, &c)
		return s, c
	}
	if err := cal.MakeCurrent(ctx, "", spring); err != nil {
		t.Fatal(err)
	}
	w.dispatch(enrollment.SemesterAdvancer{})
	if s, c := semester(w.aliSP); s != 4 || c != 2 {
		t.Errorf("ali = %d. yarıyıl, %d. sınıf; 4/2 olmalı", s, c)
	}
	if s, _ := semester(w.berkSP); s != 3 {
		t.Errorf("dondurulmuş kayıt ilerlememeli: %d", s)
	}
	// Güze dönüp yeniden bahara geçmek yarıyılı tekrar ilerletmez.
	if err := cal.MakeCurrent(ctx, "", fall); err != nil {
		t.Fatal(err)
	}
	if err := cal.MakeCurrent(ctx, "", spring); err != nil {
		t.Fatal(err)
	}
	w.dispatch(enrollment.SemesterAdvancer{})
	if s, _ := semester(w.aliSP); s != 4 {
		t.Errorf("tekrar ilerletildi: %d", s)
	}
}

// Ders seçme penceresi bitince gönderilmemiş sepetin kontenjanı serbest kalır; ek süresi
// süren kayda dokunulmaz.
func TestExpireDrafts(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	for _, s := range []struct{ user, sp string }{{w.ali, w.aliSP}, {w.berk, w.berkSP}} {
		if r := w.add(s.user, s.sp, "ALG201"); r.status != http.StatusCreated {
			t.Fatalf("ekleme: %d %s", r.status, r.raw)
		}
	}
	w.now = time.Date(2026, time.September, 25, 9, 0, 0, 0, time.UTC)
	if r := w.do("POST", "/api/v1/student-programs/"+w.berkSP+"/registration/extension", w.registrar,
		map[string]string{"until": w.now.Add(24 * time.Hour).Format(time.RFC3339), "reason": "Rapor süresi"}, "X-Test-MFA", "1"); r.status != 200 {
		t.Fatalf("ek süre: %d %s", r.status, r.raw)
	}

	svc := registration.NewService(w.pool, func() time.Time { return w.now }, nil)
	// Pencere açıkken (geçmişe dönülürse) bir şey yapılmaz.
	early := registration.NewService(w.pool, func() time.Time { return time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC) }, nil)
	if n, err := early.ExpireDrafts(ctx); err != nil || n != 0 {
		t.Fatalf("açık pencerede: %d, %v", n, err)
	}
	n, err := svc.ExpireDrafts(ctx)
	if err != nil || n != 1 {
		t.Fatalf("boşaltılan = %d, %v; 1 olmalı (ek süreli kayıt hariç)", n, err)
	}
	if got := w.count(`SELECT enrolled_count FROM offering.sections WHERE id = $1`, w.sections["ALG201"]); got != 1 {
		t.Errorf("şube mevcudu = %d, 1 olmalı", got)
	}
	if got := w.count(`
		SELECT count(*) FROM enrollment.registration_events e JOIN enrollment.registrations r ON r.id = e.registration_id
		WHERE r.student_program_id = $1 AND e.event_type = 'EXPIRED'`, w.aliSP); got != 1 {
		t.Errorf("EXPIRED olayı = %d", got)
	}
	if n, _ := svc.ExpireDrafts(ctx); n != 0 {
		t.Errorf("ikinci çalıştırma %d sepet boşalttı", n)
	}
}

// Aynı Idempotency-Key ile tekrarlanan ekleme ikinci kez yer ayırmaz; ilk yanıt döner.
func TestIdempotentAdd(t *testing.T) {
	w := newWorld(t)
	path := "/api/v1/student-programs/" + w.aliSP + "/registration/items"
	body := map[string]string{"section_id": w.sections["ALG201"]}
	first := w.do("POST", path, w.ali, body, "Idempotency-Key", "ekle-alg201-0001")
	again := w.do("POST", path, w.ali, body, "Idempotency-Key", "ekle-alg201-0001")
	if first.status != http.StatusCreated || again.status != http.StatusCreated || again.header.Get("Idempotent-Replayed") != "true" {
		t.Fatalf("ilk %d, tekrar %d %v", first.status, again.status, again.header)
	}
	if n := w.count(`SELECT enrolled_count FROM offering.sections WHERE id = $1`, w.sections["ALG201"]); n != 1 {
		t.Errorf("şube mevcudu = %d, 1 olmalı", n)
	}
	// Anahtarsız tekrar kural motoruna takılır (aynı ders).
	if r := w.do("POST", path, w.ali, body); r.status != 422 {
		t.Errorf("anahtarsız tekrar: %d", r.status)
	}
}
