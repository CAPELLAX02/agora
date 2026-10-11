-- +goose Up

-- Kesinleşmiş ders kayıtları: öğrencinin bir dönemde bir şubede aldığı ders ve sonucu.
-- Ders seçmede onaylanan sepetten, ekle-bıraktan ya da eski sistemden aktarımla gelir.
-- Ders geçmişi (alttan dersler, ön koşullar, GANO) bu tablodan okunur.
CREATE TABLE enrollment.enrollments (
    id                 uuid         PRIMARY KEY DEFAULT uuidv7(),
    student_program_id uuid         NOT NULL REFERENCES enrollment.student_programs (id),
    section_id         uuid         NOT NULL REFERENCES offering.sections (id),
    term_id            uuid         NOT NULL REFERENCES academic.terms (id),     -- şubenin dönemi (sorgular için kopya)
    course_id          uuid         NOT NULL REFERENCES curriculum.courses (id), -- şubenin dersi (sorgular için kopya)
    attempt_no         smallint     NOT NULL CHECK (attempt_no BETWEEN 1 AND 20), -- dersin (ya da eşdeğerinin) kaçıncı alınışı
    status             text         NOT NULL DEFAULT 'ENROLLED' CHECK (status IN ('ENROLLED', 'DROPPED', 'COMPLETED')),
    attendance_status  text         NOT NULL DEFAULT 'CONTINUING' CHECK (attendance_status IN ('CONTINUING', 'ATTENDANCE_FAIL')),
    weighted_score     numeric(5,2) CHECK (weighted_score BETWEEN 0 AND 100),
    grade_scale_id     uuid         REFERENCES curriculum.grade_scales (id),
    letter_grade       text,
    ects_snapshot      numeric(4,1) NOT NULL CHECK (ects_snapshot >= 0),  -- alındığı andaki AKTS
    credit_snapshot    numeric(4,1) NOT NULL CHECK (credit_snapshot >= 0), -- alındığı andaki ulusal kredi
    -- Tekrar alınan derste son not geçerlidir: eski denemenin yerini alan yeni deneme.
    superseded_by_id   uuid         REFERENCES enrollment.enrollments (id),
    source             text         NOT NULL DEFAULT 'REGISTRATION' CHECK (source IN ('REGISTRATION', 'ADD_DROP', 'IMPORT')),
    grade_published_at timestamptz,
    version            integer      NOT NULL DEFAULT 1,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    updated_at         timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT enrollments_program_section_key UNIQUE (student_program_id, section_id),
    -- Harf notu, kaydın bağlı olduğu not ölçeğinde tanımlı olmalı.
    CONSTRAINT enrollments_grade_fkey FOREIGN KEY (grade_scale_id, letter_grade)
        REFERENCES curriculum.grade_scale_items (grade_scale_id, letter),
    CONSTRAINT enrollments_completed_check CHECK ((status = 'COMPLETED') = (letter_grade IS NOT NULL)),
    CONSTRAINT enrollments_scale_check CHECK (letter_grade IS NULL OR grade_scale_id IS NOT NULL),
    CONSTRAINT enrollments_superseded_check CHECK (superseded_by_id <> id)
);

-- Aynı ders aynı dönemde iki kez alınamaz (bırakılan hariç).
CREATE UNIQUE INDEX enrollments_term_course_idx
    ON enrollment.enrollments (student_program_id, term_id, course_id) WHERE status <> 'DROPPED';
-- Ders geçmişi: öğrencinin bir dersi kaç kez, hangi sonuçla aldığı.
CREATE INDEX enrollments_program_course_idx ON enrollment.enrollments (student_program_id, course_id);
-- Şubenin sınıf listesi.
CREATE INDEX enrollments_section_idx ON enrollment.enrollments (section_id) WHERE status <> 'DROPPED';
CREATE INDEX enrollments_term_idx ON enrollment.enrollments (term_id);

-- +goose Down
DROP TABLE enrollment.enrollments;
