-- +goose Up
-- Ders açma: bir dönemde açılan dersler, şubeleri, program bazlı kontenjanları,
-- öğretim elemanları ve haftalık programı.
CREATE SCHEMA offering;

-- Gün içi saat aralığı. Haftalık program "pazartesi [09:00, 10:50)" gibi tekrarlayan
-- oturumlardan oluşur; çakışma kontrolü bu aralıkların && işleciyle yapılır.
CREATE TYPE offering.timerange AS RANGE (subtype = time);

CREATE TABLE offering.course_offerings (
    id            uuid        PRIMARY KEY DEFAULT uuidv7(),
    term_id       uuid        NOT NULL REFERENCES academic.terms (id),
    course_id     uuid        NOT NULL REFERENCES curriculum.courses (id),
    department_id uuid        NOT NULL REFERENCES org.departments (id), -- dersi açan bölüm (yetki kapsamı)
    status        text        NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED', 'OPEN', 'CLOSED', 'CANCELLED')),
    external_ref  text,       -- eski sistemdeki ders açma numarası
    note          text,
    version       integer     NOT NULL DEFAULT 1,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    -- Bir ders bir dönemde bir kez açılır; farklı bölümlerin öğrencileri için ayrı şubeler olur.
    CONSTRAINT course_offerings_term_course_key UNIQUE (term_id, course_id)
);

CREATE INDEX course_offerings_department_idx ON offering.course_offerings (term_id, department_id);
CREATE INDEX course_offerings_course_idx ON offering.course_offerings (course_id);

CREATE TABLE offering.sections (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    offering_id      uuid        NOT NULL REFERENCES offering.course_offerings (id) ON DELETE CASCADE,
    section_code     text        NOT NULL CHECK (section_code ~ '^[A-Z0-9]{1,3}$'),
    capacity         integer     NOT NULL CHECK (capacity BETWEEN 0 AND 2000),
    enrolled_count   integer     NOT NULL DEFAULT 0,
    -- OPEN: her programdan öğrenci alır. RESERVED: sadece kontenjan ayrılmış programlardan.
    quota_mode       text        NOT NULL DEFAULT 'OPEN' CHECK (quota_mode IN ('OPEN', 'RESERVED')),
    instruction_mode text        NOT NULL DEFAULT 'IN_PERSON' CHECK (instruction_mode IN ('IN_PERSON', 'ONLINE', 'HYBRID')),
    language         text        NOT NULL CHECK (language IN ('TR', 'EN')),
    status           text        NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'CANCELLED')),
    version          integer     NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT sections_offering_code_key UNIQUE (offering_id, section_code),
    -- Kontenjan aşımı veritabanı düzeyinde imkânsız: ders seçme eşzamanlılığının son savunması.
    CONSTRAINT sections_enrolled_check CHECK (enrolled_count BETWEEN 0 AND capacity)
);

-- RESERVED modda program bazlı alt kontenjan.
CREATE TABLE offering.section_quotas (
    id         uuid    PRIMARY KEY DEFAULT uuidv7(),
    section_id uuid    NOT NULL REFERENCES offering.sections (id) ON DELETE CASCADE,
    program_id uuid    NOT NULL REFERENCES org.programs (id),
    quota      integer NOT NULL CHECK (quota >= 0),
    enrolled   integer NOT NULL DEFAULT 0,

    CONSTRAINT section_quotas_section_program_key UNIQUE (section_id, program_id),
    CONSTRAINT section_quotas_enrolled_check CHECK (enrolled BETWEEN 0 AND quota)
);

-- Şubenin öğretim elemanları: ilişkiye dayalı yetkinin (not girişi, değerlendirme planı,
-- yoklama) kaynağı. Her şubenin en fazla bir sorumlu öğretim elemanı olur.
CREATE TABLE offering.section_instructors (
    section_id uuid NOT NULL REFERENCES offering.sections (id) ON DELETE CASCADE,
    staff_id   uuid NOT NULL REFERENCES people.staff (id),
    role       text NOT NULL CHECK (role IN ('PRIMARY', 'CO_INSTRUCTOR', 'ASSISTANT')),
    PRIMARY KEY (section_id, staff_id)
);

CREATE UNIQUE INDEX section_instructors_single_primary_idx ON offering.section_instructors (section_id) WHERE role = 'PRIMARY';
CREATE INDEX section_instructors_staff_idx ON offering.section_instructors (staff_id);

-- Haftalık program oturumu. term_id şubenin döneminin kopyasıdır: derslik çakışması
-- kısıtı dönem içinde çalışır ve EXCLUDE başka tabloya bakamaz.
CREATE TABLE offering.schedule_slots (
    id           uuid               PRIMARY KEY DEFAULT uuidv7(),
    section_id   uuid               NOT NULL REFERENCES offering.sections (id) ON DELETE CASCADE,
    term_id      uuid               NOT NULL REFERENCES academic.terms (id),
    day_of_week  smallint           NOT NULL CHECK (day_of_week BETWEEN 1 AND 7), -- ISO: 1 pazartesi
    time_span    offering.timerange NOT NULL,
    classroom_id uuid               REFERENCES org.classrooms (id), -- çevrim içi oturumda boş
    session_type text               NOT NULL CHECK (session_type IN ('THEORY', 'PRACTICE', 'LAB')),

    CONSTRAINT schedule_slots_time_check CHECK (
        NOT isempty(time_span) AND lower(time_span) >= '07:00' AND upper(time_span) <= '23:00'
        AND lower_inc(time_span) AND NOT upper_inc(time_span)
    ),
    -- Bir derslik aynı dönemde aynı gün ve saatte iki oturuma verilemez.
    CONSTRAINT schedule_slots_classroom_no_overlap EXCLUDE USING gist (
        term_id WITH =, classroom_id WITH =, day_of_week WITH =, time_span WITH &&
    ) WHERE (classroom_id IS NOT NULL),
    -- Bir şubenin oturumları birbiriyle çakışamaz.
    CONSTRAINT schedule_slots_section_no_overlap EXCLUDE USING gist (
        section_id WITH =, day_of_week WITH =, time_span WITH &&
    )
);

CREATE INDEX schedule_slots_term_day_idx ON offering.schedule_slots (term_id, day_of_week);

-- +goose Down
DROP TABLE offering.schedule_slots;
DROP TABLE offering.section_instructors;
DROP TABLE offering.section_quotas;
DROP TABLE offering.sections;
DROP TABLE offering.course_offerings;
DROP TYPE offering.timerange;
DROP SCHEMA offering;
