package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
)

// absentLetter, demo notlarında devamsızlıktan kalmayı gösterir: dönemin ölçeğindeki
// devamsızlık harfi verilir (2023 öncesi F1, sonrası F4).
const absentLetter = "DEVAMSIZ"

// demoStudent, Bilgisayar Mühendisliği (İngilizce) programına kayıtlı bir demo öğrencisidir.
// Geçmiş dönem notları üretilir (bkz. history.go); letters, demo senaryoları için belirli
// derslerin notlarını sabitler: sırayla her denemenin harf notu. Listenin son notu kalma
// notuysa ders o dönemden sonra tekrar alınmamıştır (bu dönem alttan dersidir).
type demoStudent struct {
	no         string
	admission  int
	semester   int // 2026 güzünde hazırlık hariç kaçıncı yarıyıl
	classLevel int
	ability    float64 // not dağılımı: 0 ortalama öğrenci, artı başarılı
	clean      bool    // hiç dersten kalmamış
	letters    map[string][]string
	hold       enrollment.HoldType // ders seçmeyi kapatan engel
	holdReason string
	advisor    string // danışmanın personel numarası; boşsa P10001
}

// Demo öğrencileri. 2022 girişliler hazırlık okumuştur (2026 güzünde 7. yarıyıl).
var demoStudents = append([]demoStudent{
	{no: "22290001", admission: 2022, semester: 7, classLevel: 4, ability: 1.4, clean: true},
	// Alttan ders ve eşdeğerlik: Fizik I'den iki kez kaldı (artık yeni kodu PHY0105 açılıyor),
	// İşletim Sistemleri'nden kaldı ama derse devam etti (çakışma uyarı olur).
	{no: "22290002", admission: 2022, semester: 7, classLevel: 4, ability: 0.25,
		letters: map[string][]string{"PHY0101": {"F2", "F2"}, "COM3035": {"F2"}}},
	{no: "26290004", admission: 2026, semester: 1, classLevel: 1},
	// Ön koşul: Bilgisayar Programlama II'ye devamsızlıktan kaldı, Veri Yapıları'nı alamaz.
	{no: "25290011", admission: 2025, semester: 3, classLevel: 2, ability: -0.1,
		letters: map[string][]string{"COM1002": {absentLetter}}},
	// Düşük GANO (30 AKTS sınırı) ve alttan ders.
	{no: "24290012", admission: 2024, semester: 5, classLevel: 3, ability: -1.4,
		letters: map[string][]string{"COM2067": {"F2"}}},
	{no: "23290013", admission: 2023, semester: 7, classLevel: 4, ability: 1.7, clean: true},
	// Katkı payı borcu: ders seçemez.
	{no: "24290014", admission: 2024, semester: 5, classLevel: 3, ability: 0.4,
		hold: enrollment.HoldTuitionDebt, holdReason: "2026 güz dönemi katkı payı ödenmedi"},
	{no: "23290015", admission: 2023, semester: 7, classLevel: 4, ability: 0.6},
	{no: "25290016", admission: 2025, semester: 3, classLevel: 2, ability: 0.3, clean: true},
	{no: "26290017", admission: 2026, semester: 1, classLevel: 1},
}, cohortStudents()...)

func demoByNo(no string) (demoStudent, bool) {
	for _, d := range demoStudents {
		if d.no == no {
			return d, true
		}
	}
	return demoStudent{}, false
}

// enrollDemoStudents, demo öğrencilerini Bilgisayar Mühendisliği programına kaydeder,
// danışmanlarını (varsayılan P10001, bölüm danışmanı) atar ve engellerini koyar. Kayıtlı
// öğrencinin yarıyılı ve sınıfı demo tanımına çekilir; tekrar çalıştırılabilir.
func enrollDemoStudents(ctx context.Context, pool *pgxpool.Pool) error {
	svc := enrollment.NewService(pool)

	var programID string
	if err := pool.QueryRow(ctx, `SELECT id FROM org.programs WHERE code = 'BIL-EN-NO'`).Scan(&programID); err != nil {
		return fmt.Errorf("BIL-EN-NO programı bulunamadı (önce SQL seed'leri yükleyin): %w", err)
	}
	advisors := map[string]string{} // personel no → personel kimliği
	for _, d := range demoStudents {
		no := cmp.Or(d.advisor, "P10001")
		if _, ok := advisors[no]; ok {
			continue
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM people.staff WHERE staff_no = $1`, no).Scan(&id); err != nil {
			return fmt.Errorf("%s bulunamadı: %w", no, err)
		}
		advisors[no] = id
	}

	for _, d := range demoStudents {
		advisor := cmp.Or(d.advisor, "P10001")
		var studentID string
		if err := pool.QueryRow(ctx, `SELECT id FROM people.students WHERE student_no = $1`, d.no).Scan(&studentID); err != nil {
			return fmt.Errorf("%s bulunamadı: %w", d.no, err)
		}
		spID, err := svc.CreateStudentProgram(ctx, "", enrollment.NewStudentProgram{
			StudentID: studentID, ProgramID: programID, Kind: enrollment.KindMajor,
			AdmissionType: enrollment.AdmissionOSYS, AdmissionYear: d.admission,
			AdmittedOn: time.Date(d.admission, time.September, 19, 0, 0, 0, 0, time.UTC),
			Status:     enrollment.StatusActive, ClassLevel: d.classLevel, CurrentSemester: d.semester,
		})
		switch {
		case errors.Is(err, enrollment.ErrConflict):
			tag, err := pool.Exec(ctx, `
				UPDATE enrollment.student_programs SET current_semester = $3, class_level = $4,
					version = version + 1, updated_at = now()
				WHERE student_id = $1 AND program_id = $2 AND (current_semester, class_level) <> ($3, $4)`,
				studentID, programID, d.semester, d.classLevel)
			if err != nil {
				return fmt.Errorf("%s yarıyılı güncellenemedi: %w", d.no, err)
			}
			if tag.RowsAffected() > 0 {
				fmt.Printf("GÜNCELLENDİ %-9s %d. yarıyıl\n", d.no, d.semester)
			}
			if err := pool.QueryRow(ctx, `SELECT id FROM enrollment.student_programs WHERE student_id = $1 AND program_id = $2`,
				studentID, programID).Scan(&spID); err != nil {
				return err
			}
		case err != nil:
			return fmt.Errorf("%s programa kaydedilemedi: %w", d.no, err)
		default:
			if err := svc.AssignAdvisor(ctx, "", spID, advisors[advisor], "Demo danışmanı"); err != nil {
				return fmt.Errorf("%s danışman atanamadı: %w", d.no, err)
			}
			fmt.Printf("KAYDEDİLDİ %-9s BIL-EN-NO, %d. yarıyıl, danışman %s\n", d.no, d.semester, advisor)
		}

		if d.hold != "" {
			_, err := svc.PlaceHold(ctx, "", spID, d.hold, d.hold.DefaultBlocks(), d.holdReason)
			if err != nil && !errors.Is(err, enrollment.ErrHoldExists) {
				return fmt.Errorf("%s engeli konulamadı: %w", d.no, err)
			}
		}
	}
	return nil
}
