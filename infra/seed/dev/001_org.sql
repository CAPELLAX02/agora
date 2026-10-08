-- Geliştirme ortamı seed verisi: organizasyon yapısı.
--
-- Ankara Üniversitesi'nin birim yapısını TEMSİLİ olarak modeller. Resmi bir kaynak
-- değildir, birim-yerleşke eşleşmeleri gerçekle birebir uyuşmayabilir.
-- Tekrar tekrar çalıştırılabilir (idempotent): var olan kayıtlara dokunmaz.
-- Kullanım: make seed

BEGIN;

INSERT INTO org.campuses (code, name, address) VALUES
    ('GOLBASI',  '50. Yıl Yerleşkesi',  'Gölbaşı, Ankara'),
    ('TANDOGAN', 'Tandoğan Yerleşkesi', 'Beşevler, Yenimahalle, Ankara'),
    ('CEBECI',   'Cebeci Yerleşkesi',   'Cebeci, Çankaya, Ankara'),
    ('SIHHIYE',  'Sıhhiye Yerleşkesi',  'Sıhhiye, Altındağ, Ankara'),
    ('DISKAPI',  'Dışkapı Yerleşkesi',  'Dışkapı, Altındağ, Ankara')
ON CONFLICT (code) DO NOTHING;

INSERT INTO org.faculties (code, name_tr, name_en, unit_type, campus_id)
SELECT v.code, v.name_tr, v.name_en, v.unit_type, c.id
FROM (VALUES
    ('MUH',    'Mühendislik Fakültesi',                 'Faculty of Engineering',                       'FACULTY',           'GOLBASI'),
    ('FEN',    'Fen Fakültesi',                         'Faculty of Science',                           'FACULTY',           'TANDOGAN'),
    ('DTCF',   'Dil ve Tarih-Coğrafya Fakültesi',       'Faculty of Languages, History and Geography',  'FACULTY',           'SIHHIYE'),
    ('TIP',    'Tıp Fakültesi',                         'Faculty of Medicine',                          'FACULTY',           'SIHHIYE'),
    ('SBF',    'Siyasal Bilgiler Fakültesi',            'Faculty of Political Science',                 'FACULTY',           'CEBECI'),
    ('HUK',    'Hukuk Fakültesi',                       'Faculty of Law',                               'FACULTY',           'CEBECI'),
    ('ILT',    'İletişim Fakültesi',                    'Faculty of Communication',                     'FACULTY',           'CEBECI'),
    ('EBF',    'Eğitim Bilimleri Fakültesi',            'Faculty of Educational Sciences',              'FACULTY',           'CEBECI'),
    ('SAG',    'Sağlık Bilimleri Fakültesi',            'Faculty of Health Sciences',                   'FACULTY',           'CEBECI'),
    ('ILH',    'İlahiyat Fakültesi',                    'Faculty of Divinity',                          'FACULTY',           'CEBECI'),
    ('ZRT',    'Ziraat Fakültesi',                      'Faculty of Agriculture',                       'FACULTY',           'DISKAPI'),
    ('VET',    'Veteriner Fakültesi',                   'Faculty of Veterinary Medicine',               'FACULTY',           'DISKAPI'),
    ('DIS',    'Diş Hekimliği Fakültesi',               'Faculty of Dentistry',                         'FACULTY',           'TANDOGAN'),
    ('ECZ',    'Eczacılık Fakültesi',                   'Faculty of Pharmacy',                          'FACULTY',           'TANDOGAN'),
    ('SPOR',   'Spor Bilimleri Fakültesi',              'Faculty of Sport Sciences',                    'FACULTY',           'GOLBASI'),
    ('UBF',    'Uygulamalı Bilimler Fakültesi',         'Faculty of Applied Sciences',                  'FACULTY',           'GOLBASI'),
    ('AUZEF',  'Açık ve Uzaktan Eğitim Fakültesi',      'Faculty of Open and Distance Education',       'FACULTY',           'GOLBASI'),
    ('YDYO',   'Yabancı Diller Yüksekokulu',            'School of Foreign Languages',                  'SCHOOL',            'CEBECI'),
    ('DK',     'Devlet Konservatuvarı',                 'State Conservatory',                           'CONSERVATORY',      'CEBECI'),
    ('SHMYO',  'Sağlık Hizmetleri Meslek Yüksekokulu',  'Vocational School of Health Services',         'VOCATIONAL_SCHOOL', 'CEBECI'),
    ('BEYMYO', 'Beypazarı Meslek Yüksekokulu',          'Beypazarı Vocational School',                  'VOCATIONAL_SCHOOL', NULL),
    ('KALMYO', 'Kalecik Meslek Yüksekokulu',            'Kalecik Vocational School',                    'VOCATIONAL_SCHOOL', NULL),
    ('FBE',    'Fen Bilimleri Enstitüsü',               'Graduate School of Natural and Applied Sciences', 'INSTITUTE',      'TANDOGAN'),
    ('SOBE',   'Sosyal Bilimler Enstitüsü',             'Graduate School of Social Sciences',           'INSTITUTE',         'CEBECI'),
    ('SABE',   'Sağlık Bilimleri Enstitüsü',            'Graduate School of Health Sciences',           'INSTITUTE',         'SIHHIYE')
) AS v(code, name_tr, name_en, unit_type, campus_code)
LEFT JOIN org.campuses c ON c.code = v.campus_code
ON CONFLICT (code) DO NOTHING;

INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
SELECT f.id, v.code, v.name_tr, v.name_en
FROM (VALUES
    ('MUH', 'BIL',  'Bilgisayar Mühendisliği',           'Computer Engineering'),
    ('MUH', 'EEM',  'Elektrik-Elektronik Mühendisliği',  'Electrical and Electronics Engineering'),
    ('MUH', 'BME',  'Biyomedikal Mühendisliği',          'Biomedical Engineering'),
    ('MUH', 'YZV',  'Yapay Zekâ ve Veri Mühendisliği',   'Artificial Intelligence and Data Engineering'),
    ('MUH', 'KIM',  'Kimya Mühendisliği',                'Chemical Engineering'),
    ('MUH', 'GIDA', 'Gıda Mühendisliği',                 'Food Engineering'),
    ('MUH', 'JEO',  'Jeoloji Mühendisliği',              'Geological Engineering'),
    ('FEN', 'MAT',  'Matematik',                         'Mathematics'),
    ('FEN', 'FIZ',  'Fizik',                             'Physics'),
    ('FEN', 'IST',  'İstatistik',                        'Statistics'),
    ('SBF', 'ULI',  'Uluslararası İlişkiler',            'International Relations'),
    ('SBF', 'IKT',  'İktisat',                           'Economics')
) AS v(faculty_code, code, name_tr, name_en)
JOIN org.faculties f ON f.code = v.faculty_code
ON CONFLICT (code) DO NOTHING;

INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language,
                          education_type, duration_semesters, max_duration_years,
                          total_ects_required, has_prep_class)
SELECT d.id, v.code, v.name_tr, v.name_en, v.degree_level, v.language,
       v.education_type, v.semesters, v.max_years, v.ects, v.prep
FROM (VALUES
    ('BIL', 'BIL-EN-NO',  'Bilgisayar Mühendisliği (İngilizce)',         'Computer Engineering',                         'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240.0, true),
    ('BIL', 'BIL-YL',     'Bilgisayar Mühendisliği Yüksek Lisans',       'Computer Engineering (MSc)',                   'MASTER',   'TR', 'DAYTIME', 4, 3, 120.0, false),
    ('EEM', 'EEM-EN-NO',  'Elektrik-Elektronik Mühendisliği (İngilizce)', 'Electrical and Electronics Engineering',      'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240.0, true),
    ('BME', 'BME-EN-NO',  'Biyomedikal Mühendisliği (İngilizce)',        'Biomedical Engineering',                       'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240.0, true),
    ('YZV', 'YZV-NO',     'Yapay Zekâ ve Veri Mühendisliği',             'Artificial Intelligence and Data Engineering', 'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240.0, false),
    ('KIM', 'KIM-NO',     'Kimya Mühendisliği',                          'Chemical Engineering',                         'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240.0, false),
    ('MAT', 'MAT-NO',     'Matematik',                                   'Mathematics',                                  'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240.0, false),
    ('IST', 'IST-NO',     'İstatistik',                                  'Statistics',                                   'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240.0, false),
    ('IKT', 'IKT-NO',     'İktisat',                                     'Economics',                                    'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240.0, false),
    ('IKT', 'IKT-IO',     'İktisat (İkinci Öğretim)',                    'Economics (Evening)',                          'BACHELOR', 'TR', 'EVENING', 8, 7, 240.0, false)
) AS v(department_code, code, name_tr, name_en, degree_level, language, education_type, semesters, max_years, ects, prep)
JOIN org.departments d ON d.code = v.department_code
ON CONFLICT (code) DO NOTHING;

COMMIT;
