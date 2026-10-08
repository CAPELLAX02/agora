-- Geliştirme seed'i: binalar ve derslikler. Gerçek bina adlarından esinlenilmiştir ama
-- kodlar ve kapasiteler kurgusaldır. Tekrar çalıştırılabilir.

INSERT INTO org.buildings (campus_id, faculty_id, code, name)
SELECT c.id, f.id, v.code, v.name
FROM (VALUES
    ('GOLBASI',  'MUH',  'MUH-A',   'Mühendislik Fakültesi A Blok'),
    ('GOLBASI',  'MUH',  'MUH-B',   'Mühendislik Fakültesi B Blok'),
    ('GOLBASI',  'MUH',  'MUH-LAB', 'Mühendislik Laboratuvarları'),
    ('GOLBASI',  NULL,   'GOL-AMF', '50. Yıl Ortak Amfiler'),
    ('TANDOGAN', 'FEN',  'FEN-A',   'Fen Fakültesi A Blok'),
    ('TANDOGAN', 'FEN',  'FEN-B',   'Fen Fakültesi B Blok'),
    ('CEBECI',   'SBF',  'SBF-ANA', 'Siyasal Bilgiler Fakültesi Ana Bina'),
    ('CEBECI',   'HUK',  'HUK-ANA', 'Hukuk Fakültesi Ana Bina'),
    ('SIHHIYE',  'DTCF', 'DTCF-A',  'Dil ve Tarih-Coğrafya Fakültesi A Blok')
) AS v(campus_code, faculty_code, code, name)
JOIN org.campuses c ON c.code = v.campus_code
LEFT JOIN org.faculties f ON f.code = v.faculty_code
ON CONFLICT (campus_id, code) DO NOTHING;

-- Her binaya türüne göre derslik dizisi üretilir.
INSERT INTO org.classrooms (building_id, code, name, capacity, exam_capacity, room_type, features)
SELECT b.id, r.code, r.name, r.capacity, r.capacity / 2, r.room_type, r.features
FROM org.buildings b
CROSS JOIN LATERAL (
    -- Sınıflar: 101-110, kapasite 30-80
    SELECT format('%s%s', left(split_part(b.code, '-', 2), 1), 100 + n) AS code,
           format('Derslik %s', 100 + n) AS name,
           30 + (n % 6) * 10 AS capacity,
           'LECTURE' AS room_type,
           CASE WHEN n % 3 = 0 THEN ARRAY['PROJECTOR', 'SMART_BOARD'] ELSE ARRAY['PROJECTOR'] END AS features
    FROM generate_series(1, 10) n
    WHERE b.code NOT IN ('MUH-LAB', 'GOL-AMF')
    UNION ALL
    -- Laboratuvarlar
    SELECT format('LAB-%s', n), format('Bilgisayar Laboratuvarı %s', n), 30, 'LAB',
           ARRAY['COMPUTERS', 'PROJECTOR', 'AIR_CONDITIONING']
    FROM generate_series(1, 4) n
    WHERE b.code = 'MUH-LAB'
    UNION ALL
    -- Amfiler
    SELECT format('AMFI-%s', n), format('Amfi %s', n), 150 + n * 50, 'AMPHI',
           ARRAY['PROJECTOR', 'SOUND_SYSTEM', 'RECORDING', 'ACCESSIBLE']
    FROM generate_series(1, 3) n
    WHERE b.code = 'GOL-AMF'
) r
ON CONFLICT (building_id, code) DO NOTHING;
