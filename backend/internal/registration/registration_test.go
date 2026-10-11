package registration_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func codes(r res) []string {
	var out []string
	for _, v := range r.body["violations"].([]any) {
		out = append(out, obj(v)["code"].(string)+":"+obj(v)["severity"].(string))
	}
	return out
}

func (w *world) add(user, sp, course string) res {
	w.t.Helper()
	return w.do("POST", "/api/v1/student-programs/"+sp+"/registration/items", user, map[string]string{"section_id": w.sections[course]})
}

func (w *world) registration(user, sp string) res {
	w.t.Helper()
	r := w.do("GET", "/api/v1/student-programs/"+sp+"/registration", user, nil)
	if r.status != http.StatusOK {
		w.t.Fatalf("kayıt: %d %s", r.status, r.raw)
	}
	return r
}

func etag(r res) string {
	return fmt.Sprintf(`"%v"`, r.body["version"])
}

func itemFor(r res, course string) string {
	for _, it := range r.body["items"].([]any) {
		if obj(obj(obj(it)["section"])["course"])["code"] == course {
			return obj(it)["id"].(string)
		}
	}
	return ""
}

func TestOffer(t *testing.T) {
	w := newWorld(t)
	r := w.do("GET", "/api/v1/student-programs/"+w.aliSP+"/registration/offer", w.ali, nil)
	if r.status != http.StatusOK {
		t.Fatalf("alınabilir dersler: %d %s", r.status, r.raw)
	}
	if r.body["ects_limit"].(float64) != 40 || r.body["semester"].(float64) != 3 {
		t.Errorf("sınır/yarıyıl = %v/%v", r.body["ects_limit"], r.body["semester"])
	}
	got := map[string]map[string]any{}
	var order []string
	for _, it := range r.items() {
		code := obj(obj(it)["course"])["code"].(string)
		got[code] = obj(it)
		order = append(order, code)
	}
	for code, want := range map[string]string{
		"MAT111": "REPEAT", "VY201": "CURRENT", "ALG201": "CURRENT", "DB201": "CURRENT",
		"SEC301": "ELECTIVE", "SEC302": "ELECTIVE", "SEC303": "ELECTIVE", "OS301": "UPPER",
	} {
		if got[code]["category"] != want {
			t.Errorf("%s kategorisi = %v, want %s", code, got[code]["category"], want)
		}
	}
	// Eski koddan kalınan ders yeni eşdeğeriyle, alttan ve tekrar olarak görünür.
	if got["MAT111"]["kind"] != "REPEAT_FAILED" || got["MAT111"]["last_letter"] != "F2" || obj(got["MAT111"]["line"])["code"] != "MAT101" {
		t.Errorf("MAT111 = %v", got["MAT111"])
	}
	if p := obj(got["VY201"]["prerequisite"]); p["code"] != "PREREQUISITE" {
		t.Errorf("VY201 ön koşul = %v", got["VY201"]["prerequisite"])
	}
	if order[0] != "MAT111" || order[len(order)-1] != "OS301" {
		t.Errorf("sıra = %v (alttan önce, üst yarıyıl sonda)", order)
	}
	sec := obj(got["SEC302"]["sections"].([]any)[0])
	if sec["quota_mode"] != "RESERVED" || sec["quota_left"].(float64) != 0 || sec["available"] != false {
		t.Errorf("başka programa ayrılmış şube = %v", sec)
	}
	if slot := obj(obj(got["DB201"]["sections"].([]any)[0])["slots"].([]any)[0]); slot["start"] != "10:00" || slot["day"].(float64) != 1 {
		t.Errorf("program = %v", slot)
	}
}

func TestBasketAndWorkflow(t *testing.T) {
	w := newWorld(t)

	// --- Kural ihlalleri eklemeyi engeller.
	if r := w.add(w.ali, w.aliSP, "VY201"); r.status != 422 || codes(r)[0] != "PREREQUISITE:ERROR" {
		t.Fatalf("ön koşul: %d %s", r.status, r.raw)
	}
	r := w.add(w.ali, w.aliSP, "MAT111")
	if r.status != http.StatusCreated || r.body["status"] != "DRAFT" || len(r.body["items"].([]any)) != 1 {
		t.Fatalf("ekleme: %d %s", r.status, r.raw)
	}
	if kind := obj(r.body["items"].([]any)[0])["kind"]; kind != "REPEAT_FAILED" {
		t.Errorf("tür = %v", kind)
	}
	// MAT101'den F2 ile kaldı (derse devam etti): çakışma uyarıdır.
	r = w.add(w.ali, w.aliSP, "DB201")
	if r.status != http.StatusCreated || len(r.body["warnings"].([]any)) != 1 ||
		obj(r.body["warnings"].([]any)[0])["code"] != "SCHEDULE_CONFLICT" {
		t.Fatalf("uyarılı çakışma: %d %s", r.status, r.raw)
	}
	if r := w.add(w.ali, w.aliSP, "ALG201"); r.status != http.StatusCreated {
		t.Fatalf("ALG201: %d %s", r.status, r.raw)
	}
	if r := w.add(w.ali, w.aliSP, "SEC303"); r.status != 422 || codes(r)[0] != "SCHEDULE_CONFLICT:ERROR" {
		t.Errorf("çakışma hatası: %d %s", r.status, r.raw)
	}
	if r := w.add(w.ali, w.aliSP, "ALG201"); r.status != 422 || codes(r)[0] != "DUPLICATE_COURSE:ERROR" {
		t.Errorf("aynı ders: %d %s", r.status, r.raw)
	}
	if r := w.add(w.ali, w.aliSP, "SEC302"); r.status != 409 || r.body["code"] != "SECTION_NOT_FOR_PROGRAM" {
		t.Errorf("başka programın kontenjanı: %d %s", r.status, r.raw)
	}
	if r := w.add(w.ali, w.aliSP, "SEC301"); r.status != http.StatusCreated {
		t.Fatalf("SEC301: %d %s", r.status, r.raw)
	}
	if r := w.add(w.berk, w.berkSP, "SEC301"); r.status != 409 || r.body["code"] != "SECTION_FULL" {
		t.Errorf("dolu şube: %d %s", r.status, r.raw)
	}
	// Başkasının sepetine ekleme: danışman bile ders seçemez, başka öğrenci kaydı göremez.
	if r := w.add(w.berk, w.aliSP, "OS301"); r.status != 404 {
		t.Errorf("başka öğrencinin sepeti: %d", r.status)
	}

	r = w.registration(w.ali, w.aliSP)
	if r.body["total_ects"].(float64) != 20 || r.body["ects_limit"].(float64) != 40 || r.body["evaluation"].(map[string]any)["blocking"] != false {
		t.Errorf("sepet = %v / %v / %v", r.body["total_ects"], r.body["ects_limit"], r.body["evaluation"])
	}
	if a := obj(r.body["actions"]); a["can_edit"] != true || a["can_submit"] != true || a["can_approve"] != false {
		t.Errorf("öğrencinin işlemleri = %v", a)
	}
	var enrolled int
	_ = w.pool.QueryRow(context.Background(), `SELECT enrolled_count FROM offering.sections WHERE id = $1`, w.sections["ALG201"]).Scan(&enrolled)
	if enrolled != 1 {
		t.Errorf("ALG201 mevcudu = %d", enrolled)
	}

	// --- Şube değiştirme ve çıkarma kontenjanı iade eder.
	dbItem := itemFor(r, "DB201")
	if r := w.do("PUT", "/api/v1/student-programs/"+w.aliSP+"/registration/items/"+dbItem, w.ali,
		map[string]string{"section_id": w.sections["OS301"]}); r.status != 400 {
		t.Errorf("başka dersin şubesine geçiş: %d %s", r.status, r.raw)
	}
	if r := w.do("DELETE", "/api/v1/student-programs/"+w.aliSP+"/registration/items/"+dbItem, w.ali, nil); r.status != 200 ||
		len(r.body["items"].([]any)) != 3 {
		t.Fatalf("çıkarma: %d %s", r.status, r.raw)
	}
	_ = w.pool.QueryRow(context.Background(), `SELECT enrolled_count FROM offering.sections WHERE id = $1`, w.sections["DB201"]).Scan(&enrolled)
	if enrolled != 0 {
		t.Errorf("DB201 mevcudu iade edilmedi: %d", enrolled)
	}

	// --- Gönderim
	submit := "/api/v1/student-programs/" + w.aliSP + "/registration/submit"
	if r := w.do("POST", submit, w.ali, nil); r.status != 428 {
		t.Errorf("If-Match olmadan: %d", r.status)
	}
	r = w.registration(w.ali, w.aliSP)
	if s := w.do("POST", submit, w.ali, nil, "If-Match", `"999"`); s.status != 412 {
		t.Errorf("eski sürüm: %d", s.status)
	}
	r = w.do("POST", submit, w.ali, nil, "If-Match", etag(r))
	if r.status != 200 || r.body["status"] != "SUBMITTED" || r.body["max_ects_allowed"].(float64) != 40 {
		t.Fatalf("gönderim: %d %s", r.status, r.raw)
	}
	if r := w.add(w.ali, w.aliSP, "OS301"); r.status != 409 || r.body["code"] != "REGISTRATION_LOCKED" {
		t.Errorf("gönderilmiş sepete ekleme: %d %s", r.status, r.raw)
	}

	// --- Danışman listesi ve kararlar
	list := w.do("GET", "/api/v1/me/advisee-registrations", w.advisor, nil)
	if items := list.items(); len(items) != 2 || obj(items[0])["status"] != "SUBMITTED" || obj(items[1])["status"] != "NOT_STARTED" {
		t.Fatalf("danışman listesi = %s", list.raw)
	}
	regID := r.body["id"].(string)
	reject := "/api/v1/registrations/" + regID + "/reject"
	if r := w.do("POST", reject, w.head, map[string]string{"note": "Uygun değil"}, "If-Match", etag(r)); r.status != 403 {
		t.Errorf("danışman olmayan karar veremez: %d %s", r.status, r.raw)
	}
	if r := w.do("POST", reject, w.advisor, map[string]string{"note": ""}, "If-Match", etag(r)); r.status != 400 {
		t.Errorf("gerekçesiz red: %d", r.status)
	}
	r = w.do("POST", reject, w.advisor, map[string]string{"note": "Seçmeli yerine OS301 alın"}, "If-Match", etag(r))
	if r.status != 200 || r.body["status"] != "REJECTED" || r.body["decision_note"] != "Seçmeli yerine OS301 alın" {
		t.Fatalf("red: %d %s", r.status, r.raw)
	}

	// Reddedilen sepet düzenlenince taslağa döner.
	secItem := itemFor(w.registration(w.ali, w.aliSP), "SEC301")
	if r := w.do("DELETE", "/api/v1/student-programs/"+w.aliSP+"/registration/items/"+secItem, w.ali, nil); r.status != 200 ||
		r.body["status"] != "DRAFT" {
		t.Fatalf("red sonrası düzenleme: %d %s", r.status, r.raw)
	}
	if r := w.add(w.ali, w.aliSP, "OS301"); r.status != http.StatusCreated {
		t.Fatalf("OS301: %d %s", r.status, r.raw)
	}
	r = w.do("POST", submit, w.ali, nil, "If-Match", etag(w.registration(w.ali, w.aliSP)))
	if r.status != 200 {
		t.Fatalf("yeniden gönderim: %d %s", r.status, r.raw)
	}
	if a := obj(w.do("GET", "/api/v1/registrations/"+regID, w.advisor, nil).body["actions"]); a["can_approve"] != true || a["can_edit"] != false {
		t.Errorf("danışmanın işlemleri = %v", a)
	}

	approve := "/api/v1/registrations/" + regID + "/approve"
	if r := w.do("POST", approve, w.advisor, map[string]string{}, "If-Match", `"1"`); r.status != 412 {
		t.Errorf("eski sürümle onay: %d", r.status)
	}
	r = w.do("POST", approve, w.advisor, map[string]string{}, "If-Match", etag(r))
	if r.status != 200 || r.body["status"] != "APPROVED" {
		t.Fatalf("onay: %d %s", r.status, r.raw)
	}
	var n, attempt int
	_ = w.pool.QueryRow(context.Background(), `SELECT count(*) FROM enrollment.enrollments WHERE student_program_id = $1 AND status = 'ENROLLED'`, w.aliSP).Scan(&n)
	_ = w.pool.QueryRow(context.Background(), `
		SELECT attempt_no FROM enrollment.enrollments WHERE student_program_id = $1 AND course_id = $2`, w.aliSP, w.courses["MAT111"]).Scan(&attempt)
	if n != 3 || attempt != 2 {
		t.Errorf("kesin kayıtlar = %d, MAT111 denemesi = %d (MAT101'in ikinci alınışı)", n, attempt)
	}
	progress := w.do("GET", "/api/v1/student-programs/"+w.aliSP+"/progress", w.ali, nil)
	for _, l := range progress.body["lines"].([]any) {
		if obj(l)["code"] == "MAT101" && obj(l)["status"] != "IN_PROGRESS" {
			t.Errorf("onaydan sonra MAT101 = %v", obj(l)["status"])
		}
	}

	// --- Onayı geri alma kesin kayıtları siler.
	r = w.do("POST", "/api/v1/registrations/"+regID+"/revoke", w.advisor, map[string]string{"note": "Öğrenci talebiyle"}, "If-Match", etag(r))
	if r.status != 200 || r.body["status"] != "REJECTED" {
		t.Fatalf("geri alma: %d %s", r.status, r.raw)
	}
	_ = w.pool.QueryRow(context.Background(), `SELECT count(*) FROM enrollment.enrollments WHERE student_program_id = $1 AND status = 'ENROLLED'`, w.aliSP).Scan(&n)
	if n != 0 {
		t.Errorf("geri alınan onayın %d kesin kaydı kaldı", n)
	}

	events := w.do("GET", "/api/v1/student-programs/"+w.aliSP+"/registration/events", w.advisor, nil).items()
	var types []string
	for _, e := range events {
		types = append(types, obj(e)["type"].(string))
	}
	want := "CREATED ITEM_ADDED ITEM_ADDED ITEM_ADDED ITEM_ADDED ITEM_REMOVED SUBMITTED REJECTED RETURNED_TO_DRAFT ITEM_REMOVED ITEM_ADDED SUBMITTED APPROVED APPROVAL_REVOKED"
	if strings.Join(types, " ") != want {
		t.Errorf("olaylar = %v", types)
	}
}

func TestWindowsHoldsAndOverride(t *testing.T) {
	w := newWorld(t)
	if r := w.add(w.berk, w.berkSP, "ALG201"); r.status != http.StatusCreated {
		t.Fatalf("ekleme: %d %s", r.status, r.raw)
	}
	r := w.do("POST", "/api/v1/student-programs/"+w.berkSP+"/registration/submit", w.berk, nil,
		"If-Match", etag(w.registration(w.berk, w.berkSP)))
	if r.status != 200 {
		t.Fatalf("gönderim: %d %s", r.status, r.raw)
	}
	regID := r.body["id"].(string)

	// Pencereler kapandı (21 Eylül sonrası ders seçme, 23 Eylül sonrası onay).
	w.now = time.Date(2026, time.September, 25, 9, 0, 0, 0, time.UTC)
	if r := w.add(w.ali, w.aliSP, "ALG201"); r.status != 409 || r.body["code"] != "WINDOW_CLOSED" {
		t.Errorf("kapalı pencerede ekleme: %d %s", r.status, r.raw)
	}
	approve := "/api/v1/registrations/" + regID + "/approve"
	if r := w.do("POST", approve, w.advisor, map[string]string{}, "If-Match", etag(r)); r.status != 409 || r.body["code"] != "WINDOW_CLOSED" {
		t.Errorf("kapalı pencerede danışman onayı: %d %s", r.status, r.raw)
	}
	// Öğrenci işleri pencere dışında gerekçeyle müdahale eder; yetki iki adımlı doğrulama ister.
	if r := w.do("POST", approve, w.registrar, map[string]string{"note": "Danışman raporlu"}, "If-Match", etag(r)); r.status != 403 {
		t.Errorf("iki adımlı doğrulamasız müdahale: %d", r.status)
	}
	if r := w.do("POST", approve, w.registrar, map[string]string{"note": ""}, "If-Match", etag(r), "X-Test-MFA", "1"); r.status != 400 {
		t.Errorf("gerekçesiz müdahale: %d", r.status)
	}
	r = w.do("POST", approve, w.registrar, map[string]string{"note": "Danışman raporlu, fakülte kararı"}, "If-Match", etag(r), "X-Test-MFA", "1")
	if r.status != 200 || r.body["status"] != "APPROVED" {
		t.Fatalf("müdahale: %d %s", r.status, r.raw)
	}
	var override bool
	_ = w.pool.QueryRow(context.Background(), `
		SELECT (payload->>'override')::bool FROM enrollment.registration_events WHERE registration_id = $1 AND event_type = 'APPROVED'`,
		regID).Scan(&override)
	if !override {
		t.Error("müdahale olay günlüğünde işaretlenmeli")
	}

	// Kayda özel ek süre: pencere kapalıyken öğrenci sepetini düzenleyebilir.
	ext := "/api/v1/student-programs/" + w.aliSP + "/registration/extension"
	until := w.now.Add(48 * time.Hour).Format(time.RFC3339)
	if r := w.do("POST", ext, w.advisor, map[string]string{"until": until, "reason": "Sağlık raporu"}); r.status != 403 {
		t.Errorf("danışman ek süre tanıyamaz: %d", r.status)
	}
	r = w.do("POST", ext, w.registrar, map[string]string{"until": until, "reason": "Sağlık raporu"}, "X-Test-MFA", "1")
	if r.status != 200 || r.body["extension_until"] == nil || obj(r.body["registration_window"])["extended"] != true {
		t.Fatalf("ek süre: %d %s", r.status, r.raw)
	}
	if r := w.add(w.ali, w.aliSP, "ALG201"); r.status != http.StatusCreated {
		t.Errorf("ek sürede ekleme: %d %s", r.status, r.raw)
	}

	// Ders seçmeyi kapatan engel: hiçbir ders eklenemez.
	w.now = time.Date(2026, time.September, 10, 9, 0, 0, 0, time.UTC)
	w.exec(`INSERT INTO enrollment.holds (student_program_id, hold_type, blocks, reason)
		VALUES ('` + w.aliSP + `', 'TUITION_DEBT', '{REGISTRATION,DOCUMENT}', 'Katkı payı')`)
	if r := w.add(w.ali, w.aliSP, "MAT111"); r.status != 422 || codes(r)[0] != "HOLD:ERROR" {
		t.Errorf("engel: %d %s", r.status, r.raw)
	}
	if ev := obj(w.registration(w.ali, w.aliSP).body["evaluation"]); ev["blocking"] != true {
		t.Errorf("engelli sepet gönderilememeli: %v", ev)
	}
}

func TestBulkApprove(t *testing.T) {
	w := newWorld(t)
	var regs []map[string]any
	for _, s := range []struct{ user, sp, course string }{{w.ali, w.aliSP, "MAT111"}, {w.berk, w.berkSP, "ALG201"}} {
		if r := w.add(s.user, s.sp, s.course); r.status != http.StatusCreated {
			t.Fatalf("ekleme: %d %s", r.status, r.raw)
		}
		r := w.do("POST", "/api/v1/student-programs/"+s.sp+"/registration/submit", s.user, nil, "If-Match", etag(w.registration(s.user, s.sp)))
		if r.status != 200 {
			t.Fatalf("gönderim: %d %s", r.status, r.raw)
		}
		regs = append(regs, r.body)
	}
	r := w.do("POST", "/api/v1/registrations/bulk-approve", w.advisor, map[string]any{"items": []map[string]any{
		{"id": regs[0]["id"], "version": regs[0]["version"]},
		{"id": regs[1]["id"], "version": 1}, // eski sürüm
	}})
	if r.status != 200 || r.body["approved"].(float64) != 1 || r.body["failed"].(float64) != 1 {
		t.Fatalf("toplu onay: %d %s", r.status, r.raw)
	}
	if res := obj(r.body["results"].([]any)[1]); res["code"] != "VERSION_MISMATCH" {
		t.Errorf("ikinci sonuç = %v", res)
	}
	if r := w.do("POST", "/api/v1/registrations/bulk-approve", w.ali, map[string]any{"items": []map[string]any{{"id": regs[0]["id"], "version": 1}}}); r.status != 403 {
		t.Errorf("öğrenci toplu onay yapamaz: %d", r.status)
	}
}
