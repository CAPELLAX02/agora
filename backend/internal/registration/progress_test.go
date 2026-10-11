package registration_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestProgressAndHistory(t *testing.T) {
	w := newWorld(t)
	progress := "/api/v1/student-programs/" + w.aliSP + "/progress"

	r := w.do("GET", progress, w.ali, nil)
	if r.status != http.StatusOK {
		t.Fatalf("ders durumu: %d %s", r.status, r.raw)
	}
	lines := map[string]map[string]any{}
	for _, l := range r.body["lines"].([]any) {
		key := obj(l)["code"].(string)
		if obj(l)["item_type"] == "ELECTIVE_SLOT" {
			key += fmt.Sprint(obj(l)["semester_no"])
		}
		lines[key] = obj(l)
	}
	for code, want := range map[string]string{
		"MAT101": "FAILED", "PRG101": "PASSED", "PRG102": "FAILED", "VY201": "NOT_TAKEN", "OS301": "NOT_TAKEN",
	} {
		if got := lines[code]["status"]; got != want {
			t.Errorf("%s = %v, want %s", code, got, want)
		}
	}
	if last := obj(lines["PRG102"]["last_attempt"]); last["letter"] != "F1" || last["term_code"] != "2025-SPRING" {
		t.Errorf("PRG102 son deneme = %v", last)
	}
	if lines["TEKSEC3"]["status"] != "NOT_TAKEN" || lines["TEKSEC3"]["remaining"].(float64) != 1 ||
		lines["TEKSEC5"]["remaining"].(float64) != 2 {
		t.Errorf("seçmeli yuvaları = %v / %v", lines["TEKSEC3"], lines["TEKSEC5"])
	}
	sum := obj(r.body["summary"])
	// PRG101 7 + FIZ101 5 + MAT102 6 = 18; müfredat 6+7+5+7+6+6+5+5+5+4+8 = 64.
	if sum["completed_ects"].(float64) != 18 || sum["required_ects"].(float64) != 64 || sum["remaining_courses"].(float64) != 9 {
		t.Errorf("özet = %v", sum)
	}
	if r.body["gpa"].(float64) != 2.4 || r.body["current_semester"].(float64) != 3 {
		t.Errorf("GANO/yarıyıl = %v/%v", r.body["gpa"], r.body["current_semester"])
	}

	// Erişim: danışman ve bölüm görür; başka öğrenci kaydın varlığını bile öğrenemez.
	for name, c := range map[string]struct {
		user string
		want int
	}{
		"danışman": {w.advisor, 200}, "bölüm başkanı": {w.head, 200}, "başka öğrenci": {w.berk, 404},
	} {
		if r := w.do("GET", progress, c.user, nil); r.status != c.want {
			t.Errorf("%s: %d, want %d", name, r.status, c.want)
		}
	}

	history := w.do("GET", "/api/v1/student-programs/"+w.aliSP+"/history", w.advisor, nil).items()
	if len(history) != 5 {
		t.Fatalf("ders geçmişi = %v", history)
	}
	first := obj(history[0])
	if obj(first["term"])["code"] != "2025-FALL" || first["attempt_no"].(float64) != 1 || first["source"] != "IMPORT" {
		t.Errorf("ilk kayıt = %v", first)
	}
	for _, h := range history {
		if h := obj(h); obj(h["course"])["code"] == "PRG102" && (h["attendance_fail"] != true || h["is_passing"] != false) {
			t.Errorf("devamsızlık kaydı = %v", h)
		}
	}
}
