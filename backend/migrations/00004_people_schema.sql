-- +goose Up
CREATE SCHEMA people;

-- Gerçek kişi. Bir kişi aynı anda hem öğrenci hem personel olabilir
-- (ör. doktora öğrencisi araştırma görevlisi), bu yüzden rol değil, kimliktir.
-- TC Kimlik No (şifreli) ve iletişim bilgileri KVKK katmanıyla (Faz 9) eklenecek.
CREATE TABLE people.persons (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    first_name text        NOT NULL CHECK (btrim(first_name) <> ''),
    last_name  text        NOT NULL CHECK (btrim(last_name) <> ''),
    birth_date date,
    gender     text        CHECK (gender IN ('FEMALE', 'MALE', 'UNDISCLOSED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE people.academic_titles (
    code    text     PRIMARY KEY,
    name_tr text     NOT NULL,
    name_en text     NOT NULL,
    rank    smallint NOT NULL UNIQUE
);

INSERT INTO people.academic_titles (code, name_tr, name_en, rank) VALUES
    ('PROF',                  'Prof. Dr.',      'Prof. Dr.',              1),
    ('ASSOC_PROF',            'Doç. Dr.',       'Assoc. Prof. Dr.',       2),
    ('ASSIST_PROF',           'Dr. Öğr. Üyesi', 'Assist. Prof. Dr.',      3),
    ('LECTURER_DR',           'Öğr. Gör. Dr.',  'Lecturer Dr.',           4),
    ('LECTURER',              'Öğr. Gör.',      'Lecturer',               5),
    ('RESEARCH_ASSISTANT_DR', 'Arş. Gör. Dr.',  'Research Assistant Dr.', 6),
    ('RESEARCH_ASSISTANT',    'Arş. Gör.',      'Research Assistant',     7);

CREATE TABLE people.students (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    person_id  uuid        NOT NULL UNIQUE REFERENCES people.persons (id),
    student_no text        NOT NULL UNIQUE CHECK (student_no ~ '^[0-9]{8,11}$'),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE people.staff (
    id                    uuid        PRIMARY KEY DEFAULT uuidv7(),
    person_id             uuid        NOT NULL UNIQUE REFERENCES people.persons (id),
    staff_no              text        NOT NULL UNIQUE,
    staff_type            text        NOT NULL CHECK (staff_type IN ('ACADEMIC', 'ADMINISTRATIVE')),
    academic_title_code   text        REFERENCES people.academic_titles (code),
    primary_department_id uuid        REFERENCES org.departments (id),
    office_location       text,
    employment_status     text        NOT NULL DEFAULT 'ACTIVE'
        CHECK (employment_status IN ('ACTIVE', 'ON_LEAVE', 'LEFT')),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),

    -- Akademik unvan sadece akademik personelde olabilir.
    CONSTRAINT staff_title_academic_only_check
        CHECK (staff_type = 'ACADEMIC' OR academic_title_code IS NULL)
);

CREATE INDEX staff_primary_department_id_idx ON people.staff (primary_department_id);

-- +goose Down
DROP TABLE people.staff;
DROP TABLE people.students;
DROP TABLE people.academic_titles;
DROP TABLE people.persons;
DROP SCHEMA people;