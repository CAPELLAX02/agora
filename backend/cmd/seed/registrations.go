package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/registration"
)

// Ders seçme demosu. Gerçek takvimde 2026 güz ders seçmesi geçmişte kalır (Eylül). Ders
// seçme ekranlarının her gün denenebilmesi için Bilgisayar Mühendisliği programına, seed'in
// çalıştığı andan itibaren üç hafta açık kalan program kapsamlı pencereler eklenir (yol
// haritası §5 #13). Pencere motoru en dar kapsamı uyguladığı için sadece bu program etkilenir;
// üniversite takvimi değişmez. Pencereler takvim ekranında görünür ve oradan kapatılabilir.

const demoWindowNote = "Geliştirme ortamı demo penceresi: seed'in çalıştığı andan itibaren üç hafta açık kalır."

var demoWindowTypes = []demoWindowType{
	{"COURSE_REGISTRATION", "Ders seçme (demo)", "Course registration (demo)"},
	{"ADVISOR_APPROVAL", "Danışman onayı (demo)", "Advisor approval (demo)"},
	{"ADD_DROP_STUDENT", "Ekle-bırak (demo)", "Add-drop (demo)"},
	{"ADD_DROP_ADVISOR", "Ekle-bırak danışman onayı (demo)", "Add-drop advisor approval (demo)"},
	{"ADD_DROP_DEPT_HEAD", "Ekle-bırak bölüm başkanı onayı (demo)", "Add-drop head of department approval (demo)"},
}

// openDemoWindows, demo pencerelerini açar ya da süresi azalmışsa yeniler: BM programı için
// ders seçme pencereleri, Mühendislik Fakültesi için not girişi pencereleri (bkz. grades.go).
func openDemoWindows(ctx context.Context, pool *pgxpool.Pool, now time.Time) error {
	var termID, programID, facultyID string
	err := pool.QueryRow(ctx, `
		SELECT t.id, p.id, f.id FROM academic.terms t, org.programs p, org.faculties f
		WHERE t.is_current AND p.code = 'BIL-EN-NO' AND f.code = 'MUH'`).Scan(&termID, &programID, &facultyID)
	if errors.Is(err, pgx.ErrNoRows) {
		fmt.Println("ATLANDI    demo pencereleri: aktif dönem, BIL-EN-NO ya da MUH yok")
		return nil
	}
	if err != nil {
		return fmt.Errorf("demo pencereleri: %w", err)
	}
	if err := openWindows(ctx, pool, now, termID, "PROGRAM", programID, demoWindowTypes); err != nil {
		return err
	}
	return openWindows(ctx, pool, now, termID, "FACULTY", facultyID, demoGradeWindowTypes)
}

type demoWindowType struct{ code, titleTR, titleEN string }

// openWindows, verilen kapsamda her tür için üç hafta açık kalan bir demo penceresi açar; bir
// haftadan az kalmışsa yeniler. Aynı kapsamda o türün başka bir olayı varsa dokunmaz.
func openWindows(ctx context.Context, pool *pgxpool.Pool, now time.Time, termID, scopeType, scopeID string, types []demoWindowType) error {
	start, end := now.Add(-time.Hour).Truncate(time.Hour), now.Add(21*24*time.Hour).Truncate(time.Hour)
	for _, t := range types {
		tag, err := pool.Exec(ctx, `
			UPDATE academic.calendar_events SET period = tstzrange($4, $5, '[)'), version = version + 1, updated_at = now()
			WHERE term_id = $1 AND event_type_code = $2 AND scope_type = $8 AND scope_id = $3 AND note = $6
			  AND upper(period) < $7`,
			termID, t.code, scopeID, start, end, demoWindowNote, now.Add(7*24*time.Hour), scopeType)
		if err != nil {
			return fmt.Errorf("demo penceresi %s yenilenemedi: %w", t.code, err)
		}
		if tag.RowsAffected() > 0 {
			fmt.Printf("YENİLENDİ  demo penceresi %s\n", t.code)
			continue
		}
		tag, err = pool.Exec(ctx, `
			INSERT INTO academic.calendar_events (term_id, event_type_code, title_tr, title_en, period, scope_type, scope_id, note)
			SELECT $1, $2, $3, $4, tstzrange($6, $7, '[)'), $9, $5, $8
			WHERE NOT EXISTS (
				SELECT 1 FROM academic.calendar_events
				WHERE term_id = $1 AND event_type_code = $2 AND scope_type = $9 AND scope_id = $5)`,
			termID, t.code, t.titleTR, t.titleEN, scopeID, start, end, demoWindowNote, scopeType)
		if err != nil {
			return fmt.Errorf("demo penceresi %s açılamadı: %w", t.code, err)
		}
		if tag.RowsAffected() > 0 {
			fmt.Printf("AÇILDI     demo penceresi %s (%s'e kadar)\n", t.code, end.Format("2006-01-02"))
		}
	}
	return nil
}

// demoRegistrations: P10001'in danışman kuyruğunda farklı durumlar ve 1. sınıf demo grubunun
// onaylanmış kayıtları. Sepetler ders seçme servisiyle (kurallar ve kontenjan denetimiyle)
// kurulur; onayı öğrencinin danışmanı verir.
var demoRegistrations = append([]demoRegistration{
	{"23290013", true, false},  // onay bekliyor
	{"25290016", true, false},  // onay bekliyor
	{"26290017", true, false},  // onay bekliyor (1. sınıf: dönemin bütün zorunlu dersleri)
	{"23290015", true, true},   // onaylanmış: ekle-bırak denenebilir
	{"24290012", false, false}, // taslak (30 AKTS sınırı, alttan ders)
}, cohortRegistrations()...)

type demoRegistration struct {
	no      string
	submit  bool
	approve bool
}

// seedRegistrations, demo öğrencilerinin 2026 güz ders seçme kayıtlarını oluşturur. Kaydı
// olan öğrenciye dokunmaz.
func seedRegistrations(ctx context.Context, pool *pgxpool.Pool) error {
	termID, _, err := registration.NewRepository(pool).CurrentTerm(ctx)
	if errors.Is(err, registration.ErrNoTerm) {
		return nil
	}
	if err != nil {
		return err
	}
	svc := registration.NewService(pool, time.Now, nil)
	students := enrollment.NewRepository(pool)

	for _, d := range demoRegistrations {
		var spID string
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT sp.id, EXISTS (SELECT 1 FROM enrollment.registrations r WHERE r.student_program_id = sp.id AND r.term_id = $2)
			FROM enrollment.student_programs sp JOIN people.students s ON s.id = sp.student_id
			JOIN org.programs p ON p.id = sp.program_id
			WHERE s.student_no = $1 AND p.code = 'BIL-EN-NO'`, d.no, termID).Scan(&spID, &exists)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		sp, err := students.StudentProgram(ctx, spID)
		if err != nil {
			return err
		}
		added, err := autoBasket(ctx, svc, sp, termID)
		if err != nil {
			return fmt.Errorf("%s sepeti kurulamadı: %w", d.no, err)
		}
		status := "taslak"
		if d.submit {
			reg, err := registration.NewRepository(pool).Registration(ctx, sp.ID, sp.Program.ID, termID)
			if err != nil {
				return err
			}
			if err := svc.Submit(ctx, "", sp, termID, reg.Version); err != nil {
				return fmt.Errorf("%s gönderilemedi: %w", d.no, err)
			}
			status = "onay bekliyor"
			if d.approve {
				reg, err = registration.NewRepository(pool).Registration(ctx, sp.ID, sp.Program.ID, termID)
				if err != nil {
					return err
				}
				var advisorUser string
				if err := pool.QueryRow(ctx, `
					SELECT u.id FROM enrollment.advisor_assignments aa
					JOIN people.staff st ON st.id = aa.advisor_staff_id JOIN iam.users u ON u.person_id = st.person_id
					WHERE aa.student_program_id = $1 AND aa.valid_until IS NULL`, sp.ID).Scan(&advisorUser); err != nil {
					return fmt.Errorf("%s danışmanı bulunamadı: %w", d.no, err)
				}
				if err := svc.Decide(ctx, registration.Decider{UserID: advisorUser}, sp, reg.ID, reg.Version, registration.Approve, "Başarılar."); err != nil {
					return fmt.Errorf("%s onaylanamadı: %w", d.no, err)
				}
				status = "onaylandı"
			}
		}
		fmt.Printf("DERS SEÇME %-9s %d ders, %s\n", d.no, added, status)
	}
	return nil
}

// autoBasket, öğrencinin sepetini bir öğrencinin yapacağı gibi doldurur: önce alttan dersler,
// sonra dönem dersleri, sonra yarıyılına gelmiş ve dolmamış seçmeli yuvaları. Ön koşulu sağlanmayan, kurala
// takılan (çakışma, AKTS sınırı) ya da dolu şubeler atlanır, varsa dersin başka şubesi denenir.
func autoBasket(ctx context.Context, svc *registration.Service, sp enrollment.StudentProgram, termID string) (int, error) {
	offers, _, err := svc.Offers(ctx, sp, termID)
	if err != nil {
		return 0, err
	}
	taken := map[string]int{} // seçmeli yuvası → eklenen ders
	added := 0
	for _, o := range offers {
		if o.Category == registration.CategoryUpper || o.Category == registration.CategoryUpgrade || o.Prerequisite != nil {
			continue
		}
		// Seçmeliler: yarıyılına gelmiş (ya da geçmiş) yuvalar, gereken sayı kadar.
		if o.Category == registration.CategoryElective &&
			(o.Line.Semester > sp.CurrentSemester || taken[o.Line.ID] >= o.Remaining) {
			continue
		}
		for _, sec := range o.Sections {
			_, err := svc.AddItem(ctx, "", sp, sec.ID)
			var re *registration.RuleError
			switch {
			case err == nil:
				added++
				taken[o.Line.ID]++
			case errors.As(err, &re), errors.Is(err, registration.ErrSectionFull),
				errors.Is(err, registration.ErrQuotaFull), errors.Is(err, registration.ErrNotForProgram):
				continue
			default:
				return added, err
			}
			break
		}
	}
	return added, nil
}
