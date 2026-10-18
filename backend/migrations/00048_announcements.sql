-- +goose Up

-- Kurumsal duyurular (OBS duyuru modeli: özet, içerik, tür, yayın penceresi). Ders duyuruları
-- ders alanının Duyurular forumundadır. Yayımlama yetkisi (announcement:publish) birim kapsamında:
-- hedef kitle yayımlayanın kapsamındaki birimlerden seçilir.
CREATE TABLE communication.announcements (
    id                uuid        PRIMARY KEY DEFAULT uuidv7(),
    title             text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    summary           text        NOT NULL DEFAULT '' CHECK (char_length(summary) <= 500),
    body_html         text        NOT NULL DEFAULT '',
    announcement_type text        NOT NULL CHECK (announcement_type IN ('GENERAL', 'ACADEMIC', 'EXAM', 'URGENT', 'EVENT')),
    publish_from      timestamptz NOT NULL DEFAULT now(),
    publish_until     timestamptz,
    is_pinned         boolean     NOT NULL DEFAULT false,
    status            text        NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'ARCHIVED')),
    author_id         uuid        NOT NULL REFERENCES iam.users (id),
    -- Yayın başladığında hedef kitleye bir kez bildirilir; ileri tarihli olanları worker bildirir.
    notified_at       timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    version           integer     NOT NULL DEFAULT 1,
    CONSTRAINT announcements_window_check CHECK (publish_until IS NULL OR publish_until > publish_from)
);
-- Bildirilmeyi bekleyen yayımlanmış duyurular (worker).
CREATE INDEX announcements_due_idx ON communication.announcements (publish_from) WHERE status = 'PUBLISHED' AND notified_at IS NULL;
CREATE INDEX announcements_published_idx ON communication.announcements (publish_from DESC) WHERE status = 'PUBLISHED';

-- Hedef kitle: birim (üniversite, fakülte, bölüm, program) ve kişiler (herkes, öğrenciler,
-- personel). Bir duyurunun birden çok hedefi olabilir; program hedefi sadece öğrencileri kapsar.
CREATE TABLE communication.announcement_audiences (
    announcement_id uuid NOT NULL REFERENCES communication.announcements (id) ON DELETE CASCADE,
    scope_type      text NOT NULL CHECK (scope_type IN ('UNIVERSITY', 'FACULTY', 'DEPARTMENT', 'PROGRAM')),
    scope_id        uuid,
    audience        text NOT NULL CHECK (audience IN ('ALL', 'STUDENTS', 'STAFF')),
    CONSTRAINT announcement_audiences_scope_check CHECK ((scope_type = 'UNIVERSITY') = (scope_id IS NULL)),
    CONSTRAINT announcement_audiences_key UNIQUE NULLS NOT DISTINCT (announcement_id, scope_type, scope_id, audience)
);
CREATE INDEX announcement_audiences_scope_idx ON communication.announcement_audiences (scope_id);

CREATE TABLE communication.announcement_reads (
    announcement_id uuid        NOT NULL REFERENCES communication.announcements (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES iam.users (id),
    read_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (announcement_id, user_id)
);

-- Ders duyuruları ders alanının Duyurular forumundadır (öğretim elemanı orada yazar): kurumsal
-- duyuru yetkisi birim yöneticilerinde ve öğrenci işlerinde kalır. Öğretim elemanı rolü bölüm
-- kapsamında atandığı için bu yetkiyle bütün bölüme duyuru yazabilirdi.
DELETE FROM iam.role_permissions
WHERE role_id = (SELECT id FROM iam.roles WHERE code = 'INSTRUCTOR')
  AND permission_id = (SELECT id FROM iam.permissions WHERE code = 'announcement:publish');

INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order) VALUES
    ('announcement.published', 'ANNOUNCEMENTS', 'Yeni duyuru',  'New announcement',    '{IN_APP}',       false, 50),
    ('announcement.urgent',    'ANNOUNCEMENTS', 'Acil duyuru',  'Urgent announcement', '{IN_APP,EMAIL}', false, 51);

-- +goose Down
INSERT INTO iam.role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM iam.roles r, iam.permissions p WHERE r.code = 'INSTRUCTOR' AND p.code = 'announcement:publish';
DELETE FROM communication.notification_preferences WHERE type_code LIKE 'announcement.%';
DELETE FROM communication.notifications WHERE type_code LIKE 'announcement.%';
DELETE FROM communication.notification_types WHERE code LIKE 'announcement.%';
DROP TABLE communication.announcement_reads;
DROP TABLE communication.announcement_audiences;
DROP TABLE communication.announcements;
