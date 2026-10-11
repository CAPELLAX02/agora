-- +goose Up

-- Ekle-bırak: onaylanmış ders kaydında dönem başladıktan sonra ders ekleme ve bırakma
-- talepleri. Talep önce danışmana, sonra bölüm başkanına gider; ikisi de onaylayınca
-- uygulanır. Eklenecek şubenin kontenjanı talepte ayrılır (onay beklerken başkası
-- dolduramasın), red ya da geri çekmede iade edilir. Bırakılan dersin yeri uygulamada açılır.
CREATE TABLE enrollment.add_drop_requests (
    id                 uuid        PRIMARY KEY DEFAULT uuidv7(),
    registration_id    uuid        NOT NULL REFERENCES enrollment.registrations (id),
    status             text        NOT NULL DEFAULT 'PENDING_ADVISOR'
        CHECK (status IN ('PENDING_ADVISOR', 'PENDING_DEPT_HEAD', 'APPROVED', 'REJECTED', 'WITHDRAWN')),
    student_note       text        CHECK (char_length(student_note) <= 1000),
    advisor_decided_by uuid        REFERENCES iam.users (id),
    advisor_decided_at timestamptz,
    advisor_note       text        CHECK (char_length(advisor_note) <= 1000),
    head_decided_by    uuid        REFERENCES iam.users (id),
    head_decided_at    timestamptz,
    head_note          text        CHECK (char_length(head_note) <= 1000),
    version            integer     NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT add_drop_head_after_advisor_check CHECK (head_decided_at IS NULL OR advisor_decided_at IS NOT NULL)
);

-- Bir kayıtta aynı anda tek açık talep.
CREATE UNIQUE INDEX add_drop_requests_one_open_idx
    ON enrollment.add_drop_requests (registration_id) WHERE status IN ('PENDING_ADVISOR', 'PENDING_DEPT_HEAD');
CREATE INDEX add_drop_requests_status_idx ON enrollment.add_drop_requests (status, created_at);

CREATE TABLE enrollment.add_drop_items (
    id                   uuid         PRIMARY KEY DEFAULT uuidv7(),
    request_id           uuid         NOT NULL REFERENCES enrollment.add_drop_requests (id) ON DELETE CASCADE,
    action               text         NOT NULL CHECK (action IN ('ADD', 'DROP')),
    section_id           uuid         NOT NULL REFERENCES offering.sections (id),
    course_id            uuid         NOT NULL REFERENCES curriculum.courses (id),
    registration_item_id uuid         REFERENCES enrollment.registration_items (id), -- bırakılan sepet satırı
    ects                 numeric(4,1) NOT NULL CHECK (ects >= 0),
    quota_program_id     uuid         REFERENCES org.programs (id), -- eklemede düşülen program kontenjanı

    CONSTRAINT add_drop_items_action_key UNIQUE (request_id, action, course_id),
    CONSTRAINT add_drop_items_drop_check CHECK ((action = 'DROP') = (registration_item_id IS NOT NULL))
);

-- +goose Down
DROP TABLE enrollment.add_drop_items;
DROP TABLE enrollment.add_drop_requests;
