import '@testing-library/jest-dom/vitest'
import '@/i18n'

import { cleanup } from '@testing-library/react'
import { afterAll, afterEach, beforeAll, beforeEach } from 'vitest'

import i18n from '@/i18n'

import { server } from './server'

// jsdom matchMedia'yı uygulamaz: tema sağlayıcısı sistem temasını buradan okur.
// Testlerde sistem teması her zaman açıktır.
const noop = () => undefined
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: noop,
    removeEventListener: noop,
    addListener: noop,
    removeListener: noop,
    dispatchEvent: () => false,
  }),
})

// Radix Select'in kullandığı ve jsdom'un uygulamadığı DOM API'leri.
for (const [name, value] of [
  ['hasPointerCapture', () => false],
  ['releasePointerCapture', noop],
  ['scrollIntoView', noop],
] as const) {
  if (!(name in Element.prototype)) {
    Object.defineProperty(Element.prototype, name, { value, writable: true })
  }
}

// Sunucuya giden her istek bir handler'la karşılanmalı: unutulan bir uç testte hata verir.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(async () => {
  localStorage.clear()
  await i18n.changeLanguage('tr')
})
afterEach(() => {
  cleanup()
  server.resetHandlers()
})
afterAll(() => server.close())
