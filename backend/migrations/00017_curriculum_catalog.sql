-- +goose Up
-- Ders kataloğu: dersler, ön koşullar, eşdeğerlikler ve seçmeli ders grupları (havuzlar).
-- Bir dersin saati, kredisi ya da AKTS'si değişince üniversite yeni bir kod verir (ör.
-- COM4519 6 AKTS → COM4569 4 AKTS): değerler ders kaydında sabittir, eski ve yeni kod
-- eşdeğerlik tablosuyla bağlanır.
CREATE SCHEMA curriculum;

CREATE TABLE curriculum.courses (
    id                  uuid         PRIMARY KEY DEFAULT uuidv7(),
    code                text         NOT NULL UNIQUE CHECK (code ~ '^[A-Z]{2,6}[0-9]{3,4}$'),
    owner_department_id uuid         REFERENCES org.departments (id), -- boşsa üniversite ortak dersi (Türk Dili, İngilizce ...)
    name_tr             text         NOT NULL CHECK (btrim(name_tr) <> ''),
    name_en             text         NOT NULL CHECK (btrim(name_en) <> ''),
    theory_hours        smallint     NOT NULL DEFAULT 0 CHECK (theory_hours BETWEEN 0 AND 40),
    practice_hours      smallint     NOT NULL DEFAULT 0 CHECK (practice_hours BETWEEN 0 AND 40),
    national_credit     numeric(4,1) NOT NULL DEFAULT 0 CHECK (national_credit >= 0),
    ects                numeric(4,1) NOT NULL CHECK (ects >= 0),
    language            text         NOT NULL CHECK (language IN ('TR', 'EN')),
    course_kind         text         NOT NULL DEFAULT 'REGULAR'
        CHECK (course_kind IN ('REGULAR', 'NON_CREDIT', 'INTERNSHIP', 'PROJECT', 'PREP', 'ACTIVITY')),
    grading_mode        text         NOT NULL DEFAULT 'LETTER' CHECK (grading_mode IN ('LETTER', 'PASS_FAIL')),
    description_tr      text,
    description_en      text,
    learning_outcomes   jsonb        NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(learning_outcomes) = 'array'),
    is_active           boolean      NOT NULL DEFAULT true,
    version             integer      NOT NULL DEFAULT 1,
    created_at          timestamptz  NOT NULL DEFAULT now(),
    updated_at          timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX courses_owner_department_idx ON curriculum.courses (owner_department_id);
-- Kod ya da adla arama (ILIKE '%...%').
CREATE INDEX courses_search_idx ON curriculum.courses
    USING gin ((code || ' ' || name_tr || ' ' || name_en) gin_trgm_ops);

-- Ön koşul: aynı group_no içindeki satırlar VEYA, farklı gruplar VE ile bağlanır.
-- (1: COM1002) VE (2: MTH0144 VEYA MTH0142) gibi.
CREATE TABLE curriculum.course_prerequisites (
    id                     uuid     PRIMARY KEY DEFAULT uuidv7(),
    course_id              uuid     NOT NULL REFERENCES curriculum.courses (id) ON DELETE CASCADE,
    prerequisite_course_id uuid     NOT NULL REFERENCES curriculum.courses (id),
    requirement            text     NOT NULL DEFAULT 'PASSED' CHECK (requirement IN ('PASSED', 'ATTENDED')),
    group_no               smallint NOT NULL DEFAULT 1 CHECK (group_no BETWEEN 1 AND 20),

    CONSTRAINT course_prerequisites_pair_key UNIQUE (course_id, prerequisite_course_id),
    CONSTRAINT course_prerequisites_self_check CHECK (course_id <> prerequisite_course_id)
);

CREATE INDEX course_prerequisites_prerequisite_idx ON curriculum.course_prerequisites (prerequisite_course_id);

-- Eşdeğerlik: eski ↔ yeni kod (intibak). Tekrar alınan dersin eski kayda bağlanması ve
-- müfredattan çıkan dersin yerine geçecek ders buradan bulunur.
CREATE TABLE curriculum.course_equivalences (
    id                   uuid        PRIMARY KEY DEFAULT uuidv7(),
    course_id            uuid        NOT NULL REFERENCES curriculum.courses (id) ON DELETE CASCADE,
    equivalent_course_id uuid        NOT NULL REFERENCES curriculum.courses (id) ON DELETE CASCADE,
    is_bidirectional     boolean     NOT NULL DEFAULT true,
    valid_from_year      smallint    CHECK (valid_from_year BETWEEN 1946 AND 2100),
    note                 text,
    created_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT course_equivalences_pair_key UNIQUE (course_id, equivalent_course_id),
    CONSTRAINT course_equivalences_self_check CHECK (course_id <> equivalent_course_id)
);

CREATE INDEX course_equivalences_equivalent_idx ON curriculum.course_equivalences (equivalent_course_id);

-- Seçmeli ders grubu: müfredatta tek satır gibi duran bir havuz (COMTE02 "2. sınıf teknik
-- seçmeliler", UNVGOFECG "üniversite alan dışı seçmeliler").
CREATE TABLE curriculum.elective_groups (
    id                  uuid        PRIMARY KEY DEFAULT uuidv7(),
    code                text        NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9]{3,20}$'),
    name_tr             text        NOT NULL CHECK (btrim(name_tr) <> ''),
    name_en             text        NOT NULL CHECK (btrim(name_en) <> ''),
    owner_department_id uuid        REFERENCES org.departments (id), -- boşsa üniversite geneli havuz
    group_kind          text        NOT NULL
        CHECK (group_kind IN ('TECHNICAL', 'UNIVERSITY_GENERAL', 'PEDAGOGICAL', 'SOCIAL', 'FREE')),
    is_active           boolean     NOT NULL DEFAULT true,
    version             integer     NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE curriculum.elective_group_courses (
    elective_group_id uuid NOT NULL REFERENCES curriculum.elective_groups (id) ON DELETE CASCADE,
    course_id         uuid NOT NULL REFERENCES curriculum.courses (id),
    PRIMARY KEY (elective_group_id, course_id)
);

CREATE INDEX elective_group_courses_course_idx ON curriculum.elective_group_courses (course_id);

-- +goose Down
DROP TABLE curriculum.elective_group_courses;
DROP TABLE curriculum.elective_groups;
DROP TABLE curriculum.course_equivalences;
DROP TABLE curriculum.course_prerequisites;
DROP TABLE curriculum.courses;
DROP SCHEMA curriculum;
