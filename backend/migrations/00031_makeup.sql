-- +goose Up

-- Bütünleme sonuçlarının ilanı: harf notları ilan edildikten sonra, bütünleme hakkı olan
-- öğrencilerin bütünleme notuyla harfleri yeniden hesaplanır. Bir kez yapılır.
ALTER TABLE offering.sections
    ADD COLUMN makeup_published_at timestamptz,
    ADD COLUMN makeup_published_by uuid REFERENCES iam.users (id),
    ADD CONSTRAINT sections_makeup_after_grades_check
        CHECK (makeup_published_at IS NULL OR grades_published_at IS NOT NULL);

-- Not geçmişi: bir kesin kayda verilen her harf (ilan, bütünleme, not düzeltme). Kesin
-- kayıttaki harf her zaman son satırdır; öncekiler transkript ve itirazlar için saklanır.
CREATE TABLE grading.grade_history (
    id             bigint       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    enrollment_id  uuid         NOT NULL REFERENCES enrollment.enrollments (id),
    kind           text         NOT NULL CHECK (kind IN ('PUBLISHED', 'MAKEUP', 'CORRECTION')),
    letter         text         NOT NULL,
    weighted_score numeric(5,2),
    letter_reason  text,
    recorded_by    uuid         REFERENCES iam.users (id),
    recorded_at    timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX grade_history_enrollment_idx ON grading.grade_history (enrollment_id, id);

-- Hangi harflerin bütünleme hakkı verdiği (yol haritası §5 #16). Devamsız öğrenci (F1, ya da
-- başarılı/başarısız notlu derste devamsızlıktan BŞZ) bütünlemeye giremez.
INSERT INTO curriculum.regulation_parameters (key, value, effective_from, description_tr) VALUES
    ('makeup_eligible_letters', '["F2", "BŞZ"]', '2016-09-01',
     'Bütünleme sınavına girme hakkı veren harf notları (devamsızlıktan kalanlar hariç)');

-- +goose Down
DELETE FROM curriculum.regulation_parameters WHERE key = 'makeup_eligible_letters';
DROP TABLE grading.grade_history;
ALTER TABLE offering.sections
    DROP CONSTRAINT sections_makeup_after_grades_check,
    DROP COLUMN makeup_published_by,
    DROP COLUMN makeup_published_at;
