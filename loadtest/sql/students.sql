-- Yük testinin öğrencileri: Bilgisayar Mühendisliği'nin (İngilizce) ders seçme dönemindeki
-- sentetik öğrencileri. Demo öğrencileri (seed'in elle tanımladıkları) ve engelliler hariç.
-- Güz döneminde dersleri açılan yarıyıllar: 1, 3, 5, 7.
COPY (
    SELECT s.student_no
    FROM enrollment.student_programs sp
    JOIN people.students s ON s.id = sp.student_id
    JOIN org.programs p ON p.id = sp.program_id
    JOIN iam.users u ON u.person_id = s.person_id AND u.status = 'ACTIVE'
    WHERE p.code = 'BIL-EN-NO' AND sp.status = 'ACTIVE' AND sp.curriculum_id IS NOT NULL
      AND sp.current_semester IN (1, 3, 5, 7)
      AND s.student_no !~ '^2[2-6]29'
      AND NOT EXISTS (SELECT 1 FROM enrollment.holds h WHERE h.student_program_id = sp.id AND h.released_at IS NULL)
    ORDER BY md5(s.student_no) -- yarıyıllar karışık gelsin
) TO STDOUT;
