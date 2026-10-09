-- +goose Up
-- Not ölçeği: harf notları, katsayıları ve puan aralıkları. Yönetmelik değişince yeni
-- ölçek tanımlanır; eski notlar kendi ölçekleriyle yorumlanmaya devam eder.
CREATE TABLE curriculum.grade_scales (
    id                  uuid        PRIMARY KEY DEFAULT uuidv7(),
    code                text        NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9-]{2,30}$'),
    name_tr             text        NOT NULL CHECK (btrim(name_tr) <> ''),
    name_en             text        NOT NULL CHECK (btrim(name_en) <> ''),
    effective_from_year smallint    NOT NULL CHECK (effective_from_year BETWEEN 1946 AND 2100),
    is_default          boolean     NOT NULL DEFAULT false,
    version             integer     NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX grade_scales_single_default_idx ON curriculum.grade_scales ((true)) WHERE is_default;

-- Puan aralığı, ders başarı puanının tam sayıya yuvarlanmış hali için kapsayıcıdır
-- ([80, 89] → B1). Aralığı olmayan harfler puanla değil durumla verilir: F1 devamsızlık,
-- BŞR/BŞZ geçti/kaldı dersleri, MUAF muafiyet.
CREATE TABLE curriculum.grade_scale_items (
    id                 uuid         PRIMARY KEY DEFAULT uuidv7(),
    grade_scale_id     uuid         NOT NULL REFERENCES curriculum.grade_scales (id) ON DELETE CASCADE,
    letter             text         NOT NULL CHECK (letter ~ '^[A-ZÇĞİÖŞÜ][A-ZÇĞİÖŞÜ0-9]{0,4}$'),
    coefficient        numeric(3,2) CHECK (coefficient BETWEEN 0 AND 4),
    min_score          numeric(5,2) CHECK (min_score BETWEEN 0 AND 100),
    max_score          numeric(5,2) CHECK (max_score BETWEEN 0 AND 100),
    is_passing         boolean      NOT NULL,
    counts_in_gpa      boolean      NOT NULL, -- ağırlıklı not ortalamasına girer mi?
    earns_ects         boolean      NOT NULL, -- AKTS kazandırır mı?
    is_attendance_fail boolean      NOT NULL DEFAULT false,
    sort_order         smallint     NOT NULL,

    CONSTRAINT grade_scale_items_letter_key UNIQUE (grade_scale_id, letter),
    CONSTRAINT grade_scale_items_range_check CHECK ((min_score IS NULL) = (max_score IS NULL) AND min_score <= max_score),
    CONSTRAINT grade_scale_items_gpa_check CHECK (NOT counts_in_gpa OR coefficient IS NOT NULL),
    CONSTRAINT grade_scale_items_no_overlap EXCLUDE USING gist (
        grade_scale_id WITH =,
        numrange(min_score, max_score, '[]') WITH &&
    ) WHERE (min_score IS NOT NULL)
);

-- Yönetmelik parametreleri: kurallar kodda sabit değil, tarihli veri olarak tutulur. Bir
-- değer değişince eskisi kapanır, yenisi kendi başlangıç tarihiyle eklenir; geçmiş bir
-- dönemin kararı o dönemde geçerli değerle yeniden üretilebilir.
CREATE TABLE curriculum.regulation_parameters (
    key            text        NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{2,63}$'),
    value          jsonb       NOT NULL,
    effective_from date        NOT NULL,
    effective_to   date,       -- hariç; boşsa hâlâ geçerli
    description_tr text        NOT NULL,
    note           text,
    created_at     timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (key, effective_from),
    CONSTRAINT regulation_parameters_period_check CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT regulation_parameters_no_overlap EXCLUDE USING gist (
        key WITH =,
        daterange(effective_from, effective_to) WITH &&
    )
);

-- Ankara Üniversitesi Önlisans ve Lisans Eğitim-Öğretim Yönetmeliği not ölçeği.
INSERT INTO curriculum.grade_scales (code, name_tr, name_en, effective_from_year, is_default)
VALUES ('AU-LISANS', 'Ankara Üniversitesi önlisans ve lisans not ölçeği', 'Ankara University undergraduate grading scale', 2016, true);

INSERT INTO curriculum.grade_scale_items
    (grade_scale_id, letter, coefficient, min_score, max_score, is_passing, counts_in_gpa, earns_ects, is_attendance_fail, sort_order)
SELECT s.id, v.letter, v.coefficient, v.min_score, v.max_score, v.is_passing, v.counts_in_gpa, v.earns_ects, v.is_attendance_fail, v.sort_order
FROM curriculum.grade_scales s
CROSS JOIN (VALUES
    ('A',    4.00, 90,   100,  true,  true,  true,  false, 1),
    ('B1',   3.50, 80,   89,   true,  true,  true,  false, 2),
    ('B2',   3.25, 75,   79,   true,  true,  true,  false, 3),
    ('B3',   3.00, 70,   74,   true,  true,  true,  false, 4),
    ('C1',   2.75, 65,   69,   true,  true,  true,  false, 5),
    ('C2',   2.50, 60,   64,   true,  true,  true,  false, 6),
    ('C3',   2.25, 50,   59,   true,  true,  true,  false, 7),
    ('F2',   0.00, 0,    49,   false, true,  false, false, 8),
    ('F1',   0.00, NULL, NULL, false, true,  false, true,  9),
    ('BŞR',  NULL, NULL, NULL, true,  false, true,  false, 10),
    ('BŞZ',  NULL, NULL, NULL, false, false, false, false, 11),
    ('MUAF', NULL, NULL, NULL, true,  false, true,  false, 12)
) AS v(letter, coefficient, min_score, max_score, is_passing, counts_in_gpa, earns_ects, is_attendance_fail, sort_order)
WHERE s.code = 'AU-LISANS';

INSERT INTO curriculum.regulation_parameters (key, value, effective_from, description_tr) VALUES
    ('ects_limit_by_gpa', '[{"max_gpa": 1.99, "ects": 30}, {"max_gpa": 2.99, "ects": 40}, {"max_gpa": 4.00, "ects": 45}]', '2016-09-01',
     'Genel not ortalamasına göre bir yarıyılda alınabilecek en fazla AKTS'),
    ('semester_ects_load', '30', '2016-09-01',
     'Ders planında bir yarıyılın standart AKTS yükü'),
    ('final_min_score', '50', '2016-09-01',
     'Başarı notunun hesaplanabilmesi için final ya da bütünleme sınavında alınması gereken en düşük puan'),
    ('pass_min_score', '50', '2016-09-01',
     'Dersten başarılı sayılmak için en düşük ders başarı puanı'),
    ('attendance_max_absence_pct', '{"THEORY": 30, "PRACTICE": 20}', '2016-09-01',
     'Devamsızlıktan kalmadan kaçırılabilecek en fazla ders saati oranı (yüzde)'),
    ('max_courses_after_max_duration', '5', '2016-09-01',
     'Azami süre sonunda sınav hakkı tanınan en fazla ders sayısı'),
    ('honor_gpa', '{"HONOR": 3.00, "HIGH_HONOR": 3.50}', '2016-09-01',
     'Onur ve yüksek onur öğrencisi için en düşük dönem not ortalaması');

UPDATE iam.permissions SET description = 'Not ölçeği ve yönetmelik parametresi yönetimi' WHERE code = 'gradescale:manage';

-- +goose Down
UPDATE iam.permissions SET description = 'Not ölçeği yönetimi' WHERE code = 'gradescale:manage';
DROP TABLE curriculum.regulation_parameters;
DROP TABLE curriculum.grade_scale_items;
DROP TABLE curriculum.grade_scales;
