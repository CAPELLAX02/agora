-- Yük testinden sonra: kontenjan aşımı = 0 kanıtı.
\echo '--- Kontenjanını aşan şube (0 olmalı)'
SELECT count(*) AS asim FROM offering.sections WHERE enrolled_count > capacity;
\echo '--- Mevcudu sepetlerle tutmayan şube, içinde bulunulan dönem (0 olmalı)'
SELECT count(*) AS tutarsiz
FROM offering.sections s
JOIN offering.course_offerings o ON o.id = s.offering_id
JOIN academic.terms t ON t.id = o.term_id AND t.is_current
WHERE s.enrolled_count <> (SELECT count(*) FROM enrollment.registration_items i WHERE i.section_id = s.id AND i.status = 'ACTIVE');
\echo '--- Program kontenjanı aşımı ya da tutarsızlığı (0 olmalı)'
SELECT count(*) AS kontenjan_tutarsiz FROM offering.section_quotas q
WHERE q.enrolled > q.quota
   OR q.enrolled <> (SELECT count(*) FROM enrollment.registration_items i
                     WHERE i.section_id = q.section_id AND i.quota_program_id = q.program_id AND i.status = 'ACTIVE');
\echo '--- Aynı dersi iki kez alan sepet (0 olmalı)'
SELECT count(*) AS cift FROM (
    SELECT registration_id, course_id FROM enrollment.registration_items WHERE status = 'ACTIVE'
    GROUP BY 1, 2 HAVING count(*) > 1) x;
\echo '--- AKTS sınırını aşan sepet (0 olmalı; sınır GANO''ya göre)'
SELECT count(*) AS akts_asim
FROM enrollment.registrations r
JOIN enrollment.student_programs sp ON sp.id = r.student_program_id
WHERE r.total_ects > CASE WHEN sp.gpa_cache IS NULL THEN 30 WHEN sp.gpa_cache <= 1.99 THEN 30 WHEN sp.gpa_cache <= 2.99 THEN 40 ELSE 45 END;
\echo '--- Dolan şubeler'
SELECT c.code || '-' || s.section_code AS sube, s.capacity AS kontenjan, s.enrolled_count AS mevcut
FROM offering.sections s
JOIN offering.course_offerings o ON o.id = s.offering_id
JOIN academic.terms t ON t.id = o.term_id AND t.is_current
JOIN curriculum.courses c ON c.id = o.course_id
WHERE s.enrolled_count = s.capacity ORDER BY 1;
\echo '--- Kayıt durumları'
SELECT r.status, count(*) FROM enrollment.registrations r
JOIN academic.terms t ON t.id = r.term_id AND t.is_current GROUP BY 1 ORDER BY 1;
