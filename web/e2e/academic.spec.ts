import { expect, test, type Page } from '@playwright/test'

import { login, nav } from './support/app'

// Faz 2 seed'i: 2026-2027 takvimi, Bilgisayar Mühendisliği'nin gerçek ders planları ve
// 2026 güz ders açmaları (backend/cmd/seed, infra/seed/dev). Senaryolar tarihten bağımsızdır:
// pencerenin açık olup olmadığına değil, hangi kapsamın uygulandığına bakılır.
test.describe.configure({ mode: 'serial' })

/** choose, bir Radix seçicisinde verilen seçeneği seçer. */
async function choose(page: Page, label: string, option: string | RegExp, scope = page.locator('body')) {
  await scope.getByRole('combobox', { name: label }).click()
  await page.getByRole('option', { name: option }).click()
}

test('takvimde birimin kendi penceresi üniversitenin penceresinin yerine geçer', async ({ page }) => {
  await login(page, '22290002')
  await nav(page, 'Akademik takvim').click()

  const addDrop = page.getByRole('listitem', { name: 'Ekle-bırak (öğrenci)' })
  await expect(addDrop).toBeVisible()
  await expect(addDrop.getByText('Birim takvimi')).toHaveCount(0)

  await choose(page, 'Birim', 'Mühendislik Fakültesi')
  await expect(addDrop.getByText('Birim takvimi')).toBeVisible()
  await expect(page.getByRole('cell', { name: /Ekle-bırak \(Mühendislik, uzatma\)/ })).toBeVisible()
})

test('öğrenci giriş yılına göre izlediği ders planını görür', async ({ page }) => {
  await login(page, '22290002')
  await nav(page, 'Ders planım').click()

  await expect(page.getByRole('heading', { name: /2022 Ders Planı/ })).toBeVisible()
  await expect(page.getByText(/Giriş yılınız 2022/)).toBeVisible()
  await expect(page.getByLabel('Mezuniyet özeti')).toContainText('240 / 240')

  const seventh = page.getByRole('region', { name: '7. yarıyıl' })
  await expect(seventh.getByRole('link', { name: 'Araştırma Teknikleri I' })).toBeVisible()
  // 2022 planında 4. sınıf teknik seçmeli yuvası 2 dersten (12 AKTS) oluşur.
  await expect(seventh.getByRole('row', { name: /COMTE04/ }).getByText('2 ders')).toBeVisible()
  await seventh.getByRole('button', { name: /4. Sınıf Teknik Seçmeli Dersler/ }).click()
  await expect(page.getByRole('link', { name: /COM4519 Veri Madenciliği/ })).toBeVisible()
})

test('bölüm başkanı ders açar; derslik ve öğretim elemanı çakışmaları reddedilir', async ({ page }) => {
  await login(page, 'P10002')
  await nav(page, 'Ders açma').click()

  // Bölüm başkanının bölümü varsayılan seçilidir.
  await expect(page.getByRole('combobox', { name: 'Bölüm' })).toHaveText('Bilgisayar Mühendisliği')
  await page.getByRole('link', { name: 'Ayrık Yapılar' }).click()

  // COM2039'un öğretim elemanının pazartesi sabah başka bir dersi (COM2553) var.
  const section = page.getByRole('region', { name: 'Şube 1' })
  await section.getByRole('button', { name: 'Oturum ekle' }).click()
  let dialog = page.getByRole('dialog', { name: 'Oturum ekle' })
  await dialog.getByRole('button', { name: 'Oturum ekle' }).click()
  await expect(
    dialog.getByText(/Doç. Dr. Selin Koç bu saatte başka bir derste: COM2553-1 \(Pazartesi 09:00-11:50\)/),
  ).toBeVisible()
  await dialog.getByRole('button', { name: 'Vazgeç' }).click()

  // Açılmamış bir ders açılır ve programı kurulur.
  await page.getByRole('link', { name: 'Ders açma' }).first().click()
  await page.getByRole('button', { name: 'Ders aç' }).click()
  dialog = page.getByRole('dialog', { name: 'Ders aç' })
  await dialog.getByRole('searchbox').fill('COM2075')
  await dialog.getByRole('button', { name: /COM2075/ }).click()
  await dialog.getByRole('button', { name: 'Dersi aç' }).click()
  await expect(page.getByRole('heading', { name: /COM2075/ })).toBeVisible()
  await expect(page.getByText('Planlanıyor')).toBeVisible()

  await page.getByRole('button', { name: 'Şube ekle' }).click()
  dialog = page.getByRole('dialog', { name: 'Şube ekle' })
  await dialog.getByLabel('Kontenjan', { exact: true }).fill('40')
  await dialog.getByRole('button', { name: 'Kaydet' }).click()
  const created = page.getByRole('region', { name: 'Şube 1' })
  await expect(created).toBeVisible()

  // Salı sabahı A104 3. sınıfın İşletim Sistemleri dersine verilmiş.
  await created.getByRole('button', { name: 'Oturum ekle' }).click()
  dialog = page.getByRole('dialog', { name: 'Oturum ekle' })
  await choose(page, 'Gün', 'Salı', dialog)
  await choose(page, 'Bina', /MUH-A/, dialog)
  await choose(page, 'Derslik', /A104/, dialog)
  await dialog.getByRole('button', { name: 'Oturum ekle' }).click()
  await expect(dialog.getByText('Derslik bu saatte dolu: COM3035-1 (Salı 09:00-11:50).')).toBeVisible()

  // Boş bir saat ve derslik kabul edilir.
  await choose(page, 'Gün', 'Çarşamba', dialog)
  await dialog.getByLabel('Başlangıç').fill('15:00')
  await dialog.getByLabel('Bitiş').fill('16:50')
  await choose(page, 'Derslik', /A110/, dialog)
  await dialog.getByRole('button', { name: 'Oturum ekle' }).click()
  await expect(dialog).toHaveCount(0)
  await expect(created.getByRole('cell', { name: '15:00–16:50' })).toBeVisible()

  await page.getByRole('button', { name: 'Ders seçmeye aç' }).click()
  await expect(page.getByText('Ders seçmeye açıldı.')).toBeVisible()
})
