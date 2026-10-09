import 'i18next'

import type tr from './locales/tr.json'

// Çeviri anahtarları tip olarak denetlenir: olmayan bir anahtar derleme hatasıdır.
declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    resources: { translation: typeof tr }
  }
}
