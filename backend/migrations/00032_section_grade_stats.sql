-- +goose Up

-- Sınıf istatistikleri: şubenin ilan edilmiş notlarının özeti. Öğrenci kendi notunun yanında
-- sınıfın ortalamasını görür. Önbellektir: kesin kayıtlardan yeniden üretilir ve her ilanda
-- (harf notu, bütünleme, not düzeltme) aynı transaction'da güncellenir.
CREATE TABLE grading.section_grade_stats (
    section_id          uuid         PRIMARY KEY REFERENCES offering.sections (id),
    -- Harf notu ilan edilen öğrenciler; ortalama, medyan ve standart sapma ders başarı puanı
    -- olanlardan (scored_count) hesaplanır: devamsızlıktan kalanın puanı yoktur.
    student_count       int          NOT NULL CHECK (student_count > 0),
    scored_count        int          NOT NULL,
    passed_count        int          NOT NULL,
    mean                numeric(5,2),
    median              numeric(5,2),
    stddev              numeric(5,2),
    letter_distribution jsonb        NOT NULL,
    computed_at         timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT section_grade_stats_counts_check
        CHECK (scored_count BETWEEN 0 AND student_count AND passed_count BETWEEN 0 AND student_count),
    CONSTRAINT section_grade_stats_scored_check
        CHECK ((scored_count = 0 AND mean IS NULL AND median IS NULL AND stddev IS NULL)
            OR (scored_count > 0 AND mean IS NOT NULL AND median IS NOT NULL AND stddev IS NOT NULL)),
    CONSTRAINT section_grade_stats_distribution_check
        CHECK (jsonb_typeof(letter_distribution) = 'object')
);

-- +goose Down
DROP TABLE grading.section_grade_stats;
