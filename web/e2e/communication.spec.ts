import { expect, test, type Browser, type Page } from '@playwright/test'

import { login, nav } from './support/app'
import { subjectsSince } from './support/mailpit'

// Faz 6: iletişim. Bölüm başkanı Mehmet Yıldız (P10002) Bilgisayar Mühendisliği'ne duyuru
// yayımlar; demo öğrencilerinin hepsi bu bölümdedir, Elif Şahin (P10003) bölümün öğretim
// elemanıdır. Emre Yılmaz'ın (24290012) danışmanı Ayşe Demir'dir (P10001). Oturumlar ayrı
// tarayıcı bağlamlarında açık kalır: bildirimler ve iletiler sayfa yenilenmeden gelmelidir.
test.describe.configure({ mode: 'serial', timeout: 120_000 })

const suffix = Date.now().toString().slice(-5)

async function session(browser: Browser, user: string): Promise<Page> {
  const page = await (await browser.newContext()).newPage()
  await login(page, user)
  return page
}

/** publish, bölüm başkanı olarak bölümün öğrencilerine duyuru yayımlar. */
async function publish(page: Page, title: string, type?: string) {
  await login(page, 'P10002')
  await nav(page, 'Duyuru yönetimi').click()
  await page.getByRole('link', { name: 'Yeni duyuru' }).click()
  await page.getByLabel('Başlık', { exact: true }).fill(title)
  if (type) {
    await page.getByRole('combobox', { name: 'Tür' }).click()
    await page.getByRole('option', { name: type, exact: true }).click()
  }
  await page.getByLabel('Özet', { exact: true }).fill('Ayrıntılar bölüm panosunda.')
  await page.getByRole('combobox', { name: '1. hedef: birim' }).click()
  await page.getByRole('option', { name: 'Bilgisayar Mühendisliği', exact: true }).click()
  await page.getByRole('combobox', { name: '1. hedef: kişiler' }).click()
  await page.getByRole('option', { name: 'Öğrenciler', exact: true }).click()
  await page.getByRole('button', { name: 'Yayımla' }).click()
  await expect(page.getByText('Duyuru yayımlandı.')).toBeVisible()
}

test('bölüm duyurusu hedef kitledeki öğrenciye canlı bildirim olarak düşer, kitle dışındakine düşmez', async ({
  page,
  browser,
}) => {
  const title = `Laboratuvar saatleri değişti ${suffix}`
  const student = await session(browser, '23290013')
  const staff = await session(browser, 'P10003')

  await publish(page, title)

  // Öğrencinin açık sayfasında uyarı belirir; bildirim listesinde ve duyurularda görünür.
  await expect(student.getByText(`Duyuru: ${title}`)).toBeVisible()
  await student.goto('/duyurular')
  await student.getByRole('link', { name: new RegExp(title) }).click()
  await expect(student.getByRole('heading', { name: title })).toBeVisible()

  // Hedef öğrenciler: aynı bölümün öğretim elemanı ne bildirim alır ne duyuruyu görür.
  await staff.goto('/bildirimler')
  await expect(staff.getByRole('heading', { name: 'Bildirimler', level: 1 })).toBeVisible()
  await expect(staff.getByText(title)).toHaveCount(0)
  await staff.goto('/duyurular')
  await expect(staff.getByRole('heading', { name: 'Duyurular', level: 1 })).toBeVisible()
  await expect(staff.getByText(title)).toHaveCount(0)

  // Yayımlayan duyurunun sayfasındadır: hedef kitleden bir kişi okudu.
  await page.reload()
  await expect(page.getByText('1 kişi okudu')).toBeVisible()
})

test('e-postayı kapatan öğrenciye acil duyuru e-postası gitmez, varsayılandaki öğrenciye gider', async ({
  page,
  browser,
}) => {
  const irem = await session(browser, '26290017')
  await irem.goto('/bildirimler/tercihler')
  const email = irem.getByRole('checkbox', { name: 'Acil duyuru: E-posta' })
  await expect(email).toBeChecked()
  await email.click()
  await irem.getByRole('button', { name: 'Kaydet' }).click()
  await expect(irem.getByText('Bildirim tercihleriniz kaydedildi.')).toBeVisible()

  const since = new Date(Date.now() - 1000)
  const title = `Kampüste elektrik kesintisi ${suffix}`
  await publish(page, title, 'Acil')

  // E-postalar worker'la gider: Kaan Tekin'e (varsayılan kanallar) gelince aynı toplu gönderimin
  // İrem Çakır'a bir e-posta içermediği kesinleşir.
  await expect
    .poll(() => subjectsSince('kaan.tekin@ogrenci.agora.test', since), { timeout: 20_000, intervals: [500] })
    .toContain(`Acil duyuru: ${title}`)
  expect(await subjectsSince('irem.cakir@ogrenci.agora.test', since)).not.toContain(`Acil duyuru: ${title}`)
  // Uygulama içi kanal açık kalmıştır.
  await irem.goto('/bildirimler')
  await expect(irem.getByText(`Acil duyuru: ${title}`)).toBeVisible()
})

test('öğrenci danışmanına yazar; danışman iletiyi canlı görür ve yanıtlar', async ({ page, browser }) => {
  const question = `Hocam merhaba, alttan aldığım ders için görüşebilir miyiz? (${suffix})`
  const answer = `Tabii Emre, salı 14.00'te ofisime gel. (${suffix})`
  const advisor = await session(browser, 'P10001')
  await nav(advisor, 'Mesajlar').click()
  await expect(advisor.getByRole('heading', { name: 'Mesajlar', level: 1 })).toBeVisible()

  await login(page, '24290012')
  await nav(page, 'Mesajlar').click()
  await page.getByRole('button', { name: 'Yeni konuşma' }).click()
  await page
    .getByRole('dialog', { name: 'Yeni konuşma' })
    .getByRole('button', { name: /Ayşe Demir/ })
    .click()
  await page.getByRole('textbox', { name: 'İleti' }).fill(question)
  await page.getByRole('button', { name: 'Gönder' }).click()
  await expect(page.getByRole('list', { name: 'İletiler' }).getByText(question)).toBeVisible()

  // Danışmanın açık listesi canlı tazelenir: konuşma okunmamış iletiyle belirir.
  const conversation = advisor.getByRole('link', { name: /Emre Yılmaz/ })
  await expect(conversation.getByLabel('1 okunmamış ileti')).toBeVisible()
  await conversation.click()
  await expect(advisor.getByRole('list', { name: 'İletiler' }).getByText(question)).toBeVisible()
  await advisor.getByRole('textbox', { name: 'İleti' }).fill(answer)
  await advisor.getByRole('button', { name: 'Gönder' }).click()

  // Yanıt öğrencinin açık konuşmasına canlı gelir.
  await expect(page.getByRole('list', { name: 'İletiler' }).getByText(answer)).toBeVisible()
})
