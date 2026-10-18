package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/lms"
	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
)

// Ders alanı demosu: COM1007-1 (P10001, asistan P10007) için tanıtım metni, haftalık bölümler,
// sayfa, bağlantı, dosya ve klasör, etiket, iki ödev, Duyurular'da bir duyuru ve Soru-cevap
// forumunda bir konu. "Ödev 0"ın süresi geçmiştir: teslimler (ikisi geç) puanlanıp
// yayımlanmıştır, teslim etmeyenler zaman çizelgesinde "gecikti" görür. "Ödev 1"in süresi
// sürmektedir: birkaç teslim ve taslak vardır. Ödevler değerlendirme bileşenine bağlı değildir:
// Laboratuvar bileşeni not girişi demosunda elle girilir (bağlama demo rehberinde bir adımdır).
// Dosyalar gerçek dosya servisiyle yüklenir; depoya ulaşılamazsa dosyasız kurulur. Tekrar
// çalıştırılırsa ders alanı varsa atlanır.

// seedSpaceDemo, demo sınıfının ders alanını kurar.
func seedSpaceDemo(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) error {
	var sectionID string
	err := pool.QueryRow(ctx, `
		SELECT s.id FROM offering.sections s
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN academic.terms t ON t.id = o.term_id AND t.is_current
		JOIN curriculum.courses c ON c.id = o.course_id
		WHERE c.code = $1 AND s.section_code = $2`, demoClassCourse, demoClassSection).Scan(&sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		fmt.Println("ATLANDI    ders alanı demosu: demo sınıfı yok")
		return nil
	}
	if err != nil {
		return fmt.Errorf("demo sınıfı okunamadı: %w", err)
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM lms.course_spaces WHERE section_id = $1)`, sectionID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		fmt.Println("ATLANDI    ders alanı demosu: ders alanı zaten var")
		return nil
	}
	ids := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT username, id FROM iam.users WHERE username = ANY($1)`,
		append([]string{demoInstructor, "P10007"}, cohortNumbers()...))
	if err != nil {
		return err
	}
	var name, id string
	if _, err := pgx.ForEachRow(rows, []any{&name, &id}, func() error { ids[name] = id; return nil }); err != nil {
		return err
	}
	teacher, assistant := ids[demoInstructor], ids["P10007"]
	if teacher == "" || assistant == "" {
		fmt.Println("ATLANDI    ders alanı demosu: öğretim elemanları yok")
		return nil
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fs, stored := demoFiles(ctx, pool, cfg)
	now := time.Now()
	at := func(d time.Duration) func() time.Time { return func() time.Time { return now.Add(d) } }
	svc := lms.NewService(pool, fs, nil, logger, time.Now)
	ptr := func(t time.Time) *time.Time { return &t }

	if err := svc.SetSummary(ctx, teacher, sectionID, `<h2>Derse hoş geldiniz</h2>
<p>Bu derste Go diliyle programlamanın temellerini öğreniyoruz: değişkenler, koşullar, döngüler,
fonksiyonlar ve basit veri yapıları. Her hafta bir bölüm açılır; ödevler ve laboratuvar
çalışmaları buradan teslim edilir.</p>
<ul><li>Ders: Pazartesi 09:00 – 11:50, A104</li><li>Laboratuvar: Çarşamba 13:00 – 14:50, B-Lab2</li>
<li>Sorularınızı <strong>Soru-cevap</strong> forumuna yazın; öğretim elemanları ve asistan yanıtlar.</li></ul>`); err != nil {
		return err
	}
	week1, err := svc.CreateTopic(ctx, teacher, sectionID, lms.TopicInput{Title: "1. Hafta: Giriş ve ortam kurulumu", Visible: true,
		Summary: "<p>Go'yu kuruyor, ilk programımızı yazıyoruz.</p>"})
	if err != nil {
		return err
	}
	week2, err := svc.CreateTopic(ctx, teacher, sectionID, lms.TopicInput{Title: "2. Hafta: Değişkenler ve ifadeler", Visible: true,
		Summary: "<p>Temel türler, sabitler, aritmetik ve mantıksal ifadeler.</p>"})
	if err != nil {
		return err
	}
	if _, err := svc.CreateTopic(ctx, teacher, sectionID, lms.TopicInput{Title: "3. Hafta: Koşullar ve döngüler", Visible: true,
		Window: lms.Window{From: ptr(now.Add(7 * 24 * time.Hour))}}); err != nil {
		return err
	}
	space, err := svc.Space(ctx, lms.Section{ID: sectionID}, lms.RolePrimary, teacher)
	if err != nil {
		return err
	}
	general, news := space.Topics[0].ID, space.Topics[0].Activities[0].ID

	add := func(topic string, in lms.ActivityInput) (string, error) {
		in.Visible = true
		return svc.CreateActivity(ctx, teacher, topic, in)
	}
	upload := func(name, content string) []string {
		if !stored {
			return nil
		}
		f, err := fs.Upload(ctx, teacher, name, strings.NewReader(content))
		if err != nil {
			fmt.Printf("UYARI      %s yüklenemedi: %v\n", name, err)
			return nil
		}
		return []string{f.ID}
	}

	page, err := add(week1, lms.ActivityInput{Type: lms.TypePage, Title: "Go'yu kurmak", Body: `<h2>Kurulum</h2>
<ol><li><a href="https://go.dev/dl/">go.dev/dl</a> adresinden işletim sisteminize uygun paketi indirin.</li>
<li>Terminalde <code>go version</code> yazarak kurulumu doğrulayın.</li>
<li>Bir klasörde <code>go mod init merhaba</code> ile modül oluşturun.</li></ol>
<h3>İlk program</h3><pre><code>package main

import "fmt"

func main() {
	fmt.Println("Merhaba, Agora!")
}</code></pre>`})
	if err != nil {
		return err
	}
	tour, err := add(week1, lms.ActivityInput{Type: lms.TypeURL, Title: "A Tour of Go", URL: "https://go.dev/tour", NewTab: true,
		Description: "<p>Dilin temellerini tarayıcıda alıştırmalarla gezin.</p>"})
	if err != nil {
		return err
	}
	if f := upload("hafta1-slaytlar.pdf", demoPDF("Hafta 1: Giris ve ortam kurulumu")); f != nil {
		if _, err := add(week1, lms.ActivityInput{Type: lms.TypeResource, Title: "Hafta 1 slaytları", FileIDs: f}); err != nil {
			return err
		}
	}
	folder := append(upload("merhaba.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Merhaba!\")\n}\n"),
		upload("toplama.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\ta, b := 3, 4\n\tfmt.Println(a + b)\n}\n")...)
	if len(folder) == 2 {
		if _, err := add(week2, lms.ActivityInput{Type: lms.TypeFolder, Title: "Örnek kodlar", FileIDs: folder}); err != nil {
			return err
		}
	}
	if _, err := add(week2, lms.ActivityInput{Type: lms.TypeLabel, Title: "Ödev 1 ipuçları",
		Description: "<p><strong>İpucu:</strong> Bölme işleminde sıfıra bölmeyi denetlemeyi unutmayın.</p>"}); err != nil {
		return err
	}
	homework1, err := add(week2, lms.ActivityInput{Type: lms.TypeAssignment, Title: "Ödev 1: Basit hesap makinesi",
		Description: "<p>İki sayı ve bir işlem (+, -, *, /) okuyup sonucu yazan bir program yazın. Kaynak kodu (.go) ya da raporu (.pdf) yükleyin.</p>",
		Assignment: &lms.AssignmentSettings{
			OpensAt: ptr(now.Add(-2 * 24 * time.Hour)), DueAt: ptr(endOfDay(now.Add(7 * 24 * time.Hour))),
			MaxAttempts: 2, SubmissionTypes: []string{lms.SubmitFile, lms.SubmitText}, MaxFiles: 3, MaxFileSize: cfg.MaxUploadBytes,
			Extensions: []string{".go", ".pdf", ".zip"}, MaxScore: 5000, LatePolicy: lms.LatePenalty, PenaltyPerDay: 1000,
		}})
	if err != nil {
		return err
	}
	homework0, err := add(general, lms.ActivityInput{Type: lms.TypeAssignment, Title: "Ödev 0: Kendini tanıt",
		Description: "<p>Kendinizi birkaç cümleyle tanıtın: neden bu bölümü seçtiniz, programlamayla ilgili beklentileriniz neler?</p>",
		Assignment: &lms.AssignmentSettings{
			OpensAt: ptr(now.Add(-10 * 24 * time.Hour)), DueAt: ptr(now.Add(-3 * 24 * time.Hour)),
			MaxAttempts: 1, SubmissionTypes: []string{lms.SubmitText}, MaxFiles: 1, MaxFileSize: cfg.MaxUploadBytes,
			Extensions: []string{}, MaxScore: 1000, LatePolicy: lms.LateAllow,
		}})
	if err != nil {
		return err
	}
	qa, err := add(general, lms.ActivityInput{Type: lms.TypeForum, Title: "Soru-cevap", Forum: &lms.ForumSettings{StudentsCanStart: true},
		Description: "<p>Dersle ilgili sorularınızı buraya yazın.</p>"})
	if err != nil {
		return err
	}

	// Duyuru ve forum.
	if _, err := svc.CreateThread(ctx, teacher, news, "Derse hoş geldiniz",
		"<p>Merhaba arkadaşlar, ilk hafta Go kurulumunu yapıp <em>A Tour of Go</em>'nun ilk bölümünü bitirmenizi bekliyorum. Ödev 1'in son tarihi bölümde yazıyor.</p>"); err != nil {
		return err
	}
	thread, err := svc.CreateThread(ctx, ids["26290103"], qa, "Kurulumdan sonra go komutu bulunamıyor",
		"<p>Windows'ta kurulumu yaptım ama terminalde <code>go</code> yazınca komut bulunamadı diyor. Ne yapmalıyım?</p>")
	if err != nil {
		return err
	}
	if _, err := svc.Reply(ctx, assistant, thread, "", "<p>Kurulumdan sonra terminali kapatıp yeniden açın; PATH değişkeni yeni oturumda güncellenir. Olmazsa laboratuvarda birlikte bakalım.</p>"); err != nil {
		return err
	}
	if _, err := svc.Reply(ctx, ids["26290103"], thread, "", "<p>Terminali yeniden açınca düzeldi, teşekkürler!</p>"); err != nil {
		return err
	}

	// Ödev 0: süresi geçti. 12 öğrenci zamanında, 2 öğrenci geç teslim etti; puanlar yayımlandı.
	cohortNos := cohortNumbers()
	submit := func(s *lms.Service, activity, user, text string) error {
		if err := s.SaveDraft(ctx, user, activity, 0, lms.DraftInput{Text: text}); err != nil {
			return err
		}
		return s.Submit(ctx, user, activity, 1)
	}
	onTime, late := lms.NewService(pool, fs, nil, logger, at(-5*24*time.Hour)), lms.NewService(pool, fs, nil, logger, at(-2*24*time.Hour))
	for i, no := range cohortNos[:14] {
		s := onTime
		if i >= 12 {
			s = late
		}
		if err := submit(s, homework0, ids[no], fmt.Sprintf("<p>Merhaba, ben %s. Bilgisayar mühendisliğini oyun geliştirmeye olan ilgim yüzünden seçtim.</p>", no)); err != nil {
			return fmt.Errorf("ödev 0 teslimi %s: %w", no, err)
		}
		score := grading.Points(700 + (i%4)*100)
		if err := svc.SetGrade(ctx, teacher, sectionID, homework0, ids[no], 0, lms.GradeInput{Score: &score,
			Feedback: "<p>Teşekkürler, tanıştığımıza memnun oldum.</p>"}); err != nil {
			return fmt.Errorf("ödev 0 puanı %s: %w", no, err)
		}
	}
	if err := svc.ReleaseGrades(ctx, teacher, homework0); err != nil {
		return err
	}
	// Ödev 1: süresi sürüyor. 5 öğrenci gönderdi, 2 öğrenci taslakta.
	for i, no := range cohortNos[:7] {
		text := "<p>Hesap makinesinin kaynak kodu ve kısa açıklaması ektedir.</p>"
		if i >= 5 {
			if err := svc.SaveDraft(ctx, ids[no], homework1, 0, lms.DraftInput{Text: "<p>Taslak: bölme kısmı eksik.</p>"}); err != nil {
				return err
			}
			continue
		}
		if err := submit(svc, homework1, ids[no], text); err != nil {
			return fmt.Errorf("ödev 1 teslimi %s: %w", no, err)
		}
	}
	// İlk hafta içeriğini gezen öğrenciler.
	for _, no := range cohortNos[:10] {
		for _, a := range []struct {
			id  string
			typ lms.ActivityType
		}{{page, lms.TypePage}, {tour, lms.TypeURL}} {
			if err := svc.MarkComplete(ctx, ids[no], a.id, a.typ); err != nil {
				return err
			}
		}
	}
	files := "dosyalarla"
	if !stored {
		files = "dosyasız (depoya ulaşılamadı)"
	}
	fmt.Printf("DERS ALANI %s-%s: 4 bölüm, 2 ödev (14 + 5 teslim), duyuru ve forum, %s\n", demoClassCourse, demoClassSection, files)
	return nil
}

// demoFiles, seed'in dosya servisidir. Ders alanı servisi dosya sınırını ondan okuduğu için depo
// olmasa da oluşturulur; depoya ulaşılamazsa stored false olur ve dosyalı etkinlikler atlanır.
func demoFiles(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) (svc *files.Service, stored bool) {
	store, err := files.NewS3Store(files.S3Config{
		Endpoint: cfg.S3Endpoint, PublicEndpoint: cfg.S3PublicEndpoint, UseSSL: cfg.S3UseSSL, Region: cfg.S3Region,
		Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
	})
	if err != nil {
		return files.NewService(pool, nil, cfg.MaxUploadBytes, time.Now), false
	}
	bctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stored = store.EnsureBucket(bctx) == nil
	return files.NewService(pool, store, cfg.MaxUploadBytes, time.Now), stored
}

// demoPDF, tek sayfalık, başlığı yazan geçerli bir PDF üretir.
func demoPDF(title string) string {
	stream := fmt.Sprintf("BT /F1 24 Tf 72 720 Td (%s) Tj ET", title)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.String()
}

// endOfDay, günün 23:59'udur (Türkiye saatiyle).
func endOfDay(t time.Time) time.Time {
	tr := time.FixedZone("TRT", 3*60*60)
	t = t.In(tr)
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 0, 0, tr)
}

// cohortNumbers, demo grubunun öğrenci numaralarıdır.
func cohortNumbers() []string {
	out := make([]string, len(cohort))
	for i, c := range cohort {
		out[i] = c.no
	}
	return out
}
