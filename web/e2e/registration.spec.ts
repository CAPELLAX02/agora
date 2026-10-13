import { expect, test, type Page } from '@playwright/test'

import { login, logout, nav } from './support/app'

// Faz 3 seed'i: demo öğrencilerinin geçmiş dönem notları, BM programı için seed anına göre
// açılan ders seçme, danışman onayı ve ekle-bırak pencereleri, P10001'in danışman kuyruğu
// (backend/cmd/seed). Senaryolar sırayla aynı veritabanını değiştirir.
test.describe.configure({ mode: 'serial' })

async function openRegistration(page: Page, user: string) {
  await login(page, user)
  await nav(page, 'Ders seçimi').click()
  await expect(page.getByRole('heading', { name: '2026-FALL dönemi ders kaydı' })).toBeVisible()
}

const add = (page: Page, label: string) => page.getByRole('button', { name: `${label}: Sepete ekle` }).click()
const basket = (page: Page) => page.getByRole('region', { name: 'Sepetim' })

test('öğrenci ders seçer; danışman geri gönderir, öğrenci düzeltir, danışman onaylar; onay belgesi', async ({
  page,
}) => {
  await openRegistration(page, '22290002')

  // Fizik I'den (PHY0101) kaldı; artık yeni kodu PHY0105 açılıyor ve alttan ders olarak görünüyor.
  const repeat = page.getByRole('region', { name: 'Alttan dersler' })
  await expect(repeat.getByRole('region', { name: /PHY0105/ })).toContainText('PHY0101 yerine sayılır')
  await add(page, 'PHY0105-1')
  await expect(page.getByText('PHY0105-1 sepete eklendi.')).toBeVisible()
  // İşletim Sistemleri'ne önceden devam etti: Fizik ile perşembe çakışması uyarıdır, engel değil.
  await add(page, 'COM3035-1')
  await expect(
    page.getByText(/COM3035-1 ile PHY0105-1 çakışıyor .*devam şartını daha önce sağladığınız/),
  ).toBeVisible()

  // İki yeni seçmeli perşembe öğleden sonra çakışır: ekleme reddedilir.
  await add(page, 'COM4519-1')
  await expect(basket(page).getByText('COM4519-1', { exact: true })).toBeVisible()
  await add(page, 'UNVG105-1')
  await expect(page.getByText(/UNVG105-1 ile COM4519-1 çakışıyor/)).toBeVisible()
  await basket(page).getByRole('button', { name: 'COM4519 dersini sepetten çıkar' }).click()
  await expect(basket(page).getByText('COM4519-1', { exact: true })).toHaveCount(0)

  await add(page, 'COM4061-1')
  await expect(basket(page).getByText('COM4061-1', { exact: true })).toBeVisible()
  await add(page, 'COM4099-1')
  await expect(basket(page).getByText('21 / 40 AKTS').first()).toBeVisible()

  await basket(page).getByRole('button', { name: 'Danışmana gönder' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Danışmana gönder' }).click()
  await expect(page.getByText('Kaydınız danışmanınıza gönderildi.')).toBeVisible()
  await expect(page.getByText(/Kaydınız danışman onayında/)).toBeVisible()
  await logout(page)

  // Danışman geri gönderir (gerekçe zorunlu).
  await login(page, 'P10001')
  await nav(page, 'Danışmanlık').click()
  await page.getByRole('link', { name: 'Ece Kaya' }).click()
  await page.getByRole('button', { name: 'Geri gönder' }).click()
  let dialog = page.getByRole('dialog')
  await dialog.getByLabel('Gerekçe').fill('Bir teknik seçmeli de ekleyin')
  await dialog.getByRole('button', { name: 'Geri gönder' }).click()
  await expect(page.getByText('Kayıt öğrenciye geri gönderildi.')).toBeVisible()
  await logout(page)

  // Öğrenci gerekçeyi görür, sepeti düzeltip yeniden gönderir.
  await openRegistration(page, '22290002')
  await expect(page.getByText('Ayşe Demir kaydınızı düzeltilmek üzere geri gönderdi')).toBeVisible()
  await expect(page.getByText('Bir teknik seçmeli de ekleyin')).toBeVisible()
  await add(page, 'COM4507-1')
  await expect(basket(page).getByText('COM4507-1', { exact: true })).toBeVisible()
  await basket(page).getByRole('button', { name: 'Danışmana gönder' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Danışmana gönder' }).click()
  await expect(page.getByText('Kaydınız danışmanınıza gönderildi.')).toBeVisible()
  await logout(page)

  await login(page, 'P10001')
  await nav(page, 'Danışmanlık').click()
  await page.getByRole('link', { name: 'Ece Kaya' }).click()
  await page.getByRole('button', { name: 'Onayla' }).click()
  dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Onayla' }).click()
  await expect(page.getByText('Kayıt onaylandı.')).toBeVisible()
  await logout(page)

  await openRegistration(page, '22290002')
  await expect(page.getByText(/Ayşe Demir kaydınızı onayladı/)).toBeVisible()
  await page.getByRole('link', { name: 'Kayıt onay belgesi' }).click()
  const report = page.getByRole('article', { name: 'Ders Kayıt Onay Belgesi' })
  await expect(report.getByRole('cell', { name: 'COM4507' })).toBeVisible()
  await expect(report).toContainText('Toplam (5 ders)')
})

test('danışman onay bekleyen kayıtları toplu onaylar', async ({ page }) => {
  await login(page, 'P10001')
  await nav(page, 'Danışmanlık').click()
  await page.getByRole('checkbox', { name: 'Buse Demirtaş kaydını seç' }).click()
  await page.getByRole('checkbox', { name: 'Kaan Tekin kaydını seç' }).click()
  await page.getByRole('button', { name: 'Seçilenleri onayla (2)' }).click()
  await page.getByRole('alertdialog').getByRole('button', { name: 'Onayla' }).click()
  await expect(page.getByText('2 kayıt onaylandı.')).toBeVisible()
  await expect(page.getByRole('row', { name: /Buse Demirtaş/ }).getByText('Onaylandı')).toBeVisible()
})

test('ekle-bırak: öğrenci talep eder, danışman ve bölüm başkanı onaylar, kayda uygulanır', async ({
  page,
}) => {
  // Derya'nın kaydı seed'de onaylı.
  await openRegistration(page, '23290015')
  await page.getByRole('button', { name: 'Yeni ekle-bırak talebi' }).click()
  // Bırakılacak ders: sadece onay kutusu (alınabilir dersler listesinde aynı şube "Sepette" görünür).
  await page.getByRole('checkbox', { name: /^COM4507-1/ }).check()
  await page.getByRole('button', { name: 'COM4511-1: Talebe ekle' }).click()
  await page.getByRole('button', { name: 'Talebi gönder' }).click()
  await expect(page.getByText('Ekle-bırak talebiniz danışmanınıza gönderildi.')).toBeVisible()
  await expect(
    page.getByRole('list', { name: 'Ekle-bırak talepleri' }).getByText('Danışman onayında'),
  ).toBeVisible()
  await logout(page)

  for (const user of ['P10001', 'P10002']) {
    await login(page, user)
    await nav(page, 'Ekle-bırak onayları').click()
    const request = page.getByRole('region', { name: 'Derya Uçar talebi' })
    await request.getByRole('button', { name: 'Onayla' }).click()
    await page.getByRole('dialog').getByRole('button', { name: 'Onayla' }).click()
    await expect(page.getByText('Talep onaylandı.')).toBeVisible()
    await logout(page)
  }

  await openRegistration(page, '23290015')
  await expect(page.getByRole('list', { name: 'Ekle-bırak talepleri' }).getByText('Uygulandı')).toBeVisible()
  await expect(basket(page).getByText('COM4511-1', { exact: true })).toBeVisible()
  await expect(basket(page).getByText('COM4507-1', { exact: true })).toHaveCount(0)
})

test('kayıt engeli olan öğrenci nedenini görür ve ders ekleyemez', async ({ page }) => {
  await openRegistration(page, '24290014')
  await expect(
    basket(page).getByText(/Ders seçmenizi engelleyen bir kaydınız var \(katkı payı borcu\)/),
  ).toBeVisible()
  await page
    .getByRole('button', { name: /: Sepete ekle$/ })
    .first()
    .click()
  // Sepetteki kural listesine ek olarak işlemin reddi de bildirilir.
  await expect(page.getByText(/engelleyen bir kaydınız var/)).toHaveCount(2)
})
