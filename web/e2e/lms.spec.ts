import { expect, test, type Page } from '@playwright/test'

import { login, logout, nav } from './support/app'

// Faz 5: ders alanı. Kayıt senaryosunda (registration.spec) danışmanı Ece Kaya'nın (22290002)
// kaydını onaylar; COM4507-1'in öğretim elemanı Burak Arslan'dır (P10004). Şubenin seed'deki
// değerlendirme planı (proje %25, sunum %15, final %60) kilitli değildir; ders alanı henüz yoktur.
// Bu dosya ayrı bir projede, kayıt senaryolarından sonra çalışır.
test.describe.configure({ mode: 'serial', timeout: 120_000 })

const teacher = 'P10004'
const student = '22290002'

const pdf = (name: string) => ({
  name,
  mimeType: 'application/pdf',
  buffer: Buffer.from('%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n'),
})

async function openSpace(page: Page) {
  await nav(page, 'Derslerim').click()
  // Ana sayfadaki kartlarda da şubenin bağlantıları var: önce liste açılmalı.
  await expect(page.getByRole('heading', { name: 'Derslerim', level: 1 })).toBeVisible()
  await page.getByRole('link', { name: /COM4507-1/ }).click()
  await expect(page.getByText(/^COM4507-1 · 2026-FALL/)).toBeVisible()
}

/** activity, ders alanındaki etkinlik satırıdır (düzenleme kapalıyken). */
const activity = (page: Page, title: string) => page.getByRole('button', { name: new RegExp(`^${title}`) })

async function addActivity(page: Page, topic: string, type: string, title: string) {
  await page
    .getByRole('region', { name: topic })
    .getByRole('button', { name: 'Etkinlik ya da kaynak ekle' })
    .click()
  await page.getByRole('menuitem', { name: type, exact: true }).click()
  const dialog = page.getByRole('dialog', { name: `Yeni ${type}` })
  await dialog.getByLabel('Başlık', { exact: true }).fill(title)
  return dialog
}

test('öğretim elemanı planı kilitler; ders alanına bölüm, dosya, ödev ve forum ekler; ödevi bileşene bağlar', async ({
  page,
}) => {
  await login(page, teacher)
  await nav(page, 'Verdiğim dersler').click()
  await page
    .getByRole('listitem')
    .filter({ hasText: 'COM4507-1' })
    .getByRole('link', { name: 'Değerlendirme planı' })
    .click()
  await page.getByRole('button', { name: 'Planı kilitle' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Planı kilitle' }).click()
  await expect(page.getByText('Plan kilitlendi.')).toBeVisible()

  // İlk bölümle birlikte "Genel" bölümü ve duyuru forumu oluşur.
  await openSpace(page)
  await page.getByRole('button', { name: 'Düzenlemeyi aç' }).click()
  await page.getByRole('button', { name: 'Bölüm ekle' }).click()
  const topic = page.getByRole('dialog', { name: 'Yeni bölüm' })
  await topic.getByLabel('Başlık', { exact: true }).fill('1. Hafta')
  await topic.getByRole('button', { name: 'Kaydet', exact: true }).click()
  await expect(page.getByText('Bölüm eklendi.')).toBeVisible()
  await expect(
    page.getByRole('region', { name: 'Genel' }).getByText('Duyurular', { exact: true }),
  ).toBeVisible()

  let dialog = await addActivity(page, '1. Hafta', 'Dosya', 'Proje yönergesi')
  await dialog.getByLabel('Dosya seç').setInputFiles(pdf('proje-yonergesi.pdf'))
  await expect(dialog.getByRole('button', { name: 'proje-yonergesi.pdf dosyasını kaldır' })).toBeVisible()
  await dialog.getByRole('button', { name: 'Kaydet', exact: true }).click()
  await expect(page.getByText('Etkinlik eklendi.')).toBeVisible()

  // Ödev: son tarih bir hafta sonrası önerilir, dosya teslimi ve 100 puan varsayılandır.
  dialog = await addActivity(page, '1. Hafta', 'Ödev', 'Proje önerisi')
  await expect(dialog.getByLabel('Son tarih')).not.toHaveValue('')
  await dialog.getByRole('button', { name: 'Kaydet', exact: true }).click()
  await expect(dialog).toHaveCount(0)

  dialog = await addActivity(page, 'Genel', 'Forum', 'Soru-cevap')
  await expect(dialog.getByRole('checkbox', { name: 'Öğrenciler konu açabilir' })).toBeChecked()
  await dialog.getByRole('button', { name: 'Kaydet', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await page.getByRole('button', { name: 'Düzenleme açık' }).click()

  // Duyuru.
  await activity(page, 'Duyurular').click()
  await page.getByRole('button', { name: 'Yeni duyuru' }).click()
  dialog = page.getByRole('dialog')
  await dialog.getByLabel('Başlık', { exact: true }).fill('Proje takvimi')
  await dialog.getByRole('textbox', { name: 'İleti' }).fill('Proje önerileri bir hafta içinde yüklenecek.')
  await dialog.getByRole('button', { name: 'Yayımla' }).click()
  await expect(page.getByRole('heading', { name: 'Proje takvimi', level: 1 })).toBeVisible()

  // Ödevin puanları proje bileşenine akar.
  await openSpace(page)
  await activity(page, 'Proje önerisi').click()
  const link = page.getByRole('region', { name: 'Not tablosu bağlantısı' })
  await expect(link.getByText('Bağlı değil: ödev puanları not tablosuna akmaz.')).toBeVisible()
  await link.getByRole('combobox', { name: 'Değerlendirme bileşeni' }).click()
  await page.getByRole('option', { name: /^Proje/ }).click()
  await link.getByRole('button', { name: 'Bağlantıyı kaydet' }).click()
  await expect(page.getByText('Bağlantı kaydedildi; bileşen notları güncellendi.')).toBeVisible()
  await logout(page)
})

test('kaydı onaylanan öğrenci ders alanını görür, dosyayı indirir, forumda soru sorar ve ödevi teslim eder', async ({
  page,
}) => {
  await login(page, student)
  // Ana sayfadaki yapılacaklar listesinde son tarihi yaklaşan ödev.
  await expect(page.getByRole('link', { name: /Proje önerisi/ })).toBeVisible()

  await openSpace(page)
  await expect(page.getByText('0 / 4 etkinlik tamamlandı')).toBeVisible()
  const download = page.waitForEvent('download')
  await activity(page, 'Proje yönergesi').click()
  expect((await download).suggestedFilename()).toBe('proje-yonergesi.pdf')
  await expect(activity(page, 'Proje yönergesi').getByLabel('Tamamlandı')).toBeVisible()

  await activity(page, 'Soru-cevap').click()
  await page.getByRole('button', { name: 'Yeni konu' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByLabel('Başlık', { exact: true }).fill('Proje konusu serbest mi?')
  await dialog.getByRole('textbox', { name: 'İleti' }).fill('Kendi proje konumuzu önerebilir miyiz?')
  await dialog.getByRole('button', { name: 'Yayımla' }).click()
  await expect(page.getByRole('heading', { name: 'Proje konusu serbest mi?', level: 1 })).toBeVisible()

  await openSpace(page)
  await activity(page, 'Proje önerisi').click()
  await expect(page.getByRole('heading', { name: 'Proje önerisi', level: 1 })).toBeVisible()
  await page.getByLabel('Dosya seç').setInputFiles(pdf('oneri.pdf'))
  await expect(page.getByRole('button', { name: 'oneri.pdf dosyasını kaldır' })).toBeVisible()
  await page.getByRole('button', { name: 'Gönder', exact: true }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Gönder', exact: true }).click()
  await expect(page.getByText('Ödev gönderildi.')).toBeVisible()
  await expect(page.getByText('Puanlar henüz yayımlanmadı')).toBeVisible()

  await openSpace(page)
  await expect(page.getByText('3 / 4 etkinlik tamamlandı')).toBeVisible()
  await logout(page)
})

test('öğretim elemanı teslimi puanlayıp yayımlar, puan proje bileşenine akar; soruyu yanıtlar', async ({
  page,
}) => {
  await login(page, teacher)
  await openSpace(page)
  await activity(page, 'Proje önerisi').click()
  await page.getByRole('button', { name: 'Ece Kaya teslimini puanla' }).click()
  const sheet = page.getByRole('dialog', { name: 'Ece Kaya' })
  await expect(sheet.getByText('oneri.pdf')).toBeVisible()
  await sheet.getByLabel('Puan (en çok 100)').fill('85')
  await sheet.getByRole('textbox', { name: 'Geri bildirim' }).fill('Konu net; zaman planını ayrıntılandır.')
  await sheet.getByRole('button', { name: 'Puanı kaydet' }).click()
  await expect(page.getByText('Puan kaydedildi.')).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Puanları yayımla' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Puanları yayımla' }).click()
  await expect(page.getByText('Puanlar yayımlandı.')).toBeVisible()

  // Not tablosu (MFA'sız salt okunur): proje bileşeni ödevlerden beslenir.
  await nav(page, 'Verdiğim dersler').click()
  await page
    .getByRole('listitem')
    .filter({ hasText: 'COM4507-1' })
    .getByRole('link', { name: 'Not tablosu' })
    .click()
  await expect(page.getByRole('heading', { name: 'Not tablosu' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: /Proje.*Ödevlerden/ })).toBeVisible()
  // Hücreler: devam durumu, proje, sunum, final.
  await expect(
    page
      .getByRole('row', { name: /Ece Kaya/ })
      .getByRole('cell')
      .nth(1),
  ).toHaveText('85')

  await openSpace(page)
  await activity(page, 'Soru-cevap').click()
  await page.getByRole('link', { name: /Proje konusu serbest mi\?/ }).click()
  const reply = page.getByRole('region', { name: 'Yanıt yaz' })
  await reply.getByRole('textbox', { name: 'Yanıtınız' }).fill('Evet; önerinizde konunun kapsamını belirtin.')
  await reply.getByRole('button', { name: 'Gönder', exact: true }).click()
  await expect(page.getByText('Yanıt gönderildi.')).toBeVisible()
  await logout(page)
})

test('öğrenci puanını, geri bildirimi, yanıtı ve duyuruyu görür; ilerlemesi tamamlanır', async ({ page }) => {
  await login(page, student)
  await openSpace(page)
  await activity(page, 'Proje önerisi').click()
  const grade = page
    .getByText('Puan ve geri bildirim', { exact: true })
    .locator('xpath=ancestor::*[@data-slot="card"]')
  await expect(grade).toContainText('85')
  await expect(grade.getByText('Konu net; zaman planını ayrıntılandır.')).toBeVisible()

  await openSpace(page)
  await activity(page, 'Soru-cevap').click()
  await page.getByRole('link', { name: /Proje konusu serbest mi\?/ }).click()
  await expect(page.getByText('Evet; önerinizde konunun kapsamını belirtin.')).toBeVisible()

  await openSpace(page)
  await activity(page, 'Duyurular').click()
  await expect(page.getByRole('link', { name: /Proje takvimi/ })).toBeVisible()
  await openSpace(page)
  await expect(page.getByText('4 / 4 etkinlik tamamlandı')).toBeVisible()
})
