-- +goose Up

-- Kayıt engelleri: katkı payı borcu, disiplin cezası, eksik belge... Engel bir program
-- kaydına konur ve hangi işlemleri kapattığını söyler (ders seçme, belge, mezuniyet).
-- Kaldırılan engel silinmez, kaldırılma bilgisiyle geçmişte kalır.
CREATE TABLE enrollment.holds (
    id                 uuid        PRIMARY KEY DEFAULT uuidv7(),
    student_program_id uuid        NOT NULL REFERENCES enrollment.student_programs (id),
    hold_type          text        NOT NULL
        CHECK (hold_type IN ('TUITION_DEBT', 'DISCIPLINE_SUSPENSION', 'MISSING_DOCUMENT', 'LIBRARY_DEBT')),
    blocks             text[]      NOT NULL
        CHECK (cardinality(blocks) > 0 AND blocks <@ ARRAY['REGISTRATION', 'DOCUMENT', 'GRADUATION']),
    reason             text        NOT NULL CHECK (btrim(reason) <> '' AND char_length(reason) <= 500),
    placed_by          uuid        REFERENCES iam.users (id),
    placed_at          timestamptz NOT NULL DEFAULT now(),
    released_by        uuid        REFERENCES iam.users (id),
    released_at        timestamptz,
    release_reason     text        CHECK (char_length(release_reason) <= 500),

    CONSTRAINT holds_release_check CHECK ((released_at IS NULL) = (release_reason IS NULL)),
    CONSTRAINT holds_release_after_check CHECK (released_at IS NULL OR released_at >= placed_at)
);

-- Aynı türde aynı anda tek aktif engel; "ders seçebilir mi?" sorgusu da bu index'i kullanır.
CREATE UNIQUE INDEX holds_one_active_per_type_idx
    ON enrollment.holds (student_program_id, hold_type) WHERE released_at IS NULL;
-- Öğrenci işleri listesi: en yeni engel önce.
CREATE INDEX holds_placed_idx ON enrollment.holds (placed_at DESC, id DESC);

-- +goose Down
DROP TABLE enrollment.holds;
