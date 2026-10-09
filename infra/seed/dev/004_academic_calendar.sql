-- Geliştirme seed'i: 2026-2027 akademik yılı, dönemleri ve takvim olayları. Tarihler
-- üniversitenin yayımladığı takvimlerin yapısını izler ama temsilîdir (resmî takvim
-- değildir). Güz dönemi içinde bulunulan dönemdir. Tekrar çalıştırılabilir.
--
-- Kapsam örneği: Mühendislik Fakültesi'nin ekle-bırak süresi üniversitenin süresinden
-- sonra ayrıca uzatılmıştır; mühendislik öğrencileri için fakülte penceresi geçerlidir.

INSERT INTO academic.academic_years (start_year, starts_on, ends_on)
VALUES (2026, '2026-09-01', '2027-08-31')
ON CONFLICT (start_year) DO NOTHING;

INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status, is_current)
SELECT y.id, v.term_type, v.code, v.starts_on::date, v.ends_on::date, v.status, v.is_current
FROM academic.academic_years y
CROSS JOIN (VALUES
    ('FALL',   '2026-FALL',   '2026-09-14', '2027-02-05', 'ACTIVE',  true),
    ('SPRING', '2026-SPRING', '2027-02-08', '2027-07-02', 'PLANNED', false),
    ('SUMMER', '2026-SUMMER', '2027-07-12', '2027-08-27', 'PLANNED', false)
) AS v(term_type, code, starts_on, ends_on, status, is_current)
WHERE y.start_year = 2026
  -- Başka bir dönem aktif işaretlenmişse (elle değiştirilmişse) dokunulmaz.
  AND NOT (v.is_current AND EXISTS (SELECT 1 FROM academic.terms WHERE is_current))
ON CONFLICT (code) DO NOTHING;

-- Olaylar: (dönem, tür, kapsam, başlangıç) dördüyle tanımlanır; var olanlar eklenmez.
INSERT INTO academic.calendar_events (term_id, event_type_code, title_tr, title_en, period, scope_type, scope_id, note)
SELECT t.id, v.type_code, v.title_tr, v.title_en, tstzrange(v.starts_at::timestamptz, v.ends_at::timestamptz, '[)'),
       v.scope_type, f.id, v.note
FROM (VALUES
    -- Güz dönemi
    ('2026-FALL', 'PRE_REGISTRATION_OSYS',  NULL, NULL, '2026-08-24 09:00+03', '2026-08-29 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'PREP_REGISTRATION',      NULL, NULL, '2026-08-31 09:00+03', '2026-09-05 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'ORIENTATION',            NULL, NULL, '2026-09-07 09:00+03', '2026-09-12 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'COURSE_REGISTRATION',    NULL, NULL, '2026-09-07 10:00+03', '2026-09-12 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'ADVISOR_APPROVAL',       NULL, NULL, '2026-09-07 10:00+03', '2026-09-14 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'DEPT_HEAD_APPROVAL',     NULL, NULL, '2026-09-12 10:00+03', '2026-09-15 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'CLASSES',                NULL, NULL, '2026-09-14 08:00+03', '2026-12-26 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'ADD_DROP_STUDENT',       NULL, NULL, '2026-09-21 10:00+03', '2026-09-26 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'ADD_DROP_ADVISOR',       NULL, NULL, '2026-09-21 10:00+03', '2026-09-28 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'ADD_DROP_DEPT_HEAD',     NULL, NULL, '2026-09-26 10:00+03', '2026-09-30 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'ADD_DROP_STUDENT',       'Ekle-bırak (Mühendislik, uzatma)', 'Add-drop (Engineering, extension)',
                                                       '2026-10-05 10:00+03', '2026-10-17 00:00+03', 'FACULTY', 'MUH',
                                                       'Fakülte kurulu kararıyla laboratuvar şubelerinin açılması için uzatıldı.'),
    ('2026-FALL', 'ADD_DROP_ADVISOR',       'Ekle-bırak danışman onayı (Mühendislik, uzatma)', 'Add-drop advisor approval (Engineering, extension)',
                                                       '2026-10-05 10:00+03', '2026-10-19 00:00+03', 'FACULTY', 'MUH', NULL),
    ('2026-FALL', 'HOLIDAY',                'Cumhuriyet Bayramı', 'Republic Day',
                                                       '2026-10-28 13:00+03', '2026-10-30 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'MIDTERM_EXAMS',          NULL, NULL, '2026-11-09 08:00+03', '2026-11-21 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'MIDTERM_GRADE_ENTRY',    NULL, NULL, '2026-11-09 08:00+03', '2026-12-05 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'SURVEY_PERIOD',          NULL, NULL, '2026-12-14 09:00+03', '2027-01-04 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'HOLIDAY',                'Yılbaşı', 'New Year''s Day',
                                                       '2027-01-01 00:00+03', '2027-01-02 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'FINAL_EXAMS',            NULL, NULL, '2026-12-28 08:00+03', '2027-01-10 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'FINAL_GRADE_ENTRY',      NULL, NULL, '2026-12-28 08:00+03', '2027-01-14 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'MANUAL_LETTER_GRADE',    NULL, NULL, '2027-01-11 08:00+03', '2027-01-15 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'MAKEUP_EXAMS',           NULL, NULL, '2027-01-18 08:00+03', '2027-01-25 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'MAKEUP_GRADE_ENTRY',     NULL, NULL, '2027-01-18 08:00+03', '2027-01-28 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-FALL', 'THREE_COURSE_EXAMS',     NULL, NULL, '2027-02-01 08:00+03', '2027-02-06 00:00+03', 'UNIVERSITY', NULL, NULL),
    -- Bahar dönemi
    ('2026-SPRING', 'DOUBLE_MAJOR_APPLICATION', NULL, NULL, '2027-01-25 09:00+03', '2027-02-01 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'COURSE_REGISTRATION',  NULL, NULL, '2027-02-01 10:00+03', '2027-02-06 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'ADVISOR_APPROVAL',     NULL, NULL, '2027-02-01 10:00+03', '2027-02-08 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'DEPT_HEAD_APPROVAL',   NULL, NULL, '2027-02-06 10:00+03', '2027-02-09 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'CLASSES',              NULL, NULL, '2027-02-08 08:00+03', '2027-05-29 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'ADD_DROP_STUDENT',     NULL, NULL, '2027-02-15 10:00+03', '2027-02-20 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'ADD_DROP_ADVISOR',     NULL, NULL, '2027-02-15 10:00+03', '2027-02-22 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'MIDTERM_EXAMS',        NULL, NULL, '2027-04-05 08:00+03', '2027-04-17 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'MIDTERM_GRADE_ENTRY',  NULL, NULL, '2027-04-05 08:00+03', '2027-05-01 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'FINAL_EXAMS',          NULL, NULL, '2027-05-31 08:00+03', '2027-06-12 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'FINAL_GRADE_ENTRY',    NULL, NULL, '2027-05-31 08:00+03', '2027-06-16 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'MAKEUP_EXAMS',         NULL, NULL, '2027-06-21 08:00+03', '2027-06-28 00:00+03', 'UNIVERSITY', NULL, NULL),
    ('2026-SPRING', 'MAKEUP_GRADE_ENTRY',   NULL, NULL, '2027-06-21 08:00+03', '2027-07-01 00:00+03', 'UNIVERSITY', NULL, NULL)
) AS v(term_code, type_code, title_tr, title_en, starts_at, ends_at, scope_type, faculty_code, note)
JOIN academic.terms t ON t.code = v.term_code
LEFT JOIN org.faculties f ON f.code = v.faculty_code
WHERE NOT EXISTS (
    SELECT 1 FROM academic.calendar_events e
    WHERE e.term_id = t.id AND e.event_type_code = v.type_code AND e.scope_type = v.scope_type
      AND e.scope_id IS NOT DISTINCT FROM f.id AND lower(e.period) = v.starts_at::timestamptz
);
