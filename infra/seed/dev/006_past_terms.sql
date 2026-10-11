-- Geliştirme seed'i: geçmiş akademik yıllar ve dönemleri (2020-2021 … 2025-2026). Öğrencilerin
-- eski sistemden aktarılan ders geçmişi bu dönemlere bağlanır (backend/cmd/seed). Tarihler
-- temsilîdir; geçmiş dönemlerin takvim olayları tutulmaz. Tekrar çalıştırılabilir.

INSERT INTO academic.academic_years (start_year, starts_on, ends_on)
SELECT y, make_date(y, 9, 1), make_date(y + 1, 8, 31)
FROM generate_series(2020, 2025) AS y
ON CONFLICT (start_year) DO NOTHING;

-- Güz dönemi eylülün ikinci yarısında, bahar dönemi şubatta başlar. Bahar dönemi, yılın
-- başladığı yılın koduyla anılır (2025-SPRING, Şubat 2026'da başlar).
INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status)
SELECT ay.id, v.term_type, ay.start_year || '-' || v.term_type,
       CASE v.term_type WHEN 'FALL' THEN make_date(ay.start_year, 9, 15) ELSE make_date(ay.start_year + 1, 2, 10) END,
       CASE v.term_type WHEN 'FALL' THEN make_date(ay.start_year + 1, 2, 5) ELSE make_date(ay.start_year + 1, 7, 3) END,
       'CLOSED'
FROM academic.academic_years ay
CROSS JOIN (VALUES ('FALL'), ('SPRING')) AS v(term_type)
WHERE ay.start_year BETWEEN 2020 AND 2025
ON CONFLICT (code) DO NOTHING;
