-- Geliştirme seed'i: bütün birimler için sentetik bölüm ve program kataloğu. Bölüm adları
-- gerçek üniversite yapısından esinlenilmiştir, kodlar kurgusaldır. 001_org.sql'deki
-- kayıtlara dokunmaz. Tekrar çalıştırılabilir.

INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
SELECT f.id, v.code, v.name_tr, v.name_en
FROM (VALUES
    ('MUH',    'FIZM',  'Fizik Mühendisliği',                     'Engineering Physics'),
    ('FEN',    'KIMF',  'Kimya',                                  'Chemistry'),
    ('FEN',    'BIYO',  'Biyoloji',                               'Biology'),
    ('FEN',    'AST',   'Astronomi ve Uzay Bilimleri',            'Astronomy and Space Sciences'),
    ('DTCF',   'TAR',   'Tarih',                                  'History'),
    ('DTCF',   'COG',   'Coğrafya',                               'Geography'),
    ('DTCF',   'TDE',   'Türk Dili ve Edebiyatı',                 'Turkish Language and Literature'),
    ('DTCF',   'IDE',   'İngiliz Dili ve Edebiyatı',              'English Language and Literature'),
    ('DTCF',   'FEL',   'Felsefe',                                'Philosophy'),
    ('DTCF',   'PSI',   'Psikoloji',                              'Psychology'),
    ('DTCF',   'SOS',   'Sosyoloji',                              'Sociology'),
    ('DTCF',   'ARK',   'Arkeoloji',                              'Archaeology'),
    ('TIP',    'TIPF',  'Tıp',                                    'Medicine'),
    ('SBF',    'SBKY',  'Siyaset Bilimi ve Kamu Yönetimi',        'Political Science and Public Administration'),
    ('SBF',    'ISL',   'İşletme',                                'Business Administration'),
    ('SBF',    'MAL',   'Maliye',                                 'Public Finance'),
    ('SBF',    'CEK',   'Çalışma Ekonomisi ve Endüstri İlişkileri', 'Labour Economics and Industrial Relations'),
    ('HUK',    'HUKF',  'Hukuk',                                  'Law'),
    ('ILT',    'GAZ',   'Gazetecilik',                            'Journalism'),
    ('ILT',    'HIT',   'Halkla İlişkiler ve Tanıtım',            'Public Relations and Publicity'),
    ('ILT',    'RTS',   'Radyo, Televizyon ve Sinema',            'Radio, Television and Cinema'),
    ('EBF',    'RPD',   'Rehberlik ve Psikolojik Danışmanlık',    'Guidance and Psychological Counseling'),
    ('EBF',    'OOE',   'Okul Öncesi Eğitimi',                    'Early Childhood Education'),
    ('EBF',    'OZE',   'Özel Eğitim',                            'Special Education'),
    ('SAG',    'HEM',   'Hemşirelik',                             'Nursing'),
    ('SAG',    'EBE',   'Ebelik',                                 'Midwifery'),
    ('SAG',    'BES',   'Beslenme ve Diyetetik',                  'Nutrition and Dietetics'),
    ('SAG',    'FTR',   'Fizyoterapi ve Rehabilitasyon',          'Physiotherapy and Rehabilitation'),
    ('ILH',    'ILHF',  'İlahiyat',                               'Theology'),
    ('ZRT',    'TAR2',  'Tarla Bitkileri',                        'Field Crops'),
    ('ZRT',    'BAH',   'Bahçe Bitkileri',                        'Horticulture'),
    ('ZRT',    'ZOO',   'Zootekni',                               'Animal Science'),
    ('ZRT',    'PEY',   'Peyzaj Mimarlığı',                       'Landscape Architecture'),
    ('VET',    'VETF',  'Veteriner Hekimliği',                    'Veterinary Medicine'),
    ('DIS',    'DISF',  'Diş Hekimliği',                          'Dentistry'),
    ('ECZ',    'ECZF',  'Eczacılık',                              'Pharmacy'),
    ('SPOR',   'ANT',   'Antrenörlük Eğitimi',                    'Coaching Education'),
    ('SPOR',   'SYO',   'Spor Yöneticiliği',                      'Sports Management'),
    ('UBF',    'BAN',   'Bankacılık ve Finans',                   'Banking and Finance'),
    ('UBF',    'SIG',   'Sigortacılık ve Risk Yönetimi',          'Insurance and Risk Management'),
    ('AUZEF',  'AUZ',   'Uzaktan Eğitim Programları',             'Distance Education Programmes'),
    ('YDYO',   'MTB',   'Mütercim ve Tercümanlık',                'Translation and Interpreting'),
    ('DK',     'MUZ',   'Müzik',                                  'Music'),
    ('DK',     'SAH',   'Sahne Sanatları',                        'Performing Arts'),
    ('SHMYO',  'TLAB',  'Tıbbi Laboratuvar Teknikleri',           'Medical Laboratory Techniques'),
    ('SHMYO',  'ILK',   'İlk ve Acil Yardım',                     'First and Emergency Aid'),
    ('BEYMYO', 'BILP',  'Bilgisayar Programcılığı',               'Computer Programming'),
    ('KALMYO', 'BAGC',  'Bağcılık',                               'Viticulture')
) AS v(faculty_code, code, name_tr, name_en)
JOIN org.faculties f ON f.code = v.faculty_code
ON CONFLICT (code) DO NOTHING;

-- Her bölüme bir normal öğretim programı. Birkaç bölüme ikinci öğretim. Meslek
-- yüksekokulu bölümleri ön lisans (4 dönem), tıp 12, diş/eczacılık/veteriner 10 dönem.
INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language,
                          education_type, duration_semesters, max_duration_years, total_ects_required, has_prep_class)
SELECT d.id,
       d.code || CASE WHEN v.evening THEN '-IO' ELSE '-NO' END,
       d.name_tr || CASE WHEN v.evening THEN ' (İkinci Öğretim)' ELSE '' END,
       d.name_en || CASE WHEN v.evening THEN ' (Evening)' ELSE '' END,
       CASE WHEN f.unit_type = 'VOCATIONAL_SCHOOL' THEN 'ASSOCIATE' ELSE 'BACHELOR' END,
       CASE WHEN d.code IN ('IDE', 'MTB') THEN 'EN' ELSE 'TR' END,
       CASE WHEN v.evening THEN 'EVENING' WHEN d.code = 'AUZ' THEN 'DISTANCE' ELSE 'DAYTIME' END,
       CASE WHEN f.unit_type = 'VOCATIONAL_SCHOOL' THEN 4
            WHEN d.code = 'TIPF' THEN 12
            WHEN d.code IN ('DISF', 'ECZF', 'VETF') THEN 10
            ELSE 8 END,
       CASE WHEN f.unit_type = 'VOCATIONAL_SCHOOL' THEN 4
            WHEN d.code = 'TIPF' THEN 9
            WHEN d.code IN ('DISF', 'ECZF', 'VETF') THEN 8
            ELSE 7 END,
       CASE WHEN f.unit_type = 'VOCATIONAL_SCHOOL' THEN 120
            WHEN d.code = 'TIPF' THEN 360
            WHEN d.code IN ('DISF', 'ECZF', 'VETF') THEN 300
            ELSE 240 END,
       d.code IN ('IDE', 'MTB')
FROM org.departments d
JOIN org.faculties f ON f.id = d.faculty_id
CROSS JOIN (VALUES (false), (true)) AS v(evening)
WHERE NOT EXISTS (SELECT 1 FROM org.programs p WHERE p.department_id = d.id)
  AND (NOT v.evening OR d.code IN ('SBKY', 'ISL', 'MAL', 'TAR', 'COG', 'BILP'))
ON CONFLICT (code) DO NOTHING;
