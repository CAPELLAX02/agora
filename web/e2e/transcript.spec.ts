import { expect, test } from '@playwright/test'

import { login, nav } from './support/app'

// Seed'deki ders geçmişi: 22290002 (Ece Kaya) 2023 güzünden beri ders alıyor, Fizik I'den iki
// kez kaldı ve 2026 güzde yeni koduyla (PHY0105) tekrar alıyor.
test('öğrenci transkriptinde dönem dönem notlarını ve ortalamalarını görür', async ({ page }) => {
  await login(page, '22290002')
  await nav(page, 'Transkript').click()

  await expect(page.getByRole('heading', { name: 'Ece Kaya' })).toBeVisible()
  const first = page.getByRole('region', { name: '2023-2024 Güz' })
  await expect(first.getByText('YANO')).toBeVisible()
  await expect(first.getByRole('row', { name: /PHY0101/ }).getByText('F2')).toHaveClass(/line-through/)

  const current = page.getByRole('region', { name: '2026-2027 Güz' })
  await expect(current.getByText('Notlar henüz ilan edilmedi')).toBeVisible()
  await expect(current.getByRole('row', { name: /PHY0105/ }).getByText('tekrar')).toBeAttached()
  await expect(page.getByRole('meter', { name: 'Kazanılan AKTS' })).toBeVisible()

  // Sanal transkript: süren tekrar dersine B1 verilince eski F2 GANO'dan düşer; bir şey kaydedilmez.
  await page.getByRole('button', { name: 'Ne olur?' }).click()
  await current.getByRole('combobox', { name: 'PHY0105 için varsayımsal not' }).selectOption('B1')
  const bar = page.getByRole('status', { name: 'Sanal transkript' })
  await expect(bar.getByText(/^\+0\.\d\d$/)).toBeVisible()
  await expect(current.getByText('Sanal', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Sanal transkripti kapat' }).click()
  await expect(current.getByText('Notlar henüz ilan edilmedi')).toBeVisible()
})
