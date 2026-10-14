-- +goose Up

-- Öğrenme yönetimi (e-Kampüs'ün karşılığı). Ders alanının anahtarı şubedir (yol haritası
-- §5 #19): şube başına bir alan, ilk içerikle oluşur. Üyelik saklanmaz, kesin kayıt ve
-- öğretim elemanı atamasından sorgu anında türetilir.
CREATE SCHEMA lms;

CREATE TABLE lms.course_spaces (
    section_id   uuid        PRIMARY KEY REFERENCES offering.sections (id),
    summary_html text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- Bölümler: hafta ya da konu. Sıra position'dadır (boşluklu olabilir).
CREATE TABLE lms.topics (
    id              uuid        PRIMARY KEY DEFAULT uuidv7(),
    section_id      uuid        NOT NULL REFERENCES lms.course_spaces (section_id),
    title           text        NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    summary_html    text        NOT NULL DEFAULT '',
    position        integer     NOT NULL,
    is_visible      boolean     NOT NULL DEFAULT true,
    available_from  timestamptz,
    available_until timestamptz,
    version         integer     NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT topics_availability_check CHECK (available_from IS NULL OR available_until IS NULL OR available_from < available_until)
);
CREATE INDEX topics_section_idx ON lms.topics (section_id, position) WHERE deleted_at IS NULL;

-- Etkinlikler: ortak alanlar burada, türe özgü alanlar alt tablolarda (class-table inheritance).
-- Etiket (LABEL) sadece açıklamadır; ödev ve forum sonraki migration'larda.
CREATE TABLE lms.activities (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    section_id       uuid        NOT NULL REFERENCES lms.course_spaces (section_id),
    topic_id         uuid        NOT NULL REFERENCES lms.topics (id),
    activity_type    text        NOT NULL CHECK (activity_type IN ('RESOURCE', 'FOLDER', 'URL', 'PAGE', 'LABEL', 'ASSIGNMENT', 'FORUM')),
    title            text        NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    description_html text        NOT NULL DEFAULT '',
    position         integer     NOT NULL,
    is_visible       boolean     NOT NULL DEFAULT true,
    available_from   timestamptz,
    available_until  timestamptz,
    created_by       uuid        REFERENCES iam.users (id),
    version          integer     NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz,
    CONSTRAINT activities_availability_check CHECK (available_from IS NULL OR available_until IS NULL OR available_from < available_until)
);
CREATE INDEX activities_topic_idx ON lms.activities (topic_id, position) WHERE deleted_at IS NULL;
CREATE INDEX activities_section_idx ON lms.activities (section_id) WHERE deleted_at IS NULL;

-- Dosya (tek dosya) ve klasör (çok dosya) etkinliklerinin dosyaları.
CREATE TABLE lms.activity_files (
    activity_id uuid    NOT NULL REFERENCES lms.activities (id),
    file_id     uuid    NOT NULL REFERENCES platform.files (id),
    position    integer NOT NULL,
    PRIMARY KEY (activity_id, file_id)
);

CREATE TABLE lms.urls (
    activity_id     uuid    PRIMARY KEY REFERENCES lms.activities (id),
    url             text    NOT NULL CHECK (url ~ '^https?://' AND length(url) <= 2000),
    open_in_new_tab boolean NOT NULL DEFAULT true
);

CREATE TABLE lms.pages (
    activity_id uuid PRIMARY KEY REFERENCES lms.activities (id),
    body_html   text NOT NULL DEFAULT ''
);

-- +goose Down
DROP SCHEMA lms CASCADE;
