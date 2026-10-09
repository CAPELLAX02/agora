package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/offering"
)

// Bilgisayar Mühendisliği 2026 güz dönemi ders açma planı. Her sınıf düzeyi izlediği ders
// planının o yarıyılındaki dersleri alır: 1. sınıf 2026 planının 1. yarıyılını, 2-4. sınıflar
// 2023 (ve 2022) planının 3, 5 ve 7. yarıyıllarını. Program, aynı sınıfın zorunlu dersleri
// birbiriyle, seçmeliler zorunlularla çakışmayacak; derslikler ve öğretim elemanları da
// çakışmayacak şekilde kurulmuştur. Seed, repository'ler üzerinden yazar: aynı çakışma ve
// kapasite denetimlerinden geçer.

type slotSpec struct {
	day        int
	start, end string
	room       string // "BİNA/DERSLİK", boşsa dersliksiz
	kind       string
}

type planPart struct {
	typ    string
	weight float64
}

type sectionSpec struct {
	code        string
	capacity    int
	instructors []string // personel numaraları; ilki sorumlu
	slots       []slotSpec
	quotas      map[string]int // program kodu → kontenjan; doluysa RESERVED
}

type offeringSpec struct {
	course     string
	department string
	sections   []sectionSpec
	plan       []planPart
	lockPlan   bool
}

// block, gün içinde start saatinden başlayan n ders saatlik oturumdur (her saat 50 dakika).
func block(day, startHour, hours int, room, kind string) slotSpec {
	return slotSpec{
		day: day, start: fmt.Sprintf("%02d:00", startHour), end: fmt.Sprintf("%02d:50", startHour+hours-1),
		room: room, kind: kind,
	}
}

func th(day, start, hours int, room string) slotSpec  { return block(day, start, hours, room, "THEORY") }
func lab(day, start, hours int, room string) slotSpec { return block(day, start, hours, room, "LAB") }

const (
	mon = iota + 1
	tue
	wed
	thu
	fri
)

var (
	exam6040  = []planPart{{"MIDTERM", 40}, {"FINAL", 60}}
	exam3010  = []planPart{{"MIDTERM", 30}, {"HOMEWORK", 10}, {"FINAL", 60}}
	exam2x    = []planPart{{"MIDTERM", 25}, {"MIDTERM", 25}, {"FINAL", 50}}
	labCourse = []planPart{{"MIDTERM", 30}, {"LAB", 20}, {"FINAL", 50}}
	project   = []planPart{{"PROJECT", 40}, {"PRESENTATION", 20}, {"FINAL", 40}}
	passFail  = []planPart{{"PARTICIPATION", 40}, {"FINAL", 60}}
)

func single(capacity int, instructors []string, slots ...slotSpec) []sectionSpec {
	return []sectionSpec{{code: "1", capacity: capacity, instructors: instructors, slots: slots}}
}

func staff(nos ...string) []string { return nos }

var fall2026 = []offeringSpec{
	// 1. sınıf (2026 planı, 1. yarıyıl): iki şubeli programlama dersi, ortak amfi.
	{course: "COM1007", department: "BIL", plan: labCourse, lockPlan: true, sections: []sectionSpec{
		{code: "1", capacity: 60, instructors: staff("P10001", "P10007"), slots: []slotSpec{th(mon, 9, 4, "MUH-A/A103"), lab(wed, 13, 2, "MUH-LAB/LAB-1")}},
		{code: "2", capacity: 60, instructors: staff("P10003"), slots: []slotSpec{th(mon, 9, 4, "MUH-B/B103"), lab(wed, 13, 2, "MUH-LAB/LAB-2")}},
	}},
	{course: "COM1015", department: "BIL", plan: exam6040, lockPlan: true, sections: single(120, staff("P10004"), th(tue, 9, 3, "GOL-AMF/AMFI-1"))},
	{course: "MTH0143", department: "MAT", plan: exam2x, sections: single(120, staff("P10010"), th(wed, 9, 2, "GOL-AMF/AMFI-1"), th(thu, 9, 2, "GOL-AMF/AMFI-1"))},
	{course: "PHY0105", department: "FIZ", plan: exam6040, sections: single(120, staff("P10011"), th(thu, 13, 3, "GOL-AMF/AMFI-1"))},
	{course: "PHY0151", department: "FIZ", plan: labCourse, sections: single(120, staff("P10011"), lab(fri, 9, 2, "MUH-LAB/LAB-3"))},
	{course: "TUR171", department: "BIL", plan: exam6040, sections: single(120, nil, th(tue, 13, 2, "GOL-AMF/AMFI-1"))},
	{course: "HIS103", department: "BIL", plan: exam6040, sections: single(120, nil, th(tue, 15, 2, "GOL-AMF/AMFI-1"))},
	{course: "ENG101", department: "BIL", plan: exam3010, sections: single(120, nil, th(fri, 13, 4, "GOL-AMF/AMFI-1"))},
	{course: "OUL101", department: "BIL", plan: passFail, sections: single(120, nil)},

	// 2. sınıf (2023 planı, 3. yarıyıl) ve 2. sınıf teknik seçmelileri.
	{course: "COM2039", department: "BIL", plan: exam6040, lockPlan: true, sections: single(80, staff("P10005"), th(mon, 13, 3, "MUH-A/A105"))},
	{course: "COM2043", department: "BIL", plan: labCourse, sections: single(80, staff("P10006"), th(tue, 9, 3, "MUH-A/A105"), lab(thu, 15, 2, "MUH-LAB/LAB-1"))},
	{course: "COM2067", department: "BIL", plan: labCourse, sections: single(80, staff("P10002"), th(wed, 9, 3, "MUH-A/A105"), lab(wed, 14, 2, "MUH-LAB/LAB-3"))},
	{course: "COM2077", department: "BIL", plan: labCourse, sections: single(80, staff("P10004"), th(thu, 9, 3, "MUH-A/A105"), lab(fri, 13, 2, "MUH-LAB/LAB-2"))},
	{course: "ENG201", department: "BIL", plan: exam3010, sections: single(80, nil, th(fri, 9, 4, "MUH-A/A105"))},
	{course: "COM2501", department: "BIL", plan: exam3010, sections: []sectionSpec{{
		code: "1", capacity: 40, instructors: staff("P10006"), slots: []slotSpec{th(mon, 9, 3, "MUH-A/A101")},
		// Bölüm dışından da öğrenci alan seçmeli: kontenjanın bir kısmı Yapay Zekâ ve Veri Mühendisliğine ayrılmış.
		quotas: map[string]int{"BIL-EN-NO": 30, "YZV-NO": 10},
	}}},
	{course: "COM2537", department: "BIL", plan: project, sections: single(40, staff("P10003"), th(tue, 13, 3, "MUH-A/A101"))},
	{course: "COM2553", department: "BIL", plan: exam6040, sections: single(40, staff("P10005"), th(mon, 9, 3, "MUH-A/A102"))},

	// Üniversite alan dışı seçmelileri (sentetik dersler), bütün sınıflara açık.
	{course: "UNVG101", department: "BIL", plan: project, sections: single(40, nil, th(fri, 15, 2, "MUH-B/B101"))},
	{course: "UNVG103", department: "BIL", plan: exam6040, sections: single(40, nil, th(fri, 15, 2, "MUH-B/B102"))},
	{course: "UNVG105", department: "BIL", plan: project, sections: single(40, nil, th(thu, 13, 2, "MUH-B/B101"))},

	// 3. sınıf (2023 planı, 5. yarıyıl) ve 3. sınıf teknik seçmelileri.
	{course: "COM3025", department: "BIL", plan: labCourse, sections: single(70, staff("P10004"), th(mon, 9, 3, "MUH-A/A104"), lab(tue, 13, 2, "MUH-LAB/LAB-3"))},
	{course: "COM3035", department: "BIL", plan: labCourse, sections: single(70, staff("P10005"), th(tue, 9, 3, "MUH-A/A104"), lab(thu, 13, 2, "MUH-LAB/LAB-4"))},
	{course: "COM3067", department: "BIL", plan: exam3010, sections: single(70, staff("P10001"), th(wed, 9, 3, "MUH-A/A104"), lab(fri, 9, 2, "MUH-LAB/LAB-4"))},
	{course: "COM3071", department: "BIL", plan: exam6040, sections: single(70, staff("P10006"), th(thu, 9, 1, "MUH-A/A104"))},
	{course: "SCS301", department: "BIL", plan: passFail, sections: single(70, nil)},
	{course: "COM3533", department: "BIL", plan: exam6040, sections: single(40, staff("P10002"), th(mon, 13, 3, "MUH-A/A102"))},
	{course: "COM3537", department: "BIL", plan: project, sections: single(40, staff("P10006"), th(wed, 13, 3, "MUH-A/A102"))},
	{course: "COM3549", department: "BIL", plan: exam3010, sections: single(40, staff("P10005"), th(fri, 13, 3, "MUH-A/A102"))},
	{course: "COM3551", department: "BIL", plan: project, sections: single(40, staff("P10001"), th(tue, 15, 3, "MUH-A/A104"))},
	{course: "COM3557", department: "BIL", plan: exam6040, sections: single(40, staff("P10002"), th(thu, 15, 3, "MUH-A/A102"))},

	// 4. sınıf (2022 ve 2023 planları, 7. yarıyıl) ve 4. sınıf teknik seçmelileri.
	{course: "COM4061", department: "BIL", plan: project, sections: single(60, staff("P10002"), th(tue, 9, 2, "MUH-A/A109"), lab(tue, 11, 2, "MUH-LAB/LAB-4"))},
	{course: "COM4099", department: "BIL", plan: passFail, sections: single(60, staff("P10001"))},
	{course: "COM4501", department: "BIL", plan: project, sections: single(40, staff("P10003"), th(mon, 13, 3, "MUH-A/A109"))},
	{course: "COM4503", department: "BIL", plan: exam3010, sections: single(40, staff("P10005"), th(wed, 13, 3, "MUH-A/A109"))},
	{course: "COM4507", department: "BIL", plan: project, sections: single(40, staff("P10004"), th(wed, 9, 3, "MUH-A/A109"))},
	{course: "COM4511", department: "BIL", plan: exam6040, sections: single(40, staff("P10006"), th(fri, 13, 3, "MUH-A/A109"))},
	{course: "COM4515", department: "BIL", plan: exam3010, sections: single(40, staff("P10005"), th(thu, 9, 3, "MUH-A/A109"))},
	{course: "COM4519", department: "BIL", plan: exam2x, sections: single(40, staff("P10001"), th(thu, 13, 3, "MUH-A/A109"))},
}

// seedFall2026, Bilgisayar Mühendisliği'nin 2026 güz dönemi ders açmalarını, şubelerini,
// programını, öğretim elemanlarını ve değerlendirme planlarını oluşturur. Dönem ya da
// dersler yoksa (SQL seed'leri yüklenmemişse) atlar; açılmış dersleri tekrar açmaz.
func seedFall2026(ctx context.Context, pool *pgxpool.Pool) error {
	var termID string
	err := pool.QueryRow(ctx, `SELECT id FROM academic.terms WHERE code = '2026-FALL'`).Scan(&termID)
	if errors.Is(err, pgx.ErrNoRows) {
		fmt.Println("ATLANDI    2026 güz dönemi yok (önce SQL seed'lerini yükleyin)")
		return nil
	}
	if err != nil {
		return err
	}

	offerings := offering.NewRepository(pool)
	plans := grading.NewRepository(pool)
	ids := &lookups{pool: pool, cache: map[string]string{}}
	created := 0
	for _, spec := range fall2026 {
		courseID, err := ids.get(ctx, `SELECT id FROM curriculum.courses WHERE code = $1`, spec.course)
		if err != nil {
			return err
		}
		// Tamamlanan açılışlar ders seçmeye açık (OPEN) bırakılır. Planlama durumunda kalmış
		// bir açılış, önceki çalıştırmanın yarıda kaldığını gösterir: silinip yeniden kurulur.
		var existingID, status string
		err = pool.QueryRow(ctx, `SELECT id, status FROM offering.course_offerings WHERE term_id = $1 AND course_id = $2`,
			termID, courseID).Scan(&existingID, &status)
		switch {
		case err == nil && status != string(offering.StatusPlanned):
			continue
		case err == nil:
			if err := offerings.DeleteOffering(ctx, "", existingID); err != nil {
				return fmt.Errorf("%s yarım kalan açılışı silinemedi: %w", spec.course, err)
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		deptID, err := ids.get(ctx, `SELECT id FROM org.departments WHERE code = $1`, spec.department)
		if err != nil {
			return err
		}
		oID, err := offerings.CreateOffering(ctx, "", termID, offering.OfferingInput{CourseID: courseID, DepartmentID: deptID})
		if err != nil {
			return fmt.Errorf("%s açılamadı: %w", spec.course, err)
		}
		var language string
		if err := pool.QueryRow(ctx, `SELECT language FROM curriculum.courses WHERE id = $1`, courseID).Scan(&language); err != nil {
			return err
		}
		for _, sec := range spec.sections {
			if err := seedSection(ctx, offerings, plans, ids, oID, spec, sec, language); err != nil {
				return fmt.Errorf("%s-%s: %w", spec.course, sec.code, err)
			}
		}
		if err := offerings.UpdateOffering(ctx, "", oID, 1, offering.OfferingUpdate{Status: offering.StatusOpen}); err != nil {
			return fmt.Errorf("%s ders seçmeye açılamadı: %w", spec.course, err)
		}
		created++
	}
	if created > 0 {
		fmt.Printf("OLUŞTURULDU 2026 güz: %d ders açıldı (Bilgisayar Mühendisliği)\n", created)
	} else {
		fmt.Println("ATLANDI    2026 güz ders açmaları zaten var")
	}
	return nil
}

func seedSection(ctx context.Context, offerings *offering.Repository, plans *grading.Repository, ids *lookups,
	offeringID string, spec offeringSpec, sec sectionSpec, language string) error {
	mode := "OPEN"
	if len(sec.quotas) > 0 {
		mode = "RESERVED"
	}
	sectionID, err := offerings.CreateSection(ctx, "", offeringID, offering.SectionInput{
		Code: sec.code, Capacity: sec.capacity, QuotaMode: mode, InstructionMode: "IN_PERSON", Language: language,
	})
	if err != nil {
		return err
	}
	for _, sl := range sec.slots {
		in := offering.SlotInput{DayOfWeek: sl.day, Start: sl.start, End: sl.end, SessionType: sl.kind}
		if sl.room != "" {
			building, room, ok := strings.Cut(sl.room, "/")
			if !ok {
				return fmt.Errorf("derslik %q BİNA/DERSLİK biçiminde değil", sl.room)
			}
			if in.ClassroomID, err = ids.get(ctx, `
				SELECT cl.id FROM org.classrooms cl JOIN org.buildings b ON b.id = cl.building_id
				WHERE b.code = $1 AND cl.code = $2`, building, room); err != nil {
				return err
			}
		}
		if _, err := offerings.AddSlot(ctx, "", sectionID, in); err != nil {
			return fmt.Errorf("oturum %d %s: %w", sl.day, sl.start, err)
		}
	}
	if len(sec.quotas) > 0 {
		quotas := make([]offering.QuotaInput, 0, len(sec.quotas))
		for code, n := range sec.quotas {
			programID, err := ids.get(ctx, `SELECT id FROM org.programs WHERE code = $1`, code)
			if err != nil {
				return err
			}
			quotas = append(quotas, offering.QuotaInput{ProgramID: programID, Quota: n})
		}
		if err := offerings.SetQuotas(ctx, "", sectionID, quotas); err != nil {
			return err
		}
	}
	if len(sec.instructors) > 0 {
		items := make([]offering.InstructorInput, 0, len(sec.instructors))
		for i, no := range sec.instructors {
			staffID, err := ids.get(ctx, `SELECT id FROM people.staff WHERE staff_no = $1`, no)
			if err != nil {
				return err
			}
			role := "PRIMARY"
			if i > 0 {
				role = "ASSISTANT"
			}
			items = append(items, offering.InstructorInput{StaffID: staffID, Role: role})
		}
		if err := offerings.SetInstructors(ctx, "", sectionID, items); err != nil {
			return err
		}
	}

	parts := make([]grading.ComponentInput, 0, len(spec.plan))
	seq := map[string]int{}
	for _, p := range spec.plan {
		seq[p.typ]++
		parts = append(parts, grading.ComponentInput{TypeCode: p.typ, SequenceNo: seq[p.typ], Weight: p.weight})
	}
	if err := plans.SetPlan(ctx, "", sectionID, 1, parts); err != nil {
		return fmt.Errorf("değerlendirme planı: %w", err)
	}
	if spec.lockPlan && len(sec.instructors) > 0 {
		userID, err := ids.get(ctx, `SELECT id FROM iam.users WHERE username = $1`, sec.instructors[0])
		if err != nil {
			return err
		}
		if err := plans.Lock(ctx, userID, sectionID); err != nil {
			return fmt.Errorf("plan kilidi: %w", err)
		}
	}
	return nil
}

// lookups, kodla kimlik aramalarını önbelleğe alır.
type lookups struct {
	pool  *pgxpool.Pool
	cache map[string]string
}

func (l *lookups) get(ctx context.Context, query string, args ...any) (string, error) {
	key := query + fmt.Sprint(args...)
	if id, ok := l.cache[key]; ok {
		return id, nil
	}
	var id string
	err := l.pool.QueryRow(ctx, query, args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("kayıt bulunamadı: %v (SQL seed'leri ve demo kullanıcıları yüklü mü?)", args)
	}
	if err != nil {
		return "", err
	}
	l.cache[key] = id
	return id, nil
}
