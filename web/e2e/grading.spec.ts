import { expect, test, type Page } from '@playwright/test'

import { login, logout, nav } from './support/app'
import { enableMfa, mfaAccount, mfaLogin } from './support/mfa'

// Faz 4 seed'i: Mühendislik Fakültesi'nde seed anına göre açık not girişi pencereleri (ara sınav,
// final, elle harf notu, bütünleme) ve P10001'in COM1007-1 şubesinde 20 kişilik 1. sınıf grubu:
// ara sınav girilip yayımlanmış, laboratuvarda iki not eksik, final boş (backend/cmd/seed).
// Bu dosya ayrı bir projede, diğer senaryolardan sonra çalışır: P10001 ve P10002'ye MFA kurar.
test.describe.configure({ mode: 'serial', timeout: 120_000 })

const instructor = mfaAccount('P10001')
const head = mfaAccount('P10002')

// Final notları; Berk Karaca final barajının altında kalır (bütünleme hakkı).
const finals: Record<string, string> = {
  'Ada Aydemir': '78',
  'Ceren Doğan': '65',
  'Doruk Ekinci': '90',
  'Ela Gündoğdu': '72',
  'Efe Halıcı': '58',
  'Ezgi Işık': '81',
  'Furkan Kalkan': '70',
  'Berk Karaca': '30',
  'Gökçe Kartal': '66',
  'Hakan Korkmaz': '74',
  'Ilgın Mutlu': '88',
  'Kerem Oral': '62',
  'Lale Özkan': '77',
  'Mete Sarı': '69',
  'Nehir Soylu': '55',
  'Ozan Taş': '83',
  'Pelin Turan': '91',
  'Rüzgar Uslu': '60',
  'Selin Vural': '79',
  'Tuna Yalçın': '64',
}

async function openGradebook(page: Page) {
  await nav(page, 'Verdiğim dersler').click()
  await page
    .getByRole('listitem')
    .filter({ hasText: 'COM1007-1' })
    .getByRole('link', { name: 'Not tablosu' })
    .click()
  await expect(page.getByRole('heading', { name: 'Not tablosu' })).toBeVisible()
}

test('öğretim elemanı MFA kurar, notları girer, bileşeni yayımlar ve harf notlarını ilan eder', async ({
  page,
}) => {
  await login(page, instructor.username)
  await openGradebook(page)
  // MFA'sız tablo salt okunurdur.
  await expect(page.getByText(/iki adımlı doğrulamayla açılmış bir oturum gerekir/)).toBeVisible()
  await expect(page.getByLabel('Ada Aydemir · Final sınavı')).toHaveCount(0)

  await enableMfa(page, instructor)
  await openGradebook(page)

  for (const [name, score] of Object.entries(finals)) {
    await page.getByLabel(`${name} · Final sınavı`).fill(score)
  }
  await page.getByLabel('Nehir Soylu · Laboratuvar').fill('70')
  await page.getByLabel('Selin Vural · Laboratuvar').fill('g')
  const bar = page.getByRole('region', { name: 'Kaydedilmemiş değişiklikler' })
  await expect(bar.getByText('22 kaydedilmemiş değişiklik')).toBeVisible()
  await bar.getByRole('button', { name: 'Kaydet' }).click()
  await expect(page.getByText('22 not kaydedildi')).toBeVisible()
  await expect(bar).toHaveCount(0)

  const lab = page
    .getByRole('list', { name: 'Bileşenler' })
    .getByRole('listitem')
    .filter({ hasText: 'Laboratuvar' })
  await lab.getByRole('button', { name: 'Öğrencilere yayımla' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Öğrencilere yayımla' }).click()
  await expect(lab.getByText('Yayımlandı')).toBeVisible()

  // Harf notları: eksik not yok, dağılım ve istatistikler; ilan onayla.
  await page.getByRole('link', { name: 'Harf notları' }).click()
  await expect(page.getByRole('heading', { name: 'Harf notları' })).toBeVisible()
  await expect(page.getByText('20/20')).toBeVisible()
  await expect(page.getByText(/notu eksik/)).toHaveCount(0)
  await expect(page.getByRole('row', { name: /Berk Karaca/ }).getByText('Final barajı altı')).toBeVisible()
  await page.getByRole('button', { name: 'Harf notlarını ilan et' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Harf notlarını ilan et' }).click()
  await expect(page.getByText('Harf notları ilan edildi', { exact: true })).toBeVisible()
  await expect(page.getByText(/öğrencinin bütünleme hakkı var/)).toBeVisible()
  await expect(page.getByRole('row', { name: /Berk Karaca/ }).getByText('Bütünleme hakkı')).toBeVisible()
})

test('öğrenci harf notunu, sınıf ortalamasını ve transkriptini görür', async ({ page }) => {
  await login(page, '26290101')
  const latest = page
    .getByRole('heading', { name: 'Son notlarım' })
    .locator('xpath=ancestor::*[@data-slot="card"]')
  await expect(latest.getByText('Bilgisayar Programlama I')).toBeVisible()

  await nav(page, 'Notlarım').click()
  const course = page.getByRole('article', { name: /COM1007/ })
  await expect(course.getByText(/^(A|B[123]|C[123]|F[1-4])$/)).toBeVisible()
  await expect(course.getByText('Sınıf ortalaması').first()).toBeVisible()

  await nav(page, 'Transkript').click()
  const term = page.getByRole('region', { name: '2026-2027 Güz' })
  await expect(
    term.getByRole('row', { name: /COM1007/ }).getByText(/^(A|B[123]|C[123]|F[1-4])$/),
  ).toBeVisible()
  // Dönem sonucu (YANO) not ilanı olayıyla worker'da hesaplanır: birkaç saniye sürebilir.
  await expect(async () => {
    await page.reload()
    await expect(page.getByRole('region', { name: '2026-2027 Güz' }).getByText('YANO')).toBeVisible({
      timeout: 2000,
    })
  }).toPass({ timeout: 30_000 })
})

test('bütünleme notu sadece hakkı olana girilir; bütünleme ilanıyla harf değişir; not düzeltme talebi', async ({
  page,
}) => {
  await mfaLogin(page, instructor)
  await openGradebook(page)
  await expect(page.getByText(/Harf notları .* ilan edildi/)).toBeVisible()
  await expect(page.getByLabel('Ada Aydemir · Bütünleme')).toHaveCount(0)
  await expect(page.getByLabel('Ada Aydemir · Final sınavı')).toHaveCount(0)
  await page.getByLabel('Berk Karaca · Bütünleme').fill('85')
  await page
    .getByRole('region', { name: 'Kaydedilmemiş değişiklikler' })
    .getByRole('button', { name: 'Kaydet' })
    .click()
  await expect(page.getByText('1 not kaydedildi')).toBeVisible()

  await page.getByRole('link', { name: 'Harf notları' }).click()
  const berk = page.getByRole('row', { name: /Berk Karaca/ })
  await expect(berk.getByText(/^Bütünlemeyle/)).toBeVisible() // bütünleme notuyla olacak harf
  await page.getByRole('button', { name: 'Bütünleme sonuçlarını ilan et' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Bütünleme sonuçlarını ilan et' }).click()
  await expect(page.getByText('Bütünleme sonuçları ilan edildi', { exact: true })).toBeVisible()
  await expect(berk.locator('.line-through')).toHaveCount(1) // bütünlemeden önceki harf

  // İlandan sonra not sadece düzeltme talebiyle değişir.
  await nav(page, 'Not düzeltme').click()
  await page.getByRole('button', { name: 'Yeni talep' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('combobox', { name: 'Şube' }).click()
  await page.getByRole('option', { name: /COM1007-1/ }).click()
  await dialog.getByRole('combobox', { name: 'Öğrenci' }).click()
  await page.getByRole('option', { name: /26290103/ }).click()
  await dialog.getByRole('combobox', { name: 'Bileşen' }).click()
  await page.getByRole('option', { name: /Ara sınav/ }).click()
  await dialog.getByLabel('Doğru puan').fill('95')
  await expect(dialog.getByRole('status')).toContainText('95')
  await dialog.getByLabel('Gerekçe').fill('Ara sınav kâğıdında toplama hatası: 3. soru 20 puan.')
  await dialog.getByRole('button', { name: 'Talep et' }).click()
  await expect(
    page.getByRole('article', { name: 'COM1007 · Ceren Doğan' }).getByText('Onay bekliyor'),
  ).toBeVisible()
  await logout(page)
})

test('bölüm başkanı MFA kurar ve not düzeltmesini onaylayıp uygular', async ({ page }) => {
  await login(page, head.username)
  await enableMfa(page, head)
  await nav(page, 'Not düzeltme onayları').click()
  await expect(page.getByRole('heading', { name: 'Onay bekleyen (1)' })).toBeVisible()
  const card = page.getByRole('article', { name: 'COM1007 · Ceren Doğan' })
  await card.getByRole('button', { name: 'Onayla ve uygula' }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Onayla ve uygula' }).click()
  await expect(page.getByText('Düzeltme uygulandı.')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Onay bekleyen (0)' })).toBeVisible()
})
