-- +goose Up

-- İlişkiye dayalı roller: yetkileri rolün kapsamındaki her şeyi değil, sadece
-- ilişkili kayıtları kapsar. Bölüm danışmanı bölümdeki bütün öğrencileri değil,
-- sadece danışmanı olduğu öğrencileri görebilir. Bu rollerin yetkileri route
-- düzeyinde sayılır ("öğrenci görebilir mi?") ama hiçbir birimi kapsamaz: erişim
-- politika fonksiyonlarında ilişkiyle (danışmanlık ataması) kontrol edilir.
ALTER TABLE iam.roles ADD COLUMN relationship_scoped boolean NOT NULL DEFAULT false;
UPDATE iam.roles SET relationship_scoped = true WHERE code = 'ADVISOR';

-- Yetki çözümünün sonucu değiştiği için ilgili kullanıcıların yetki sürümü artırılır.
UPDATE iam.users SET perm_version = perm_version + 1, updated_at = now()
WHERE id IN (
    SELECT ra.user_id FROM iam.role_assignments ra
    JOIN iam.roles r ON r.id = ra.role_id
    WHERE r.code = 'ADVISOR'
);

CREATE SCHEMA enrollment;

-- Öğrencinin bir programdaki kaydı. Bir öğrenci birden fazla programda olabilir
-- (çift anadal, yandal). Müfredat ve dönem bağlantıları (curriculum_id,
-- admission_term_id, max_duration_term_id) Faz 2'de akademik takvim ve müfredatla
-- birlikte eklenecek.
CREATE TABLE enrollment.student_programs (
    id                uuid         PRIMARY KEY DEFAULT uuidv7(),
    student_id        uuid         NOT NULL REFERENCES people.students (id),
    program_id        uuid         NOT NULL REFERENCES org.programs (id),
    enrollment_kind   text         NOT NULL DEFAULT 'MAJOR'
        CHECK (enrollment_kind IN ('MAJOR', 'DOUBLE_MAJOR', 'MINOR')),
    admission_type    text         NOT NULL
        CHECK (admission_type IN ('OSYS', 'DGS', 'YOS', 'TRANSFER_INTERNAL', 'TRANSFER_EXTERNAL',
                                  'SPECIAL_TALENT', 'EXCHANGE')),
    admission_year    smallint     NOT NULL CHECK (admission_year BETWEEN 1946 AND 2100),
    admitted_on       date         NOT NULL,
    status            text         NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('PREP', 'ACTIVE', 'FROZEN', 'SUSPENDED', 'GRADUATED', 'WITHDRAWN', 'DISMISSED')),
    status_changed_at timestamptz  NOT NULL DEFAULT now(),
    class_level       smallint     NOT NULL DEFAULT 1 CHECK (class_level BETWEEN 0 AND 7), -- 0: hazırlık
    current_semester  smallint     NOT NULL DEFAULT 1 CHECK (current_semester BETWEEN 1 AND 20),
    graduated_on      date,
    gpa_cache         numeric(3,2) CHECK (gpa_cache BETWEEN 0 AND 4),
    earned_ects_cache numeric(5,1) NOT NULL DEFAULT 0 CHECK (earned_ects_cache >= 0),
    version           integer      NOT NULL DEFAULT 1,
    created_at        timestamptz  NOT NULL DEFAULT now(),
    updated_at        timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT student_programs_student_program_key UNIQUE (student_id, program_id),
    CONSTRAINT student_programs_graduation_check CHECK ((status = 'GRADUATED') = (graduated_on IS NOT NULL))
);

CREATE INDEX student_programs_program_status_idx ON enrollment.student_programs (program_id, status);

-- Kayıt durumu değişikliklerinin geçmişi: dondurma, uzaklaştırma, mezuniyet...
CREATE TABLE enrollment.student_program_status_history (
    id                 bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    student_program_id uuid        NOT NULL REFERENCES enrollment.student_programs (id) ON DELETE CASCADE,
    from_status        text,
    to_status          text        NOT NULL,
    reason             text,
    decision_ref       text,        -- yönetim kurulu karar numarası vb.
    changed_by         uuid        REFERENCES iam.users (id),
    changed_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX student_program_status_history_program_idx
    ON enrollment.student_program_status_history (student_program_id, changed_at DESC);

-- Danışman atamaları. Her program kaydının aynı anda tek bir aktif danışmanı olur;
-- eski atamalar bitiş zamanıyla geçmiş olarak saklanır.
CREATE TABLE enrollment.advisor_assignments (
    id                 uuid        PRIMARY KEY DEFAULT uuidv7(),
    student_program_id uuid        NOT NULL REFERENCES enrollment.student_programs (id),
    advisor_staff_id   uuid        NOT NULL REFERENCES people.staff (id),
    valid_from         timestamptz NOT NULL DEFAULT now(),
    valid_until        timestamptz,
    assigned_by        uuid        REFERENCES iam.users (id),
    reason             text,
    created_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT advisor_assignments_period_check CHECK (valid_until IS NULL OR valid_until > valid_from)
);

CREATE UNIQUE INDEX advisor_assignments_one_active_idx
    ON enrollment.advisor_assignments (student_program_id) WHERE valid_until IS NULL;
-- "Danışmanı olduğum öğrenciler" sorgusu için.
CREATE INDEX advisor_assignments_advisor_active_idx
    ON enrollment.advisor_assignments (advisor_staff_id) WHERE valid_until IS NULL;

-- +goose Down
DROP SCHEMA enrollment CASCADE;

UPDATE iam.users SET perm_version = perm_version + 1, updated_at = now()
WHERE id IN (
    SELECT ra.user_id FROM iam.role_assignments ra
    JOIN iam.roles r ON r.id = ra.role_id
    WHERE r.relationship_scoped
);
ALTER TABLE iam.roles DROP COLUMN relationship_scoped;
