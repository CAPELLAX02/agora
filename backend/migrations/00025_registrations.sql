-- +goose Up

-- Ders seçme: öğrencinin bir dönemdeki sepeti (kaydı), sepetteki şubeler ve olay günlüğü.
-- Kontenjan sepete eklerken ayrılır: offering.sections.enrolled_count, dönemin aktif sepet
-- satırlarının sayısıdır (koşullu atomik UPDATE ile artar, çıkarınca azalır).
CREATE TABLE enrollment.registrations (
    id                 uuid         PRIMARY KEY DEFAULT uuidv7(),
    student_program_id uuid         NOT NULL REFERENCES enrollment.student_programs (id),
    term_id            uuid         NOT NULL REFERENCES academic.terms (id),
    status             text         NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT', 'SUBMITTED', 'APPROVED', 'REJECTED')),
    total_ects         numeric(4,1) NOT NULL DEFAULT 0 CHECK (total_ects >= 0),
    -- Gönderim anının fotoğrafı: danışman öğrencinin hangi sınırla seçtiğini görür.
    max_ects_allowed   numeric(4,1),
    gpa_snapshot       numeric(3,2),
    submitted_at       timestamptz,
    decided_at         timestamptz,
    decided_by         uuid         REFERENCES iam.users (id),
    decision_note      text         CHECK (char_length(decision_note) <= 1000),
    -- Öğrenci işlerinin bu kayda özel tanıdığı ek süre (pencere dışı, gerekçeli).
    extension_until    timestamptz,
    extension_reason   text,
    extension_by       uuid         REFERENCES iam.users (id),
    version            integer      NOT NULL DEFAULT 1,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    updated_at         timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT registrations_program_term_key UNIQUE (student_program_id, term_id),
    CONSTRAINT registrations_submitted_check CHECK (status = 'DRAFT' OR submitted_at IS NOT NULL),
    CONSTRAINT registrations_decided_check CHECK (status NOT IN ('APPROVED', 'REJECTED') OR decided_at IS NOT NULL),
    CONSTRAINT registrations_extension_check CHECK ((extension_until IS NULL) = (extension_reason IS NULL))
);

-- Danışman ve öğrenci işleri listeleri: dönemin onay bekleyenleri.
CREATE INDEX registrations_term_status_idx ON enrollment.registrations (term_id, status);

CREATE TABLE enrollment.registration_items (
    id                 uuid         PRIMARY KEY DEFAULT uuidv7(),
    registration_id    uuid         NOT NULL REFERENCES enrollment.registrations (id) ON DELETE CASCADE,
    section_id         uuid         NOT NULL REFERENCES offering.sections (id),
    course_id          uuid         NOT NULL REFERENCES curriculum.courses (id), -- şubenin dersi (kopya)
    item_kind          text         NOT NULL
        CHECK (item_kind IN ('NEW', 'REPEAT_FAILED', 'REPEAT_UPGRADE', 'ELECTIVE_REPLACEMENT')),
    curriculum_item_id uuid         REFERENCES curriculum.curriculum_items (id), -- karşıladığı müfredat satırı
    ects               numeric(4,1) NOT NULL CHECK (ects >= 0),
    -- Ayrılmış kontenjanlı şubede düşülen program kontenjanı; çıkarınca aynısı iade edilir.
    quota_program_id   uuid         REFERENCES org.programs (id),
    source             text         NOT NULL DEFAULT 'REGISTRATION' CHECK (source IN ('REGISTRATION', 'ADD_DROP')),
    status             text         NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'DROPPED')),
    added_at           timestamptz  NOT NULL DEFAULT now(),
    dropped_at         timestamptz,

    CONSTRAINT registration_items_dropped_check CHECK ((status = 'DROPPED') = (dropped_at IS NOT NULL))
);

-- Aynı dersin iki şubesi aynı anda sepette olamaz.
CREATE UNIQUE INDEX registration_items_active_course_idx
    ON enrollment.registration_items (registration_id, course_id) WHERE status = 'ACTIVE';
-- Kontenjan tutarlılık denetimi ve şube listesi.
CREATE INDEX registration_items_section_idx ON enrollment.registration_items (section_id) WHERE status = 'ACTIVE';

-- Durum geçmişi ve kayıt hunisi analitiğinin kaynağı.
CREATE TABLE enrollment.registration_events (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    registration_id uuid        NOT NULL REFERENCES enrollment.registrations (id) ON DELETE CASCADE,
    event_type      text        NOT NULL CHECK (event_type IN (
        'CREATED', 'ITEM_ADDED', 'ITEM_REMOVED', 'SECTION_CHANGED', 'RETURNED_TO_DRAFT', 'SUBMITTED',
        'WITHDRAWN', 'APPROVED', 'REJECTED', 'APPROVAL_REVOKED', 'EXTENSION_GRANTED', 'EXPIRED',
        'ADD_DROP_REQUESTED', 'ADD_DROP_APPROVED', 'ADD_DROP_REJECTED', 'ADD_DROP_APPLIED', 'ADD_DROP_WITHDRAWN')),
    actor_user_id   uuid        REFERENCES iam.users (id),
    payload         jsonb       NOT NULL DEFAULT '{}',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX registration_events_registration_idx ON enrollment.registration_events (registration_id, id);

-- Kesinleşmiş ders kaydı, onaylanan sepet satırından gelir.
ALTER TABLE enrollment.enrollments
    ADD COLUMN registration_item_id uuid REFERENCES enrollment.registration_items (id);
CREATE UNIQUE INDEX enrollments_registration_item_idx
    ON enrollment.enrollments (registration_item_id) WHERE registration_item_id IS NOT NULL AND status <> 'DROPPED';

-- +goose Down
ALTER TABLE enrollment.enrollments DROP COLUMN registration_item_id;
DROP TABLE enrollment.registration_events;
DROP TABLE enrollment.registration_items;
DROP TABLE enrollment.registrations;
