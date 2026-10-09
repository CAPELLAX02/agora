"""BM (İngilizce) ders planlarından (curricula.json) SQL seed üretir.

Kullanım (depo kökünden):
    python3 -I infra/seed/source/bm-curriculum/gen_seed.py \
        infra/seed/source/bm-curriculum/curricula.json infra/seed/dev/005_bm_curriculum.sql
"""
import json
import sys

src, out_path = sys.argv[1], sys.argv[2]
versions = json.load(open(src, encoding='utf-8'))

GROUP_CODES = {'UNVSEC', 'UNVGOFECG', 'GENELSOSSEC'}

# --- Ad normalizasyonu ----------------------------------------------------------

ROMAN = {'I', 'II', 'III', 'IV', 'V'}
TR_SMALL = {'ve', 'ile', 'için', 'de', 'da'}
EN_SMALL = {'and', 'or', 'of', 'the', 'to', 'in', 'for', 'with', 'on', 'a', 'an'}
KEEP = {'RFİD': 'RFID', 'RFID': 'RFID', 'CISCO': 'CISCO', 'IOS': 'iOS', 'AI-ASSISTED': 'AI-Assisted'}


def tr_lower(s):
    return s.replace('I', 'ı').replace('İ', 'i').lower()


def tr_upper_first(w):
    if not w:
        return w
    f = w[0]
    f = {'i': 'İ', 'ı': 'I'}.get(f, f.upper())
    return f + w[1:]


def is_upper(s):
    letters = [c for c in s if c.isalpha()]
    return letters and all(c.isupper() for c in letters)


def title_tr(s):
    words = []
    for i, w in enumerate(s.split()):
        if w in KEEP:
            words.append(KEEP[w])
        elif w in ROMAN:
            words.append(w)
        else:
            low = tr_lower(w)
            words.append(low if (i > 0 and low in TR_SMALL) else tr_upper_first(low))
    return ' '.join(words)


def title_en(s):
    words = []
    for i, w in enumerate(s.split()):
        if w in KEEP:
            words.append(KEEP[w])
        elif w in ROMAN:
            words.append(w)
        else:
            low = w.lower()
            if i > 0 and low in EN_SMALL:
                words.append(low)
            else:
                words.append('-'.join(p[:1].upper() + p[1:] for p in low.split('-')))
    return ' '.join(words)


def pick_name(rows, field, titler):
    """Büyük harfle yazılmamış bir sürüm varsa onu, yoksa düzeltilmiş halini seçer.
    En yeni sürüm önceliklidir."""
    for _, r in sorted(rows.items(), reverse=True):
        if not is_upper(r[field]):
            return r[field].replace('’', "'").strip()
    latest = rows[max(rows)][field]
    return titler(latest.replace('’', "'").strip())


# Başka bölümlerin derslerinde eski formlarda Türkçe ad yerine İngilizce ad yazılmış.
TR_OVERRIDE = {
    'BME3314': 'Mikroişlemciler',
    'EEE317': 'Modelleme ve Benzetim',
    'EEE328': 'Sayısal İşaret İşleme',
    'ENE355': 'Ölçme ve Enstrümantasyon',
    'ENE356': 'Nanobilim ve Nanoteknoloji',
    'CEN338': 'Dinamik Benzetime Giriş',
}

# --- Dersler ---------------------------------------------------------------------

courses = {}   # kod → {yıl: satır}
in_version = {}
for v in versions:
    y = v['year']
    for sem, sv in v['semesters'].items():
        for r in sv['rows']:
            if r['code'] in v['groups'] or r['code'] in GROUP_CODES:
                continue
            courses.setdefault(r['code'], {})[y] = r
            in_version.setdefault(r['code'], set()).add(y)
    for g, gv in v['groups'].items():
        for c, r in gv['courses'].items():
            courses.setdefault(c, {})[y] = r
            in_version.setdefault(c, set()).add(y)

OWNER = {'COM': 'BIL', 'MTH': 'MAT', 'PHY': 'FIZ', 'STA': 'IST', 'EEE': 'EEM', 'BME': 'BME'}
TR_LANG = {'TUR', 'HIS', 'OUL', 'ART', 'PFE', 'UNVG'}


def prefix(code):
    return ''.join(ch for ch in code if ch.isalpha())


KIND = {
    'OUL101': ('NON_CREDIT', 'PASS_FAIL'),
    'SCS301': ('ACTIVITY', 'PASS_FAIL'),
    'COM4097': ('INTERNSHIP', 'PASS_FAIL'),
    'COM4099': ('INTERNSHIP', 'PASS_FAIL'),
    'COM4061': ('PROJECT', 'LETTER'),
    'COM4062': ('PROJECT', 'LETTER'),
    'COM4063': ('PROJECT', 'LETTER'),
    'COM4064': ('PROJECT', 'LETTER'),
}
DESC = {
    'COM4097': ('Staj, 4. yarıyıl sonundan itibaren yapılabilir.', 'Summer practice can be done after the end of the 4th semester.'),
    'COM4099': ('Staj, 4. yarıyıl sonundan itibaren yapılabilir.', 'Summer practice can be done after the end of the 4th semester.'),
}

SYNTHETIC_NOTE = ('Sentetik örnek ders: üniversite alan dışı seçmeli havuzu resmî ders planında listelenmediği için seed verisinde örneklenmiştir.',
                  'Synthetic sample course: the university out-of-field elective pool is not listed in the official plan, so it is sampled in seed data.')
SYNTHETIC = [
    ('UNVG101', 'Girişimcilik ve İnovasyon', 'Entrepreneurship and Innovation'),
    ('UNVG103', 'Bilim Tarihi ve Felsefesi', 'History and Philosophy of Science'),
    ('UNVG105', 'Sanat ve Toplum', 'Art and Society'),
    ('UNVG107', 'Sürdürülebilir Kalkınma', 'Sustainable Development'),
    ('UNVG109', 'Hukukun Temel Kavramları', 'Fundamentals of Law'),
    ('UNVG111', 'Etkili İletişim', 'Effective Communication'),
]

course_rows = []
canon = {}
for code in sorted(courses):
    rows = courses[code]
    latest = rows[max(rows)]
    vals = {(r['theory'], r['practice'], r['credit'], r['ects']) for r in rows.values()}
    assert len(vals) == 1, (code, vals)
    name_tr = TR_OVERRIDE.get(code) or pick_name(rows, 'name_tr', title_tr)
    name_en = pick_name(rows, 'name_en', title_en)
    if code == 'OUL102':  # eski formlarda OUL101'in adı tekrarlanmış; 2026 formu doğru adı verir
        name_tr, name_en = 'Üniversite Yaşamına Uyum ve Yaşam Becerileri', title_en(rows[2026]['name_en'])
    kind, mode = KIND.get(code, ('REGULAR', 'LETTER'))
    dtr, den = DESC.get(code, (None, None))
    lang = 'TR' if prefix(code) in TR_LANG else 'EN'
    canon[code] = (name_tr, name_en)
    course_rows.append((code, OWNER.get(prefix(code)), name_tr, name_en, int(latest['theory']), int(latest['practice']),
                        float(latest['credit']), float(latest['ects']), lang, kind, mode, dtr, den))
for code, tr, en in SYNTHETIC:
    canon[code] = (tr, en)
    course_rows.append((code, None, tr, en, 2, 0, 2.0, 3.0, 'TR', 'REGULAR', 'LETTER', SYNTHETIC_NOTE[0], SYNTHETIC_NOTE[1]))

# --- Gruplar ---------------------------------------------------------------------

GROUPS = {
    'COMTE02': ('2. Sınıf Teknik Seçmeli Dersler', '2nd Year Technical Electives', 'BIL', 'TECHNICAL'),
    'COMTE03': ('3. Sınıf Teknik Seçmeli Dersler', '3rd Year Technical Electives', 'BIL', 'TECHNICAL'),
    'COMTE04': ('4. Sınıf Teknik Seçmeli Dersler', '4th Year Technical Electives', 'BIL', 'TECHNICAL'),
    'UNVSEC': ('Alan Dışı (Üniversite) Seçmeli Dersler', 'University Out-of-Field Electives', None, 'UNIVERSITY_GENERAL'),
    'UNVGOFECG': ('Üniversite Alan Dışı Genel Seçmeli Ders Grubu', 'University General Out-of-Field Electives', None, 'UNIVERSITY_GENERAL'),
    'GENELSOSSEC': ('Genel Sosyal Seçmeli Dersler', 'General Social Electives', None, 'SOCIAL'),
}
for n in range(3, 8):
    GROUPS[f'PFESECG{n}YY'] = (f'Pedagojik Formasyon Eğitimi Grubu ({n}. yarıyıl)',
                               f'Pedagogical Formation Training Group (Semester {n})', None, 'PEDAGOGICAL')

members = set()
for v in versions:
    for g, gv in v['groups'].items():
        assert g in GROUPS, g
        for c in gv['courses']:
            members.add((g, c))
for g in ('UNVSEC', 'UNVGOFECG'):
    for code, _, _ in SYNTHETIC:
        members.add((g, code))

# Yuvadaki ders sayısı: yuva AKTS'si / havuzdaki bir dersin AKTS'si.
def per_course_ects(group, year):
    if group == 'COMTE04':
        return 4.0 if year >= 2026 else 6.0
    return {'COMTE02': 4.0, 'COMTE03': 4.0, 'UNVSEC': 3.0, 'UNVGOFECG': 3.0, 'GENELSOSSEC': 2.0}[group]

# --- Eşdeğerlikler ---------------------------------------------------------------

current = set()
v2026 = next(v for v in versions if v['year'] == 2026)
for sem, sv in v2026['semesters'].items():
    for r in sv['rows']:
        current.add(r['code'])
for g, gv in v2026['groups'].items():
    current.update(gv['courses'])

pairs = {}  # yeni → eski
NOTE_INTIBAK = '2026 ders planı intibak tablosu'
NOTE_NAME = 'Aynı içerikli dersin yeni kodu (2026 ders planı)'
for sem, sv in v2026['semesters'].items():
    for r in sv['rows']:
        if r.get('intibak'):
            old = r['intibak']
            if r['code'] == 'COM3074':
                old = 'COM3072'  # formda İSG II'nin eski kodu yanlışlıkla COM3073 yazılmış
            pairs[(r['code'], old)] = NOTE_INTIBAK
by_name = {}
for code in current:
    if code in canon:
        by_name.setdefault(canon[code][1].lower(), []).append(code)
for code, (tr, en) in canon.items():
    if code in current or code.startswith('UNVG'):
        continue
    for new in by_name.get(en.lower(), []):
        if new != code and (new, code) not in pairs and not new.startswith('OUL'):
            pairs[(new, code)] = NOTE_NAME

PREREQS = [('COM2044', 'COM1002', 'PASSED', 1), ('COM2067', 'COM1002', 'ATTENDED', 1)]

# --- Müfredatlar -----------------------------------------------------------------

RANGES = {2022: (2018, 2022), 2023: (2023, 2025), 2026: (2026, None)}

curricula = []
for v in versions:
    y = v['year']
    items = []
    total = 0.0
    for sem in sorted(v['semesters'], key=int):
        sv = v['semesters'][sem]
        sem_total = 0.0
        for pos, r in enumerate(sv['rows']):
            sem_total += r['ects']
            if r['code'] in GROUPS:
                g = r['code']
                if g.startswith('PFESECG'):
                    count = len(v['groups'][g]['courses'])
                else:
                    per = per_course_ects(g, y)
                    count = r['ects'] / per
                    assert count == int(count), (y, sem, g, r['ects'], per)
                    count = int(count)
                items.append((int(sem), 'ELECTIVE_SLOT', g, int(r['theory']), int(r['practice']), float(r['credit']), float(r['ects']), count, pos))
            else:
                assert r['code'] in canon, r['code']
                items.append((int(sem), 'COURSE', r['code'], None, None, None, None, None, pos))
        assert sem_total == sv['total_ects'], (y, sem, sem_total)
        total += sem_total
    assert total == v['totals']['ects'] == 240, (y, total)
    curricula.append((y, RANGES[y], items))

# --- SQL -------------------------------------------------------------------------

def lit(v):
    if v is None:
        return 'NULL'
    if isinstance(v, bool):
        return 'true' if v else 'false'
    if isinstance(v, (int, float)):
        return repr(v)
    return "'" + str(v).replace("'", "''") + "'"


def values(rows, indent='    '):
    return ',\n'.join(indent + '(' + ', '.join(lit(x) for x in r) + ')' for r in rows)


out = []
out.append('''-- Geliştirme seed'i: Bilgisayar Mühendisliği (İngilizce) programının gerçek ders planları.
--
-- Kaynak: bölümün yayımladığı ders planı formları (2022, 2023 ve 2026 sürümleri). Bu dosya
-- formlardan ayrıştırılan veriden üretilmiştir; elle düzenlemeyin, üreticiyi yeniden
-- çalıştırın (infra/seed/source/bm-curriculum/README.md). 2024 formu okunamadığı için
-- 2023-2025 girişlileri 2023 sürümünü izler.
--
-- Dersler, saatleri, kredileri ve AKTS'leri formlardaki gibidir. Formlarda listelenmeyen
-- üniversite alan dışı seçmeli havuzu için birkaç sentetik ders (UNVG...) eklenmiştir.
-- Eşdeğerlikler 2026 formunun intibak sütunundan ve aynı adlı derslerin kod
-- değişikliklerinden çıkarılmıştır. Tekrar çalıştırılabilir.
''')

out.append('-- Dersler')
out.append('''INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, practice_hours,
                                national_credit, ects, language, course_kind, grading_mode, description_tr, description_en)
SELECT v.code, d.id, v.name_tr, v.name_en, v.theory, v.practice, v.credit, v.ects, v.lang, v.kind, v.mode, v.dtr, v.den
FROM (VALUES
''' + values(course_rows) + '''
) AS v(code, dept, name_tr, name_en, theory, practice, credit, ects, lang, kind, mode, dtr, den)
LEFT JOIN org.departments d ON d.code = v.dept
ON CONFLICT (code) DO NOTHING;
''')

out.append('-- Seçmeli ders grupları (havuzlar)')
out.append('''INSERT INTO curriculum.elective_groups (code, name_tr, name_en, owner_department_id, group_kind)
SELECT v.code, v.name_tr, v.name_en, d.id, v.kind
FROM (VALUES
''' + values([(g, tr, en, dept, kind) for g, (tr, en, dept, kind) in sorted(GROUPS.items())]) + '''
) AS v(code, name_tr, name_en, dept, kind)
LEFT JOIN org.departments d ON d.code = v.dept
ON CONFLICT (code) DO NOTHING;
''')

out.append('INSERT INTO curriculum.elective_group_courses (elective_group_id, course_id)')
out.append('''SELECT g.id, c.id
FROM (VALUES
''' + values(sorted(members)) + '''
) AS v(group_code, course_code)
JOIN curriculum.elective_groups g ON g.code = v.group_code
JOIN curriculum.courses c ON c.code = v.course_code
ON CONFLICT DO NOTHING;
''')

out.append('-- Ön koşullar')
out.append('''INSERT INTO curriculum.course_prerequisites (course_id, prerequisite_course_id, requirement, group_no)
SELECT c.id, p.id, v.requirement, v.group_no
FROM (VALUES
''' + values(PREREQS) + '''
) AS v(course_code, prerequisite_code, requirement, group_no)
JOIN curriculum.courses c ON c.code = v.course_code
JOIN curriculum.courses p ON p.code = v.prerequisite_code
ON CONFLICT ON CONSTRAINT course_prerequisites_pair_key DO NOTHING;
''')

out.append('-- Eşdeğerlikler: yeni kod ↔ eski kod')
out.append('''INSERT INTO curriculum.course_equivalences (course_id, equivalent_course_id, is_bidirectional, valid_from_year, note)
SELECT n.id, o.id, true, 2026, v.note
FROM (VALUES
''' + values(sorted((n, o, note) for (n, o), note in pairs.items())) + '''
) AS v(new_code, old_code, note)
JOIN curriculum.courses n ON n.code = v.new_code
JOIN curriculum.courses o ON o.code = v.old_code
WHERE NOT EXISTS (
    SELECT 1 FROM curriculum.course_equivalences e
    WHERE (e.course_id = n.id AND e.equivalent_course_id = o.id) OR (e.course_id = o.id AND e.equivalent_course_id = n.id)
);
''')

for y, (frm, to), items in curricula:
    out.append(f'-- {y} ders planı: {frm}' + (f'-{to}' if to else ' ve sonrası') + ' girişliler')
    out.append(f'''WITH cur AS (
    INSERT INTO curriculum.curricula (program_id, name_tr, name_en, effective_from_year, effective_to_year,
                                      total_ects_required, status, activated_at)
    SELECT p.id, {lit(f'{y} Ders Planı')}, {lit(f'{y} Curriculum')}, {frm}, {lit(to)}, 240, 'ACTIVE', now()
    FROM org.programs p
    WHERE p.code = 'BIL-EN-NO'
      AND NOT EXISTS (SELECT 1 FROM curriculum.curricula c WHERE c.program_id = p.id AND c.effective_from_year = {frm})
    RETURNING id
)
INSERT INTO curriculum.curriculum_items (curriculum_id, semester_no, item_type, course_id, elective_group_id,
                                         slot_theory_hours, slot_practice_hours, slot_national_credit, slot_ects,
                                         slot_course_count, is_compulsory, position)
SELECT cur.id, v.semester, v.item_type, c.id, g.id, v.theory, v.practice, v.credit, v.ects, v.course_count,
       v.item_type = 'COURSE', v.position
FROM cur
CROSS JOIN (VALUES
''' + values(items) + '''
) AS v(semester, item_type, code, theory, practice, credit, ects, course_count, position)
LEFT JOIN curriculum.courses c ON v.item_type = 'COURSE' AND c.code = v.code
LEFT JOIN curriculum.elective_groups g ON v.item_type = 'ELECTIVE_SLOT' AND g.code = v.code;
''')

open(out_path, 'w', encoding='utf-8').write('\n'.join(out))
print(f'{len(course_rows)} ders, {len(GROUPS)} grup, {len(members)} üyelik, {len(pairs)} eşdeğerlik, {len(curricula)} müfredat')
for (n, o), note in sorted(pairs.items()):
    print('  ', n, '<-', o, canon[n][1], '|', canon[o][1], '|', note[:10])
