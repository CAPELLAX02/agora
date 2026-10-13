-- +goose Up

-- Ankara Üniversitesi Ön Lisans ve Lisans Eğitim-Öğretim Yönetmeliği'nin güncel hâli
-- (RG 04/11/2022-32003 ve RG 30/09/2023-32325 değişiklikleri). Eski ölçek ve parametre
-- değerleri silinmez: eski notlar kendi ölçekleriyle, geçmiş dönemler o dönemde geçerli
-- parametrelerle yorumlanmaya devam eder.

-- Finale girme hakkı olduğu hâlde girmeyen öğrencinin harfi (F3). Devamsızlık harfi gibi puanla
-- değil durumla verilir; bir ölçekte her ikisinden de en fazla bir tane olur.
ALTER TABLE curriculum.grade_scale_items
    ADD COLUMN is_final_absent boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT grade_scale_items_status_letter_check
        CHECK (NOT (is_final_absent AND is_attendance_fail)
               AND ((NOT is_final_absent AND NOT is_attendance_fail) OR min_score IS NULL));
CREATE UNIQUE INDEX grade_scale_items_final_absent_idx ON curriculum.grade_scale_items (grade_scale_id) WHERE is_final_absent;
CREATE UNIQUE INDEX grade_scale_items_attendance_fail_idx ON curriculum.grade_scale_items (grade_scale_id) WHERE is_attendance_fail;

-- md. 35/7-a, 37: C3 ve üstü başarılı; F1 (50-59) başarısız ama 1,50 katsayıyla ortalamaya
-- girer; F2 final ya da bütünleme 50'nin altı ya da puanı 49 ve altı; F3 finale girmedi; F4
-- devamsızlık (final ve bütünleme hakkı yok).
UPDATE curriculum.grade_scales SET is_default = false, version = version + 1, updated_at = now() WHERE is_default;
INSERT INTO curriculum.grade_scales (code, name_tr, name_en, effective_from_year, is_default)
VALUES ('AU-LISANS-2023', 'Ankara Üniversitesi önlisans ve lisans not ölçeği (2023)',
        'Ankara University undergraduate grading scale (2023)', 2023, true);

INSERT INTO curriculum.grade_scale_items
    (grade_scale_id, letter, coefficient, min_score, max_score, is_passing, counts_in_gpa, earns_ects,
     is_attendance_fail, is_final_absent, sort_order)
SELECT s.id, v.letter, v.coefficient, v.min_score, v.max_score, v.is_passing, v.counts_in_gpa, v.earns_ects,
       v.is_attendance_fail, v.is_final_absent, v.sort_order
FROM curriculum.grade_scales s
CROSS JOIN (VALUES
    ('A',    4.00, 90,   100,  true,  true,  true,  false, false, 1),
    ('B1',   3.50, 85,   89,   true,  true,  true,  false, false, 2),
    ('B2',   3.25, 80,   84,   true,  true,  true,  false, false, 3),
    ('B3',   3.00, 75,   79,   true,  true,  true,  false, false, 4),
    ('C1',   2.75, 70,   74,   true,  true,  true,  false, false, 5),
    ('C2',   2.50, 65,   69,   true,  true,  true,  false, false, 6),
    ('C3',   2.00, 60,   64,   true,  true,  true,  false, false, 7),
    ('F1',   1.50, 50,   59,   false, true,  false, false, false, 8),
    ('F2',   0.00, 0,    49,   false, true,  false, false, false, 9),
    ('F3',   0.00, NULL, NULL, false, true,  false, false, true,  10),
    ('F4',   0.00, NULL, NULL, false, true,  false, true,  false, 11),
    ('BŞR',  NULL, NULL, NULL, true,  false, true,  false, false, 12),
    ('BŞZ',  NULL, NULL, NULL, false, false, false, false, false, 13),
    ('MUAF', NULL, NULL, NULL, true,  false, true,  false, false, 14)
) AS v(letter, coefficient, min_score, max_score, is_passing, counts_in_gpa, earns_ects, is_attendance_fail, is_final_absent, sort_order)
WHERE s.code = 'AU-LISANS-2023';

-- Notu henüz verilmemiş kesin kayıtlar, yeni yönetmeliğin geçerli olduğu dönemlerde yeni
-- ölçekle notlanır. Notu verilmiş kayıtlar kendi ölçeğinde kalır.
UPDATE enrollment.enrollments e
SET grade_scale_id = (SELECT id FROM curriculum.grade_scales WHERE code = 'AU-LISANS-2023'),
    version = e.version + 1, updated_at = now()
FROM academic.terms t
WHERE t.id = e.term_id AND t.starts_on >= '2023-09-30' AND e.status = 'ENROLLED';

-- Parametreler: eski değer kapanır, yenisi kendi başlangıç tarihiyle eklenir.
UPDATE curriculum.regulation_parameters SET effective_to = '2022-11-04'
WHERE key = 'honor_gpa' AND effective_from = '2016-09-01';
UPDATE curriculum.regulation_parameters SET effective_to = '2023-09-30'
WHERE key IN ('pass_min_score', 'makeup_eligible_letters') AND effective_from = '2016-09-01';

INSERT INTO curriculum.regulation_parameters (key, value, effective_from, description_tr, note) VALUES
    ('honor_gpa', '{"HONOR": 3.00, "HIGH_HONOR": 3.70}', '2022-11-04',
     'Onur (üstün başarı) ve yüksek onur (çok üstün başarı) öğrencisi için en düşük dönem not ortalaması',
     'RG 04/11/2022-32003, md. 42: 3,00-3,69 üstün, 3,70 ve üstü çok üstün başarı'),
    ('pass_min_score', '60', '2023-09-30',
     'Dersten başarılı sayılmak için en düşük ders başarı puanı',
     'RG 30/09/2023-32325, md. 37/4: final ya da bütünlemeden en az 50 almak koşuluyla en az 60'),
    ('makeup_eligible_letters', '["F1", "F2", "F3", "BŞZ"]', '2023-09-30',
     'Bütünleme sınavına girme hakkı veren harf notları (devamsızlıktan kalanlar hariç)',
     'md. 29/1-a: finale girmeyen ya da F1, F2, F3 alan öğrenci; F4 (devamsızlık) giremez'),
    ('final_exam_weight', '60', '2023-09-30',
     'Ders başarı notunda final (ya da bütünleme) sınavının ağırlığı (yüzde); dönem içi değerlendirmeler kalanı oluşturur',
     'RG 30/09/2023-32325, md. 35/3: dönem içi %40, final ya da bütünleme %60');

-- +goose Down
DELETE FROM curriculum.regulation_parameters
WHERE (key = 'honor_gpa' AND effective_from = '2022-11-04')
   OR (key IN ('pass_min_score', 'makeup_eligible_letters', 'final_exam_weight') AND effective_from = '2023-09-30');
UPDATE curriculum.regulation_parameters SET effective_to = NULL
WHERE key IN ('honor_gpa', 'pass_min_score', 'makeup_eligible_letters') AND effective_from = '2016-09-01';

UPDATE enrollment.enrollments e
SET grade_scale_id = (SELECT id FROM curriculum.grade_scales WHERE code = 'AU-LISANS'),
    version = e.version + 1, updated_at = now()
WHERE e.status = 'ENROLLED' AND e.grade_scale_id = (SELECT id FROM curriculum.grade_scales WHERE code = 'AU-LISANS-2023');
UPDATE curriculum.grade_scales SET is_default = false WHERE code = 'AU-LISANS-2023';
DELETE FROM curriculum.grade_scales WHERE code = 'AU-LISANS-2023';
UPDATE curriculum.grade_scales SET is_default = true WHERE code = 'AU-LISANS';

DROP INDEX curriculum.grade_scale_items_attendance_fail_idx;
DROP INDEX curriculum.grade_scale_items_final_absent_idx;
ALTER TABLE curriculum.grade_scale_items
    DROP CONSTRAINT grade_scale_items_status_letter_check,
    DROP COLUMN is_final_absent;
