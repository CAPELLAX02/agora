-- +goose Up

-- Harf notlarının ilanı şubenin bir durumudur: ilan edilen şubede bileşen notları ve devam
-- durumları artık değişmez (not düzeltme talebiyle değişir).
ALTER TABLE offering.sections
    ADD COLUMN grades_published_at timestamptz,
    ADD COLUMN grades_published_by uuid REFERENCES iam.users (id);

-- Elle harf notu: sorumlu öğretim elemanının, elle harf notu atama penceresinde ve ilandan
-- önce, hesaplanan harfin yerine gerekçesiyle verdiği harf. İlanda kesin kayda yazılır.
CREATE TABLE grading.letter_overrides (
    enrollment_id uuid        PRIMARY KEY REFERENCES enrollment.enrollments (id),
    letter        text        NOT NULL,
    reason        text        NOT NULL CHECK (char_length(btrim(reason)) BETWEEN 5 AND 500),
    set_by        uuid        REFERENCES iam.users (id),
    set_at        timestamptz NOT NULL DEFAULT now()
);

-- Harf notunun hangi kuralla verildiği (öğrenci ve transkript için): puan aralığı,
-- devamsızlık, finale girmeme, final barajı, başarılı/başarısız notlu ders ya da elle.
-- Eski sistemden aktarılan geçmişte boştur.
ALTER TABLE enrollment.enrollments
    ADD COLUMN letter_reason text CHECK (letter_reason IN
        ('RANGE', 'ATTENDANCE_FAIL', 'FINAL_ABSENT', 'FINAL_BELOW_MIN', 'PASS_FAIL', 'MANUAL'));

-- +goose Down
ALTER TABLE enrollment.enrollments DROP COLUMN letter_reason;
DROP TABLE grading.letter_overrides;
ALTER TABLE offering.sections
    DROP COLUMN grades_published_by,
    DROP COLUMN grades_published_at;
