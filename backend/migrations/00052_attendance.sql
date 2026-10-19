-- +goose Up

-- Yoklama (Faz 7, §5 #21). Oturumlar şubenin haftalık programından derslerin sürdüğü dönem
-- boyunca üretilir (tatile düşenler hariç); öğretim elemanı oturumu iptal eder ya da telafi
-- oturumu ekler. Yoklama kaydı oturum ve kesin kayıt başına tektir.
CREATE SCHEMA attendance;

CREATE TABLE attendance.class_sessions (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    section_id       uuid        NOT NULL REFERENCES offering.sections (id) ON DELETE CASCADE,
    term_id          uuid        NOT NULL REFERENCES academic.terms (id),
    schedule_slot_id uuid        REFERENCES offering.schedule_slots (id) ON DELETE SET NULL, -- telafide boş
    session_type     text        NOT NULL CHECK (session_type IN ('THEORY', 'PRACTICE', 'LAB')),
    kind             text        NOT NULL DEFAULT 'REGULAR' CHECK (kind IN ('REGULAR', 'MAKEUP')),
    starts_at        timestamptz NOT NULL,
    ends_at          timestamptz NOT NULL,
    classroom_id     uuid        REFERENCES org.classrooms (id),
    -- HELD: yoklaması alındı. İptal edilen oturum devamsızlık sınırında sayılmaz.
    status           text        NOT NULL DEFAULT 'SCHEDULED' CHECK (status IN ('SCHEDULED', 'HELD', 'CANCELLED')),
    topic            text        CHECK (char_length(topic) <= 300),
    taken_at         timestamptz,
    taken_by         uuid        REFERENCES iam.users (id),
    version          integer     NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT class_sessions_section_start_key UNIQUE (section_id, starts_at),
    CONSTRAINT class_sessions_period_check CHECK (ends_at > starts_at),
    CONSTRAINT class_sessions_held_check CHECK ((status = 'HELD') = (taken_at IS NOT NULL))
);

CREATE INDEX class_sessions_term_idx ON attendance.class_sessions (term_id, section_id);

-- Yoklama kayıtları dönem başına liste partition'ındadır: bir dönem ~25 milyon satır olabilir,
-- eski dönemler ayrılır. Varsayılan partition yok: kayıt yazmadan önce dönemin partition'ı
-- attendance.ensure_term_partition ile açılır, unutulursa yazma hata verir.
CREATE TABLE attendance.attendance_records (
    term_id       uuid        NOT NULL REFERENCES academic.terms (id),
    session_id    uuid        NOT NULL REFERENCES attendance.class_sessions (id) ON DELETE CASCADE,
    enrollment_id uuid        NOT NULL REFERENCES enrollment.enrollments (id),
    status        text        NOT NULL CHECK (status IN ('PRESENT', 'ABSENT', 'LATE', 'EXCUSED')),
    method        text        NOT NULL DEFAULT 'MANUAL' CHECK (method IN ('MANUAL', 'QR')),
    recorded_by   uuid        REFERENCES iam.users (id),
    recorded_at   timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (term_id, session_id, enrollment_id)
) PARTITION BY LIST (term_id);

-- Öğrencinin raporu: kesin kaydın devamsızlıkları.
CREATE INDEX attendance_records_enrollment_idx ON attendance.attendance_records (enrollment_id) WHERE status = 'ABSENT';

-- +goose StatementBegin
CREATE FUNCTION attendance.ensure_term_partition(term uuid) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    name text;
BEGIN
    SELECT 'attendance_records_' || replace(lower(code), '-', '_') INTO name FROM academic.terms WHERE id = term;
    IF name IS NULL THEN
        RAISE EXCEPTION 'dönem yok: %', term;
    END IF;
    IF to_regclass('attendance.' || name) IS NOT NULL THEN
        RETURN;
    END IF;
    PERFORM pg_advisory_xact_lock(hashtext('attendance.ensure_term_partition'));
    EXECUTE format('CREATE TABLE IF NOT EXISTS attendance.%I PARTITION OF attendance.attendance_records FOR VALUES IN (%L)',
                   name, term);
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP SCHEMA attendance CASCADE;
