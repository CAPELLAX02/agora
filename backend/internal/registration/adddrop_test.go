package registration_test

import (
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestAddDrop(t *testing.T) {
	w := newWorld(t)
	for _, c := range []string{"MAT111", "ALG201"} {
		if r := w.add(w.ali, w.aliSP, c); r.status != http.StatusCreated {
			t.Fatalf("%s: %d %s", c, r.status, r.raw)
		}
	}
	r := w.do("POST", "/api/v1/student-programs/"+w.aliSP+"/registration/submit", w.ali, nil, "If-Match", etag(w.registration(w.ali, w.aliSP)))
	regID := r.body["id"].(string)
	if r = w.do("POST", "/api/v1/registrations/"+regID+"/approve", w.advisor, map[string]string{}, "If-Match", etag(r)); r.status != 200 {
		t.Fatalf("onay: %d %s", r.status, r.raw)
	}
	algItem := itemFor(r, "ALG201")
	path := "/api/v1/student-programs/" + w.aliSP + "/registration/add-drop"

	// Ekle-bırak penceresi açılmadan talep açılamaz.
	if r := w.do("POST", path, w.ali, map[string]any{"adds": []string{w.sections["OS301"]}}); r.status != 409 || r.body["code"] != "WINDOW_CLOSED" {
		t.Errorf("pencere öncesi talep: %d %s", r.status, r.raw)
	}
	w.now = time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	// Alttan dersi bırakmak kurallara takılır.
	if r := w.do("POST", path, w.ali, map[string]any{"drops": []string{itemFor(w.registration(w.ali, w.aliSP), "MAT111")}}); r.status != 422 ||
		codes(r)[0] != "REPEAT_PRIORITY:ERROR" {
		t.Errorf("alttan dersi bırakma: %d %s", r.status, r.raw)
	}
	if r := w.do("POST", path, w.ali, map[string]any{"adds": []string{w.sections["SEC302"]}}); r.status != 409 || r.body["code"] != "SECTION_NOT_FOR_PROGRAM" {
		t.Errorf("başka programın kontenjanı: %d %s", r.status, r.raw)
	}
	r = w.do("POST", path, w.ali, map[string]any{"adds": []string{w.sections["OS301"]}, "drops": []string{algItem}, "note": "Algoritmaları bahara bırakıyorum"})
	if r.status != http.StatusCreated || r.body["status"] != "PENDING_ADVISOR" || r.body["stage"] != "ADVISOR" || r.body["ects_delta"].(float64) != 0 {
		t.Fatalf("talep: %d %s", r.status, r.raw)
	}
	reqID := r.body["id"].(string)
	if n := w.count(`SELECT enrolled_count FROM offering.sections WHERE id = $1`, w.sections["OS301"]); n != 1 {
		t.Errorf("eklenecek şubenin kontenjanı talepte ayrılmalı: %d", n)
	}
	if r := w.do("POST", path, w.ali, map[string]any{"adds": []string{w.sections["SEC303"]}}); r.status != 409 || r.body["code"] != "ADD_DROP_OPEN" {
		t.Errorf("ikinci açık talep: %d %s", r.status, r.raw)
	}
	approve := "/api/v1/add-drop-requests/" + reqID + "/approve"
	if r := w.do("POST", approve, w.ali, map[string]string{}, "If-Match", etag(r)); r.status != 403 {
		t.Errorf("öğrenci kendi talebini onaylayamaz: %d", r.status)
	}
	if r := w.do("POST", approve, w.head, map[string]string{}, "If-Match", etag(r)); r.status != 403 {
		t.Errorf("danışman aşamasında bölüm başkanı karar veremez: %d", r.status)
	}

	// Danışman kuyruğu ve onayı: talep bölüm başkanına geçer.
	if q := w.do("GET", "/api/v1/me/add-drop-queue", w.advisor, nil).items(); len(q) != 1 || obj(q[0])["stage"] != "ADVISOR" {
		t.Fatalf("danışman kuyruğu = %v", q)
	}
	r = w.do("POST", approve, w.advisor, map[string]string{}, "If-Match", etag(r))
	if r.status != 200 || r.body["status"] != "PENDING_DEPT_HEAD" || obj(r.body["advisor"])["by"] == nil {
		t.Fatalf("danışman onayı: %d %s", r.status, r.raw)
	}
	if q := w.do("GET", "/api/v1/me/add-drop-queue", w.head, nil).items(); len(q) != 1 || obj(q[0])["stage"] != "DEPT_HEAD" {
		t.Fatalf("bölüm başkanı kuyruğu = %v", q)
	}
	// Bölüm başkanı penceresi 26 Eylül'de açılır.
	if r := w.do("POST", approve, w.head, map[string]string{}, "If-Match", etag(r)); r.status != 409 || r.body["code"] != "WINDOW_CLOSED" {
		t.Errorf("pencere öncesi bölüm başkanı onayı: %d %s", r.status, r.raw)
	}
	w.now = time.Date(2026, time.September, 27, 9, 0, 0, 0, time.UTC)
	r = w.do("POST", approve, w.head, map[string]string{}, "If-Match", etag(r))
	if r.status != 200 || r.body["status"] != "APPROVED" {
		t.Fatalf("bölüm başkanı onayı: %d %s", r.status, r.raw)
	}

	// Uygulandı: ALG201 bırakıldı (yeri açıldı), OS301 eklendi.
	reg := w.registration(w.ali, w.aliSP)
	if itemFor(reg, "ALG201") != "" || itemFor(reg, "OS301") == "" || reg.body["total_ects"].(float64) != 11 {
		t.Errorf("kayıt = %s", reg.raw)
	}
	if n := w.count(`SELECT enrolled_count FROM offering.sections WHERE id = $1`, w.sections["ALG201"]); n != 0 {
		t.Errorf("bırakılan şubenin yeri açılmalı: %d", n)
	}
	if n := w.count(`SELECT count(*) FROM enrollment.enrollments WHERE student_program_id = $1 AND status = 'ENROLLED'`, w.aliSP); n != 2 {
		t.Errorf("kesin kayıt = %d, 2 olmalı", n)
	}
	if n := w.count(`SELECT count(*) FROM enrollment.enrollments WHERE student_program_id = $1 AND status = 'DROPPED' AND course_id = $2`,
		w.aliSP, w.courses["ALG201"]); n != 1 {
		t.Errorf("bırakılan kesin kayıt = %d", n)
	}
	if n := w.count(`SELECT count(*) FROM platform.outbox_events WHERE event_type = 'registration.add_drop_applied'`); n != 1 {
		t.Errorf("uygulama olayı = %d", n)
	}

	// Geri çekme ve red ayrılan kontenjanı iade eder.
	w.now = time.Date(2026, time.September, 24, 9, 0, 0, 0, time.UTC)
	r = w.do("POST", path, w.ali, map[string]any{"adds": []string{w.sections["SEC303"]}})
	if r.status != http.StatusCreated {
		t.Fatalf("ikinci talep: %d %s", r.status, r.raw)
	}
	if r := w.do("POST", "/api/v1/add-drop-requests/"+r.body["id"].(string)+"/withdraw", w.ali, nil, "If-Match", etag(r)); r.status != 200 ||
		r.body["status"] != "WITHDRAWN" {
		t.Fatalf("geri çekme: %d %s", r.status, r.raw)
	}
	r = w.do("POST", path, w.ali, map[string]any{"adds": []string{w.sections["SEC303"]}})
	if r := w.do("POST", "/api/v1/add-drop-requests/"+r.body["id"].(string)+"/reject", w.advisor, map[string]string{"note": "Gerek yok"},
		"If-Match", etag(r)); r.status != 200 || r.body["status"] != "REJECTED" {
		t.Fatalf("red: %d %s", r.status, r.raw)
	}
	if n := w.count(`SELECT enrolled_count FROM offering.sections WHERE id = $1`, w.sections["SEC303"]); n != 0 {
		t.Errorf("red ve geri çekmede kontenjan iade edilmeli: %d", n)
	}
	if list := w.do("GET", path, w.ali, nil).items(); len(list) != 3 {
		t.Errorf("talepler = %d, 3 olmalı", len(list))
	}

	// Bildirimler: her talep danışmana; danışmanın onayladığı bölüm başkanına; sonuç öğrenciye.
	pending := "registration.add_drop_pending:ADVISOR"
	if got := w.notifications(w.advisor); !slices.Equal(got, []string{"registration.submitted", pending, pending, pending}) {
		t.Errorf("danışmanın bildirimleri: %v", got)
	}
	if got := w.notifications(w.head); !slices.Equal(got, []string{"registration.add_drop_pending:DEPT_HEAD"}) {
		t.Errorf("bölüm başkanının bildirimleri: %v", got)
	}
	if got := w.notifications(w.ali); !slices.Equal(got, []string{"registration.decision:APPROVED",
		"registration.add_drop_decision:APPLIED", "registration.add_drop_decision:REJECTED"}) {
		t.Errorf("öğrencinin bildirimleri: %v", got)
	}
}
