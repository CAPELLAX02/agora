-- +goose Up

-- Bileşen notları: öğretim elemanının bir değerlendirme bileşenine (ara sınav, ödev,
-- final ...) kesin kayıtlı öğrenci için girdiği puan ya da "girmedi" işareti. Not
-- girilmemiş bileşenin satırı yoktur. Harf notu bu satırlardan hesaplanır (Faz 4).
CREATE TABLE grading.assessment_scores (
    id            uuid         PRIMARY KEY DEFAULT uuidv7(),
    -- Notu olan bileşen silinemez (varsayılan RESTRICT): plan kilidi açılıp değiştirilse
    -- de girilmiş notlar kaybolmaz.
    component_id  uuid         NOT NULL REFERENCES grading.assessment_components (id),
    -- Notu olan kesin kayıt silinemez: not girildikten sonra ders kaydının onayı geri alınamaz.
    enrollment_id uuid         NOT NULL REFERENCES enrollment.enrollments (id),
    status        text         NOT NULL CHECK (status IN ('SCORED', 'ABSENT')),
    score         numeric(5,2) CHECK (score BETWEEN 0 AND 100),
    source        text         NOT NULL DEFAULT 'MANUAL' CHECK (source IN ('MANUAL', 'CSV', 'LMS')),
    entered_by    uuid         REFERENCES iam.users (id),
    entered_at    timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT assessment_scores_component_enrollment_key UNIQUE (component_id, enrollment_id),
    CONSTRAINT assessment_scores_status_score_check CHECK ((status = 'SCORED') = (score IS NOT NULL))
);

-- Öğrencinin bütün bileşen notları (harf notu hesabı, öğrencinin notları ekranı).
CREATE INDEX assessment_scores_enrollment_idx ON grading.assessment_scores (enrollment_id);

-- Bileşen başına iyimser kilit (aynı şubenin iki öğretim elemanı aynı anda not girebilir)
-- ve öğrencilere yayımlanma bilgisi.
ALTER TABLE grading.assessment_components
    ADD COLUMN scores_version integer     NOT NULL DEFAULT 1,
    ADD COLUMN published_at   timestamptz,
    ADD COLUMN published_by   uuid        REFERENCES iam.users (id);

-- +goose Down
ALTER TABLE grading.assessment_components
    DROP COLUMN published_by,
    DROP COLUMN published_at,
    DROP COLUMN scores_version;
DROP TABLE grading.assessment_scores;
