// Package messaging, kullanıcılar arası iletileşmedir: birebir konuşma, personelin kurduğu grup
// ve şube grubu (üyeleri şubenin öğretim elemanları ve kesin kaydı olan öğrencileri). İleti metni
// düz yazıdır; yeni ileti alıcıların açık akışlarına duyurulur (internal/notify), bildirim merkezine
// sadece tercihle girer.
//
// Kim kime yazabilir: personel herkese; öğrenci kendi şubelerinin öğretim elemanlarına ve
// danışmanına (öğrenciler birbirine şube grubunda yazar).
package messaging

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Konuşma türleri.
const (
	KindDirect  = "DIRECT"
	KindGroup   = "GROUP"
	KindSection = "SECTION"
)

// Hatalar.
var (
	ErrNotFound = errors.New("messaging: konuşma bulunamadı")
	// ErrNotAllowed: politika bu kişiye yazmaya ya da grup kurmaya izin vermiyor.
	ErrNotAllowed = errors.New("messaging: bu kişiye ileti gönderilemez")
)

// ValidationError, girdideki bir alanın hatasıdır.
type ValidationError struct {
	Field, Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Person, konuşmadaki ya da yazılabilecek bir kişidir.
type Person struct {
	ID     string
	Name   string
	Number string // öğrenci ya da personel numarası (kullanıcı adı)
	Staff  bool
}

// Message, bir iletidir. Silinmiş iletinin metni boştur.
type Message struct {
	ID             string
	ConversationID string
	Sender         Person
	Body           string
	ReplyToID      *string
	CreatedAt      time.Time
	EditedAt       *time.Time
	DeletedAt      *time.Time
}

// Conversation, kullanıcının gördüğü hâliyle bir konuşmadır.
type Conversation struct {
	ID            string
	Kind          string
	Title         string  // grupta adı, şube grubunda ders ve şube; birebirde boş (karşı taraf Peer)
	SectionID     string  // şube grubunda
	Peer          *Person // birebirde karşı taraf
	MemberCount   int
	LastMessage   *Message
	LastMessageAt *time.Time
	CreatedAt     time.Time
	Unread        int
	Starred       bool
	MutedUntil    *time.Time
	Moderator     bool // grubu kuran ya da şubenin öğretim elemanı
}

// Service, mesajlaşmayı yönetir.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewService, bir Service oluşturur.
func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, now: now}
}

// sectionRole, $2 kullanıcısının $1 şubesindeki rolüdür: 'INSTRUCTOR', 'STUDENT' ya da boş.
const sectionRole = `coalesce(
	(SELECT 'INSTRUCTOR' FROM offering.section_instructors si
	 JOIN people.staff st ON st.id = si.staff_id JOIN iam.users u ON u.person_id = st.person_id
	 WHERE si.section_id = $1 AND u.id = $2 LIMIT 1),
	(SELECT 'STUDENT' FROM enrollment.enrollments e
	 JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
	 JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
	 WHERE e.section_id = $1 AND e.status <> 'DROPPED' AND u.id = $2 LIMIT 1),
	'')`

// sectionMembers, $1 şubesinin üyeleridir (öğretim elemanları ve bırakmamış öğrenciler).
const sectionMembers = `
	SELECT u.id FROM offering.section_instructors si
	JOIN people.staff st ON st.id = si.staff_id JOIN iam.users u ON u.person_id = st.person_id
	WHERE si.section_id = $1
	UNION
	SELECT u.id FROM enrollment.enrollments e
	JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
	JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
	WHERE e.section_id = $1 AND e.status <> 'DROPPED'`

// access, kullanıcının konuşmaya erişimidir: birebir ve grupta ayrılmamış üyelik, şube grubunda
// şube üyeliği. Erişemeyen için ErrNotFound (konuşma yokmuş gibi).
func access(ctx context.Context, q db.Querier, conversationID, userID string) (kind string, moderator bool, err error) {
	var (
		sectionID *string
		role      *string
	)
	err = q.QueryRow(ctx, `
		SELECT c.conversation_type, c.section_id,
		       (SELECT m.member_role FROM communication.conversation_members m
		        WHERE m.conversation_id = c.id AND m.user_id = $2 AND m.left_at IS NULL)
		FROM communication.conversations c WHERE c.id = $1`, conversationID, userID).Scan(&kind, &sectionID, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrNotFound
	}
	if err != nil {
		return "", false, fmt.Errorf("messaging: konuşma okunamadı: %w", err)
	}
	if kind != KindSection {
		if role == nil {
			return "", false, ErrNotFound
		}
		return kind, *role == "OWNER", nil
	}
	var sr string
	if err := q.QueryRow(ctx, `SELECT `+sectionRole, *sectionID, userID).Scan(&sr); err != nil {
		return "", false, fmt.Errorf("messaging: şube üyeliği okunamadı: %w", err)
	}
	if sr == "" {
		return "", false, ErrNotFound
	}
	return kind, sr == "INSTRUCTOR", nil
}

// isStaff, kullanıcının etkin personel olduğunu söyler.
func isStaff(ctx context.Context, q db.Querier, userID string) (bool, error) {
	var ok bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM people.staff s JOIN iam.users u ON u.person_id = s.person_id
		               WHERE u.id = $1 AND s.employment_status = 'ACTIVE')`, userID).Scan(&ok); err != nil {
		return false, fmt.Errorf("messaging: personel bilgisi okunamadı: %w", err)
	}
	return ok, nil
}

// teachersSQL, $1 öğrencisinin şubelerinin öğretim elemanları ve danışmanıdır (etkin hesaplar).
const teachersSQL = `
	SELECT DISTINCT tu.id FROM enrollment.enrollments e
	JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
	JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
	JOIN offering.section_instructors si ON si.section_id = e.section_id
	JOIN people.staff s ON s.id = si.staff_id JOIN iam.users tu ON tu.person_id = s.person_id
	WHERE u.id = $1 AND e.status <> 'DROPPED' AND tu.status = 'ACTIVE'
	UNION
	SELECT au.id FROM enrollment.advisor_assignments aa
	JOIN enrollment.student_programs sp ON sp.id = aa.student_program_id
	JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
	JOIN people.staff s ON s.id = aa.advisor_staff_id JOIN iam.users au ON au.person_id = s.person_id
	WHERE u.id = $1 AND aa.valid_until IS NULL AND au.status = 'ACTIVE'`

// CanMessage, from'un to'ya birebir yazabildiğini söyler (bkz. paket açıklaması).
func (s *Service) CanMessage(ctx context.Context, from, to string) (bool, error) {
	if from == to {
		return false, nil
	}
	var active bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM iam.users WHERE id = $1 AND status = 'ACTIVE')`, to).Scan(&active); err != nil {
		return false, fmt.Errorf("messaging: kullanıcı okunamadı: %w", err)
	}
	if !active {
		return false, nil
	}
	staff, err := isStaff(ctx, s.pool, from)
	if err != nil || staff {
		return staff, err
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT $2::uuid IN (`+teachersSQL+`)`, from, to).Scan(&ok); err != nil {
		return false, fmt.Errorf("messaging: yazma izni okunamadı: %w", err)
	}
	return ok, nil
}

// Contacts, kullanıcının birebir yazabileceği kişilerdir: personele ad ya da numarayla arama,
// öğrenciye öğretim elemanları ve danışmanı (q ile süzülür).
func (s *Service) Contacts(ctx context.Context, userID, q string, limit int) ([]Person, error) {
	staff, err := isStaff(ctx, s.pool, userID)
	if err != nil {
		return nil, err
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	source := `SELECT id FROM iam.users WHERE status = 'ACTIVE' AND id <> $1`
	if !staff {
		source = teachersSQL
	} else if utf8.RuneCountInString(strings.TrimSpace(q)) < 2 {
		return []Person{}, nil // personel en az iki harfle arar: bütün üniversite listelenmez
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, p.first_name || ' ' || p.last_name, u.username,
		       EXISTS (SELECT 1 FROM people.staff st WHERE st.person_id = p.id AND st.employment_status = 'ACTIVE')
		FROM iam.users u JOIN people.persons p ON p.id = u.person_id
		WHERE u.id IN (`+source+`)
		  AND (lower(p.first_name || ' ' || p.last_name) LIKE $2 OR lower(u.username) LIKE $2)
		ORDER BY p.first_name COLLATE "tr-x-icu", p.last_name COLLATE "tr-x-icu"
		LIMIT $3`, userID, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("messaging: kişiler okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, scanPerson)
}

func scanPerson(row pgx.CollectableRow) (Person, error) {
	var p Person
	err := row.Scan(&p.ID, &p.Name, &p.Number, &p.Staff)
	return p, err
}

// Direct, iki kullanıcı arasındaki birebir konuşmayı döndürür; yoksa (yazma izni varsa) oluşturur.
func (s *Service) Direct(ctx context.Context, actorID, peerID string) (string, error) {
	ok, err := s.CanMessage(ctx, actorID, peerID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotAllowed
	}
	pair := []string{actorID, peerID}
	slices.Sort(pair)
	key := pair[0] + ":" + pair[1]
	var id string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO communication.conversations (conversation_type, direct_key, created_by) VALUES ('DIRECT', $1, $2)
			ON CONFLICT (direct_key) DO UPDATE SET direct_key = excluded.direct_key
			RETURNING id`, key, actorID).Scan(&id)
		if err != nil {
			return fmt.Errorf("messaging: konuşma açılamadı: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO communication.conversation_members (conversation_id, user_id) VALUES ($1, $2), ($1, $3)
			ON CONFLICT (conversation_id, user_id) DO NOTHING`, id, actorID, peerID)
		if err != nil {
			return fmt.Errorf("messaging: üyeler yazılamadı: %w", err)
		}
		return nil
	})
	return id, err
}

// CreateGroup, personelin kurduğu bir grup konuşmasıdır; kuran yöneticidir.
func (s *Service) CreateGroup(ctx context.Context, actorID, title string, memberIDs []string) (string, error) {
	title = strings.TrimSpace(title)
	if n := utf8.RuneCountInString(title); n < 1 || n > 120 {
		return "", &ValidationError{"title", "1 ile 120 karakter arasında olmalı."}
	}
	members := slices.DeleteFunc(slices.Compact(slices.Sorted(slices.Values(memberIDs))), func(id string) bool { return id == actorID })
	if len(members) == 0 || len(members) > 200 {
		return "", &ValidationError{"member_ids", "1 ile 200 arasında üye seçin."}
	}
	staff, err := isStaff(ctx, s.pool, actorID)
	if err != nil {
		return "", err
	}
	if !staff {
		return "", ErrNotAllowed
	}
	var id string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var found int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM iam.users WHERE id = ANY ($1::uuid[]) AND status = 'ACTIVE'`, members).Scan(&found); err != nil {
			return fmt.Errorf("messaging: üyeler okunamadı: %w", err)
		}
		if found != len(members) {
			return &ValidationError{"member_ids", "Üyelerden biri bulunamadı."}
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO communication.conversations (conversation_type, title, created_by) VALUES ('GROUP', $1, $2) RETURNING id`,
			title, actorID).Scan(&id); err != nil {
			return fmt.Errorf("messaging: grup kurulamadı: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO communication.conversation_members (conversation_id, user_id, member_role)
			SELECT $1::uuid, $2::uuid, 'OWNER' UNION ALL SELECT $1::uuid, u, 'MEMBER' FROM unnest($3::uuid[]) AS u`, id, actorID, members); err != nil {
			return fmt.Errorf("messaging: üyeler yazılamadı: %w", err)
		}
		return nil
	})
	return id, err
}

// ensureSections, kullanıcının içinde bulunulan dönemdeki şubelerinin grup konuşmalarını açar.
func (s *Service) ensureSections(ctx context.Context, userID string) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO communication.conversations (conversation_type, section_id)
		SELECT 'SECTION', x.section_id FROM (
			SELECT si.section_id FROM offering.section_instructors si
			JOIN people.staff st ON st.id = si.staff_id JOIN iam.users u ON u.person_id = st.person_id
			WHERE u.id = $1
			UNION
			SELECT e.section_id FROM enrollment.enrollments e
			JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
			JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
			WHERE u.id = $1 AND e.status <> 'DROPPED'
		) x
		JOIN offering.sections s ON s.id = x.section_id AND s.status <> 'CANCELLED'
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN academic.terms t ON t.id = o.term_id AND t.is_current
		ON CONFLICT (section_id) DO NOTHING`, userID); err != nil {
		return fmt.Errorf("messaging: şube grupları açılamadı: %w", err)
	}
	return nil
}

// visibleSQL, $1 kullanıcısının gördüğü konuşmalardır: üyesi olduğu birebir ve gruplar, içinde
// bulunulan dönemde üyesi olduğu şubelerin grupları.
const visibleSQL = `
	SELECT m.conversation_id AS id FROM communication.conversation_members m
	JOIN communication.conversations c ON c.id = m.conversation_id AND c.conversation_type <> 'SECTION'
	WHERE m.user_id = $1 AND m.left_at IS NULL
	UNION
	SELECT c.id FROM communication.conversations c
	JOIN offering.sections s ON s.id = c.section_id
	JOIN offering.course_offerings o ON o.id = s.offering_id
	JOIN academic.terms t ON t.id = o.term_id AND t.is_current
	WHERE c.section_id IN (
		SELECT si.section_id FROM offering.section_instructors si
		JOIN people.staff st ON st.id = si.staff_id JOIN iam.users u ON u.person_id = st.person_id WHERE u.id = $1
		UNION
		SELECT e.section_id FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
		WHERE u.id = $1 AND e.status <> 'DROPPED')`

// unreadExpr, c konuşmasında $1 kullanıcısının okumadığı (başkasının yazdığı, silinmemiş) iletilerdir.
const unreadExpr = `(SELECT count(*) FROM communication.messages x
	WHERE x.conversation_id = c.id AND x.sender_id <> $1 AND x.deleted_at IS NULL
	  AND (m.last_read_message_id IS NULL OR x.id > m.last_read_message_id))`

// Conversations, kullanıcının konuşmalarıdır: son iletiye göre yeniden eskiye (iletisi olmayanlar
// sonda), okunmamış sayısı, son ileti ve kişisel durumla.
func (s *Service) Conversations(ctx context.Context, userID string) ([]Conversation, error) {
	if err := s.ensureSections(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.conversation_type, coalesce(c.title, crs.code || '-' || sec.section_code || ' · ' || crs.name_tr, ''),
		       coalesce(c.section_id::text, ''), c.created_at, c.last_message_at,
		       coalesce(m.is_starred, false), m.muted_until,
		       CASE WHEN c.conversation_type = 'SECTION' THEN EXISTS (
		           SELECT 1 FROM offering.section_instructors si JOIN people.staff st ON st.id = si.staff_id
		           JOIN iam.users iu ON iu.person_id = st.person_id WHERE si.section_id = c.section_id AND iu.id = $1)
		       ELSE coalesce(m.member_role = 'OWNER', false) END, `+unreadExpr+`,
		       peer.id, peer.name, peer.username, peer.staff,
		       lm.id, lm.sender_id, lm.sender_name, lm.body, lm.created_at, lm.deleted_at
		FROM (`+visibleSQL+`) v
		JOIN communication.conversations c ON c.id = v.id
		LEFT JOIN communication.conversation_members m ON m.conversation_id = c.id AND m.user_id = $1
		LEFT JOIN offering.sections sec ON sec.id = c.section_id
		LEFT JOIN offering.course_offerings o ON o.id = sec.offering_id
		LEFT JOIN curriculum.courses crs ON crs.id = o.course_id
		LEFT JOIN LATERAL (
			SELECT u.id::text, p.first_name || ' ' || p.last_name AS name, u.username,
			       EXISTS (SELECT 1 FROM people.staff st WHERE st.person_id = p.id AND st.employment_status = 'ACTIVE') AS staff
			FROM communication.conversation_members pm JOIN iam.users u ON u.id = pm.user_id JOIN people.persons p ON p.id = u.person_id
			WHERE c.conversation_type = 'DIRECT' AND pm.conversation_id = c.id AND pm.user_id <> $1 LIMIT 1
		) peer ON true
		LEFT JOIN LATERAL (
			SELECT x.id::text, x.sender_id::text, sp.first_name || ' ' || sp.last_name AS sender_name,
			       CASE WHEN x.deleted_at IS NULL THEN x.body ELSE '' END AS body, x.created_at, x.deleted_at
			FROM communication.messages x JOIN iam.users su ON su.id = x.sender_id JOIN people.persons sp ON sp.id = su.person_id
			WHERE x.conversation_id = c.id ORDER BY x.id DESC LIMIT 1
		) lm ON true
		ORDER BY c.last_message_at DESC NULLS LAST, c.created_at DESC, c.id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("messaging: konuşmalar okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Conversation, error) {
		var (
			c                              Conversation
			peerID, peerName, peerNo       *string
			peerStaff                      *bool
			lmID, lmSender, lmName, lmBody *string
			lmAt                           *time.Time
			lmDeleted                      *time.Time
		)
		err := row.Scan(&c.ID, &c.Kind, &c.Title, &c.SectionID, &c.CreatedAt, &c.LastMessageAt, &c.Starred, &c.MutedUntil,
			&c.Moderator, &c.Unread, &peerID, &peerName, &peerNo, &peerStaff, &lmID, &lmSender, &lmName, &lmBody, &lmAt, &lmDeleted)
		if peerID != nil {
			c.Peer = &Person{ID: *peerID, Name: *peerName, Number: *peerNo, Staff: *peerStaff}
		}
		if lmID != nil {
			c.LastMessage = &Message{ID: *lmID, ConversationID: c.ID, Sender: Person{ID: *lmSender, Name: *lmName}, Body: *lmBody,
				CreatedAt: *lmAt, DeletedAt: lmDeleted}
		}
		return c, err
	})
}

// UnreadTotal, kullanıcının bütün konuşmalarındaki okunmamış ileti sayısıdır.
func (s *Service) UnreadTotal(ctx context.Context, userID string) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `
		SELECT coalesce(sum(`+unreadExpr+`), 0)::int FROM (`+visibleSQL+`) v
		JOIN communication.conversations c ON c.id = v.id
		LEFT JOIN communication.conversation_members m ON m.conversation_id = c.id AND m.user_id = $1
		WHERE m.muted_until IS NULL OR m.muted_until < now()`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("messaging: okunmamışlar sayılamadı: %w", err)
	}
	return n, nil
}

// Conversation, kullanıcının erişebildiği tek konuşmadır (listedeki alanlarla).
func (s *Service) Conversation(ctx context.Context, conversationID, userID string) (Conversation, []Person, error) {
	if _, _, err := access(ctx, s.pool, conversationID, userID); err != nil {
		return Conversation{}, nil, err
	}
	list, err := s.Conversations(ctx, userID)
	if err != nil {
		return Conversation{}, nil, err
	}
	i := slices.IndexFunc(list, func(c Conversation) bool { return c.ID == conversationID })
	if i < 0 {
		return Conversation{}, nil, ErrNotFound // ör. geçmiş dönemin şube grubu
	}
	c := list[i]
	members, err := s.members(ctx, c)
	if err != nil {
		return Conversation{}, nil, err
	}
	c.MemberCount = len(members)
	return c, members, nil
}

func (s *Service) members(ctx context.Context, c Conversation) ([]Person, error) {
	source := `SELECT user_id FROM communication.conversation_members WHERE conversation_id = $1 AND left_at IS NULL`
	arg := c.ID
	if c.Kind == KindSection {
		source, arg = sectionMembers, c.SectionID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, p.first_name || ' ' || p.last_name, u.username,
		       EXISTS (SELECT 1 FROM people.staff st WHERE st.person_id = p.id AND st.employment_status = 'ACTIVE')
		FROM iam.users u JOIN people.persons p ON p.id = u.person_id
		WHERE u.id IN (`+source+`)
		ORDER BY 4 DESC, p.first_name COLLATE "tr-x-icu", p.last_name COLLATE "tr-x-icu"`, arg)
	if err != nil {
		return nil, fmt.Errorf("messaging: üyeler okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, scanPerson)
}

const messageSelect = `
	SELECT x.id, x.conversation_id, x.sender_id, p.first_name || ' ' || p.last_name, u.username,
	       EXISTS (SELECT 1 FROM people.staff st WHERE st.person_id = p.id AND st.employment_status = 'ACTIVE'),
	       CASE WHEN x.deleted_at IS NULL THEN x.body ELSE '' END, x.reply_to_id, x.created_at, x.edited_at, x.deleted_at
	FROM communication.messages x JOIN iam.users u ON u.id = x.sender_id JOIN people.persons p ON p.id = u.person_id`

func scanMessage(row pgx.CollectableRow) (Message, error) {
	var m Message
	err := row.Scan(&m.ID, &m.ConversationID, &m.Sender.ID, &m.Sender.Name, &m.Sender.Number, &m.Sender.Staff,
		&m.Body, &m.ReplyToID, &m.CreatedAt, &m.EditedAt, &m.DeletedAt)
	return m, err
}

// Message, iletiyi kimliğiyle okur (erişim denetlenmiş olmalı).
func (s *Service) Message(ctx context.Context, id string) (Message, error) {
	rows, err := s.pool.Query(ctx, messageSelect+` WHERE x.id = $1`, id)
	if err != nil {
		return Message{}, fmt.Errorf("messaging: ileti okunamadı: %w", err)
	}
	m, err := pgx.CollectExactlyOneRow(rows, scanMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("messaging: ileti okunamadı: %w", err)
	}
	return m, nil
}

// Messages, konuşmanın iletileridir: before kimliğinden eskiler (boşsa en yeniler), yeniden eskiye.
func (s *Service) Messages(ctx context.Context, conversationID, userID, before string, limit int) ([]Message, bool, error) {
	if _, _, err := access(ctx, s.pool, conversationID, userID); err != nil {
		return nil, false, err
	}
	rows, err := s.pool.Query(ctx, messageSelect+`
		WHERE x.conversation_id = $1 AND ($2::uuid IS NULL OR x.id < $2::uuid)
		ORDER BY x.id DESC LIMIT $3`, conversationID, nullable(before), limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("messaging: iletiler okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanMessage)
	if err != nil {
		return nil, false, fmt.Errorf("messaging: iletiler okunamadı: %w", err)
	}
	if len(list) > limit {
		return list[:limit], true, nil
	}
	return list, false, nil
}

// recipients, konuşmanın gönderen dışındaki üyeleridir.
func recipients(ctx context.Context, tx pgx.Tx, conversationID, kind, sectionID, senderID string) ([]string, error) {
	source, arg := `SELECT user_id FROM communication.conversation_members WHERE conversation_id = $1 AND left_at IS NULL`, conversationID
	if kind == KindSection {
		source, arg = sectionMembers, sectionID
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM (`+source+`) r(id) WHERE id <> $2`, arg, senderID)
	if err != nil {
		return nil, fmt.Errorf("messaging: alıcılar okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// MaxBody, bir iletinin en uzun metnidir (karakter).
const MaxBody = 4000

// Send, konuşmaya bir ileti yazar; gönderenin okundu imleci iletiye ilerler. Alıcıların açık
// akışlarına duyurulur; tercihi açık olana (sessize almamışsa) bildirim yazılır.
func (s *Service) Send(ctx context.Context, actorID, conversationID, body, replyTo string) (string, error) {
	body = strings.TrimSpace(body)
	if n := utf8.RuneCountInString(body); n < 1 || n > MaxBody {
		return "", &ValidationError{"body", fmt.Sprintf("1 ile %d karakter arasında olmalı.", MaxBody)}
	}
	var id string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		kind, _, err := access(ctx, tx, conversationID, actorID)
		if err != nil {
			return err
		}
		var reply any
		if replyTo != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM communication.messages WHERE id = $1 AND conversation_id = $2)`,
				replyTo, conversationID).Scan(&ok); err != nil {
				return fmt.Errorf("messaging: yanıtlanan ileti okunamadı: %w", err)
			}
			if !ok {
				return &ValidationError{"reply_to_id", "Yanıtlanan ileti bu konuşmada değil."}
			}
			reply = replyTo
		}
		var (
			sectionID, title, sender string
		)
		if err := tx.QueryRow(ctx, `
			UPDATE communication.conversations SET last_message_at = now() WHERE id = $1
			RETURNING coalesce(section_id::text, ''), coalesce(title, ''),
			          (SELECT p.first_name || ' ' || p.last_name FROM iam.users u JOIN people.persons p ON p.id = u.person_id WHERE u.id = $2)`,
			conversationID, actorID).Scan(&sectionID, &title, &sender); err != nil {
			return fmt.Errorf("messaging: konuşma güncellenemedi: %w", err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO communication.messages (conversation_id, sender_id, body, reply_to_id) VALUES ($1, $2, $3, $4) RETURNING id`,
			conversationID, actorID, body, reply).Scan(&id); err != nil {
			return fmt.Errorf("messaging: ileti yazılamadı: %w", err)
		}
		if err := markRead(ctx, tx, conversationID, actorID, id); err != nil {
			return err
		}
		to, err := recipients(ctx, tx, conversationID, kind, sectionID, actorID)
		if err != nil {
			return err
		}
		if err := notify.AnnounceMessages(ctx, tx, to, conversationID); err != nil {
			return err
		}
		muted, err := mutedOf(ctx, tx, conversationID, to)
		if err != nil {
			return err
		}
		if kind == KindSection {
			title = ""
			if err := tx.QueryRow(ctx, `
				SELECT c.code || '-' || s.section_code FROM offering.sections s
				JOIN offering.course_offerings o ON o.id = s.offering_id JOIN curriculum.courses c ON c.id = o.course_id
				WHERE s.id = $1`, sectionID).Scan(&title); err != nil {
				return fmt.Errorf("messaging: şube okunamadı: %w", err)
			}
		}
		preview := []rune(body)
		if len(preview) > 140 {
			preview = append(preview[:139], '…')
		}
		return notify.Send(ctx, tx, notify.Notification{Type: notify.MessageReceived, Link: "/mesajlar/" + conversationID,
			UserIDs: slices.DeleteFunc(to, func(u string) bool { return muted[u] }),
			Data:    map[string]any{"sender": sender, "conversation": title, "preview": string(preview)}})
	})
	return id, err
}

// mutedOf, konuşmayı şu an sessize almış alıcılardır.
func mutedOf(ctx context.Context, tx pgx.Tx, conversationID string, users []string) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT user_id::text FROM communication.conversation_members
		WHERE conversation_id = $1 AND user_id = ANY ($2::uuid[]) AND muted_until > now()`, conversationID, users)
	if err != nil {
		return nil, fmt.Errorf("messaging: sessize alanlar okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("messaging: sessize alanlar okunamadı: %w", err)
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// markRead, okundu imlecini iletiye ilerletir (geri almaz); şube grubunda üyelik satırını açar.
func markRead(ctx context.Context, q db.Querier, conversationID, userID, messageID string) error {
	if _, err := q.Exec(ctx, `
		INSERT INTO communication.conversation_members (conversation_id, user_id, last_read_message_id) VALUES ($1, $2, $3)
		ON CONFLICT (conversation_id, user_id) DO UPDATE
		SET last_read_message_id = greatest(communication.conversation_members.last_read_message_id, excluded.last_read_message_id)`,
		conversationID, userID, messageID); err != nil {
		return fmt.Errorf("messaging: okundu yazılamadı: %w", err)
	}
	return nil
}

// MarkRead, kullanıcının konuşmayı messageID'ye kadar okuduğunu kaydeder.
func (s *Service) MarkRead(ctx context.Context, userID, conversationID, messageID string) error {
	if _, _, err := access(ctx, s.pool, conversationID, userID); err != nil {
		return err
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM communication.messages WHERE id = $1 AND conversation_id = $2)`,
		messageID, conversationID).Scan(&ok); err != nil {
		return fmt.Errorf("messaging: ileti okunamadı: %w", err)
	}
	if !ok {
		return &ValidationError{"message_id", "İleti bu konuşmada değil."}
	}
	return markRead(ctx, s.pool, conversationID, userID, messageID)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
