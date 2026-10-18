-- +goose Up

-- Forum: her ders alanında bir Duyurular forumu (konuyu ve yanıtı öğretim elemanı yazar) ve
-- öğretim elemanının eklediği genel forumlar (öğrencilerin konu açması ayarlanabilir). Konular
-- sabitlenir ve kilitlenir; gönderiler temizlenmiş HTML'dir, silinen gönderi yerinde "silindi"
-- olarak kalır (yanıt zinciri bozulmasın). Abonelik bildirimleri Faz 6'da.
CREATE TABLE lms.forums (
    activity_id                uuid    PRIMARY KEY REFERENCES lms.activities (id),
    forum_kind                 text    NOT NULL CHECK (forum_kind IN ('ANNOUNCEMENTS', 'GENERAL')),
    students_can_start_threads boolean NOT NULL DEFAULT true,
    CONSTRAINT forums_announcements_check CHECK (forum_kind <> 'ANNOUNCEMENTS' OR NOT students_can_start_threads)
);

CREATE TABLE lms.forum_threads (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    activity_id  uuid        NOT NULL REFERENCES lms.forums (activity_id),
    author_id    uuid        NOT NULL REFERENCES iam.users (id),
    title        text        NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    is_pinned    boolean     NOT NULL DEFAULT false,
    is_locked    boolean     NOT NULL DEFAULT false,
    post_count   integer     NOT NULL DEFAULT 0,
    last_post_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);
-- Konu listesi: sabitlenenler önce, sonra son gönderiye göre.
CREATE INDEX forum_threads_list_idx ON lms.forum_threads (activity_id, is_pinned DESC, last_post_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE lms.forum_posts (
    id             uuid        PRIMARY KEY DEFAULT uuidv7(),
    thread_id      uuid        NOT NULL REFERENCES lms.forum_threads (id),
    parent_post_id uuid        REFERENCES lms.forum_posts (id),
    author_id      uuid        NOT NULL REFERENCES iam.users (id),
    body_html      text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    edited_at      timestamptz,
    deleted_at     timestamptz
);
CREATE INDEX forum_posts_thread_idx ON lms.forum_posts (thread_id, created_at);

-- Var olan ders alanlarına Duyurular forumu: ilk bölümün başına (yeni alanlarda ders alanı
-- oluşurken eklenir).
WITH first_topic AS (
    SELECT DISTINCT ON (t.section_id) t.section_id, t.id
    FROM lms.topics t WHERE t.deleted_at IS NULL
    ORDER BY t.section_id, t.position, t.created_at
),
created AS (
    INSERT INTO lms.activities (section_id, topic_id, activity_type, title, position)
    SELECT ft.section_id, ft.id, 'FORUM', 'Duyurular', -1 FROM first_topic ft
    WHERE NOT EXISTS (SELECT 1 FROM lms.activities a JOIN lms.forums f ON f.activity_id = a.id
                      WHERE a.section_id = ft.section_id AND f.forum_kind = 'ANNOUNCEMENTS')
    RETURNING id
)
INSERT INTO lms.forums (activity_id, forum_kind, students_can_start_threads)
SELECT id, 'ANNOUNCEMENTS', false FROM created;

-- +goose Down
DROP TABLE lms.forum_posts;
DROP TABLE lms.forum_threads;
DROP TABLE lms.forums;
DELETE FROM lms.activities WHERE activity_type = 'FORUM';
