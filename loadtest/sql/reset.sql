-- Yük testinden önce: sentetik öğrencilerin içinde bulunulan dönemdeki ders seçme kayıtlarını
-- siler ve şube mevcutlarını kalan sepetlerden yeniden hesaplar. Demo öğrencilerine dokunmaz.
BEGIN;
CREATE TEMP TABLE rush_registrations ON COMMIT DROP AS
SELECT r.id FROM enrollment.registrations r
JOIN enrollment.student_programs sp ON sp.id = r.student_program_id
JOIN people.students s ON s.id = sp.student_id
JOIN academic.terms t ON t.id = r.term_id AND t.is_current
WHERE s.student_no !~ '^2[2-6]29';

DELETE FROM enrollment.enrollments WHERE registration_item_id IN (
    SELECT i.id FROM enrollment.registration_items i WHERE i.registration_id IN (SELECT id FROM rush_registrations));
DELETE FROM enrollment.add_drop_requests WHERE registration_id IN (SELECT id FROM rush_registrations);
DELETE FROM enrollment.registrations WHERE id IN (SELECT id FROM rush_registrations);

UPDATE offering.sections s
SET enrolled_count = (SELECT count(*) FROM enrollment.registration_items i WHERE i.section_id = s.id AND i.status = 'ACTIVE')
FROM offering.course_offerings o JOIN academic.terms t ON t.id = o.term_id AND t.is_current
WHERE o.id = s.offering_id;
UPDATE offering.section_quotas q
SET enrolled = (SELECT count(*) FROM enrollment.registration_items i
                WHERE i.section_id = q.section_id AND i.quota_program_id = q.program_id AND i.status = 'ACTIVE');
DELETE FROM platform.idempotency_keys;
COMMIT;
