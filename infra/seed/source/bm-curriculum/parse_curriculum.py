"""BM (İngilizce) müfredat PDF metinlerini yapılandırılmış JSON'a çevirir.

Her yarıyılın AKTS toplamı formdaki "Toplam" satırıyla karşılaştırılır: tutmazsa hata.
"""
import json, re, sys

CODE = r'(?:[A-Z]{2,4}\d{3,4}|COMTE0\d|UNVGOFECG|UNVSEC|GENELSOSSEC|PFESECG\d+YY)'
HEADER = re.compile(r'^(A|B/\d|C|D|E|ANKARA ÜNİVERSİTESİ|ÖĞRENCİ İŞLERİ DAİRE BAŞKANLIĞI|ANADAL ÖĞRETİM PROGRAMI FORMU|'
                    r'EĞİTİM-ÖĞRETİM YILI.*|FAKÜLTE/YÜKSEKOKUL ADI.*|Etkinlik Saati|Sıra Numarası|Sıra|No|'
                    r'DERS KODU.*|DERSİN ADI.*|Dersin ön koşulu.*|İntibak Dersi.*|Kuramsal|Uygulama ve Laboratuvar|Uygulama ve|'
                    r'Laboratuvar|TOPLAM SAAT.*|Ulusal kredi|AKTS Kredisi.*|AKTS|Bu seçmeli ders grubu için|bu yarıyıl tamamlanması|'
                    r'gereken asgari değer|KODU:)$')

# PDF metni bazı sayfalarda sütunları karıştırıyor. Bu satırlar formdaki değerlerle
# elle düzeltildi; her düzeltme sayfanın "Toplam" satırıyla tutarlıdır.
OVERRIDES = {
    (2022, 7): [
        {'code': 'COMTE04', 'name_en': 'Computer Engineering Technical Electives 4th Year',
         'name_tr': 'Bilgisayar Müh. Teknik Seçmeli 4. Yıl', 'theory': 6, 'practice': 0, 'credit': 6, 'ects': 12},
        {'code': 'UNVSEC', 'name_en': 'University Out-of-Field Electives', 'name_tr': 'Alan Dışı (Üniversite) Seçmeli Dersler',
         'theory': 4, 'practice': 0, 'credit': 4, 'ects': 6},
    ],
}

# Toplam satırı da karışan sayfaların formdaki yarıyıl AKTS toplamı.
TOTAL_OVERRIDES = {(2022, 5): 30, (2022, 7): 30}

def pages(text):
    parts = re.split(r'^=== SAYFA \d+ ===$', text, flags=re.M)
    return [p.strip().splitlines() for p in parts if p.strip()]

def semester_of(lines):
    for l in lines:
        m = re.search(r'(\d)\.\s*SINIF\s*/\s*(\d+)\.\s*YARIYIL', l)
        if m:
            return int(m.group(2))
    return None

def nums_tail(tokens):
    """Satır sonundaki T U Toplam Kredi AKTS beşlisini bulur (T+U=Toplam)."""
    vals = []
    for t in tokens:
        if t == '-':
            vals.append(0)
        elif re.fullmatch(r'\d+(?:[.,]\d+)?', t):
            vals.append(float(t.replace(',', '.')))
        else:
            vals.append(None)
    n = len(vals)
    for end in (n,):
        if end - 5 < 0:
            continue
        w = vals[end - 5:end]
        if None in w:
            continue
        t, u, tot, k, e = w
        if abs(t + u - tot) < 0.01:
            return (end - 5, [t, u, tot, k, e])
    return None

def split_name(s):
    s = re.sub(r'\s+', ' ', s).strip()
    m = re.match(r'^(.*?)\s*\((.*)\)\s*(\(.*\))?$', s)
    if not m:
        return s, s
    en, tr = m.group(1).strip(), m.group(2).strip()
    return en, tr

def parse_rows(body, with_markers):
    text = ' '.join(body)
    text = re.split(r'\bToplam\b', text)[0]
    # Bazı sayfalarda sıra numarası koddan sonra geliyor ("PFESECG4YY 8 Pedagojik ..."): yer değiştir.
    text = re.sub(r'(PFESECG\d+YY)\s+(\d{1,2})\s+(?=[A-ZÇĞİÖŞÜ][a-zçğıöşü])', r'\2 \1 ', text)
    rows = []
    # Sıra numaralı sayfalarda her satır "N KOD" ile başlar: bir sonraki satırın
    # numarası bu satırın sayılarına karışmasın diye satır başı numarayla birlikte bulunur.
    has_index = re.search(r'(?:^|\s)1\s+' + CODE + r'(?![0-9])', text) is not None
    pattern = (r'(?:(?<=\s)|^)(?:\d{1,2}\s+)(' if has_index else r'(?<![A-Z0-9])(') + CODE + r')(?![0-9])'
    starts = [m for m in re.finditer(pattern, text)]
    for i, m in enumerate(starts):
        chunk = text[m.end(): starts[i + 1].start() if i + 1 < len(starts) else len(text)]
        tokens = chunk.split()
        found = nums_tail(tokens)
        if not found:
            continue
        idx, (t, u, tot, k, e) = found
        name_tokens = tokens[:idx]
        prereq = intibak = None
        # İşaretler: "+ -", "- -" (ön koşul var mı, intibak mı) ya da intibak kodu "COM 1001"
        while name_tokens and name_tokens[-1] in ('+', '-'):
            name_tokens.pop()
        tail = ' '.join(name_tokens)
        mk = re.search(r'([+-])\s+([+-])\s*$', ' '.join(tokens[:idx]))
        if mk:
            prereq = mk.group(1) == '+'
        mi = re.search(r'\)\s*([A-Z]{3})\s+(\d{4})\s*$', tail)
        if mi:
            intibak = mi.group(1) + mi.group(2)
            tail = tail[:mi.start() + 1]
        en, tr = split_name(tail)
        rows.append({'code': m.group(1), 'name_en': en, 'name_tr': tr, 'theory': t, 'practice': u,
                     'credit': k, 'ects': e, 'prereq_flag': prereq, 'intibak': intibak})
    return rows

def parse(path):
    text = open(path, encoding='utf-8').read()
    year = int(re.search(r'EĞİTİM-ÖĞRETİM YILI : (\d{4})', text).group(1))
    out = {'year': year, 'semesters': {}, 'groups': {}, 'totals': None}
    for lines in pages(text):
        sem = semester_of(lines)
        joined = '\n'.join(lines)
        body = [l for l in lines if not HEADER.match(l.strip()) and not l.startswith('PROGRAM ADI') and 'ÖĞRETİM PROGRAMI' not in l]
        if 'MEZUNİYET İÇİN TAMAMLANMASI' in joined:
            m = re.search(r'TOPLAM TAMAMLANMASI GEREKEN (\d+)', joined)
            out['totals'] = {'credit': int(m.group(1))}
            continue
        if 'STAJ LİSTESİ' in joined or sem is None:
            continue
        if 'ZORUNLU DERSLERİ' in joined:
            rows = parse_rows(body, True)
            for fix in OVERRIDES.get((year, sem), []):
                rows = [r for r in rows if r['code'] != fix['code']]
                rows.append({**fix, 'prereq_flag': None, 'intibak': None, 'override': True})
            total = re.search(r'Toplam\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)', joined)
            expected = TOTAL_OVERRIDES.get((year, sem), int(total.group(5)) if total else None)
            got = sum(r['ects'] for r in rows)
            if expected is not None and abs(got - expected) > 0.01:
                sys.exit(f'{path} yarıyıl {sem}: AKTS toplamı {got} ≠ {expected}\n' + json.dumps(rows, ensure_ascii=False, indent=1))
            out['semesters'][sem] = {'rows': rows, 'total_ects': expected}
            continue
        # Seçmeli grup sayfası
        gm = re.search(r'\b(COMTE0\d|UNVGOFECG|UNVSEC|GENELSOSSEC|PFESECG\d+YY)\b', joined)
        if not gm:
            continue
        code = gm.group(1)
        name_m = re.search(r'GRUP ADI:\s*\n(.*?)\n', joined)
        rows = [r for r in parse_rows(body, False) if r['code'] != code]
        g = out['groups'].setdefault(code, {'name': name_m.group(1).strip() if name_m else code, 'courses': {}})
        for r in rows:
            g['courses'][r['code']] = r
    out['totals']['ects'] = sum(s['total_ects'] for s in out['semesters'].values())
    return out

if __name__ == '__main__':
    res = [parse(p) for p in sys.argv[1:]]
    json.dump(res, sys.stdout, ensure_ascii=False, indent=1)
