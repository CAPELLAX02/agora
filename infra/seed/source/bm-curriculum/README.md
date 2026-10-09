# Bilgisayar Mühendisliği (İngilizce) ders planları

`infra/seed/dev/005_bm_curriculum.sql` bu klasördeki veriden üretilir. Kaynak, bölümün
web sitesinde yayımlanan ders planı formlarıdır (comp.eng.ankara.edu.tr → Müfredat):
2022, 2023 ve 2026 sürümleri. PDF'ler depoya konmaz; ayrıştırılmış veri `curricula.json`'dadır.

## Hat

1. **PDF → metin** (macOS, PDFKit): `swift pdftext.swift bm-en-2026.pdf > bm-en-2026.txt`
2. **Metin → JSON**: `python3 -I parse_curriculum.py bm-en-2022.txt bm-en-2023.txt bm-en-2026.txt > curricula.json`
   Her yarıyılın AKTS toplamı formdaki "Toplam" satırıyla karşılaştırılır; tutmazsa hata verir.
   Sütunları karışan birkaç sayfa `OVERRIDES` ile formdaki değerlere göre düzeltilmiştir.
3. **JSON → SQL** (depo kökünden):
   `python3 -I infra/seed/source/bm-curriculum/gen_seed.py infra/seed/source/bm-curriculum/curricula.json infra/seed/dev/005_bm_curriculum.sql`

## Üreticinin kararları

- Ders adı için büyük harfle yazılmamış en yeni sürüm seçilir; yoksa Türkçe büyük/küçük harf
  kurallarıyla düzeltilir. Başka bölümlerin birkaç dersinde eski formlar Türkçe ad yerine
  İngilizce ad verdiği için Türkçe adlar elle eklenmiştir (`TR_OVERRIDE`).
- Seçmeli yuvadaki ders sayısı, yuva AKTS'sinin havuzdaki bir dersin AKTS'sine bölümüdür
  (ör. 2026 planında 4. sınıf teknik seçmeli: 16 / 4 = 4 ders).
- Eşdeğerlikler 2026 formunun intibak sütunundan ve 2026 planında aynı adla yeni koda geçen
  derslerden çıkarılır. Formda İş Sağlığı ve Güvenliği II'nin eski kodu yanlışlıkla COM3073
  yazılmıştır; doğrusu COM3072'dir.
- 2024 formu okunamadı (metin katmanı bozuk); 2023-2025 girişlileri 2023 sürümünü izler.
- Formlarda listelenmeyen üniversite alan dışı seçmeli havuzu için sentetik dersler (`UNVG...`)
  eklenir ve açıklamalarında sentetik oldukları yazar.
