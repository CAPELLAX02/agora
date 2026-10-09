import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import en from './locales/en.json'
import tr from './locales/tr.json'

export const languages = ['tr', 'en'] as const
export type Language = (typeof languages)[number]

const STORAGE_KEY = 'agora.lang'

function initialLanguage(): Language {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'tr' || v === 'en') {
      return v
    }
  } catch {
    // Depolama engellenmiş: varsayılan dil.
  }
  return 'tr'
}

void i18n.use(initReactI18next).init({
  resources: { tr: { translation: tr }, en: { translation: en } },
  lng: initialLanguage(),
  fallbackLng: 'tr',
  interpolation: { escapeValue: false }, // React zaten kaçışlar
})

document.documentElement.lang = i18n.language

i18n.on('languageChanged', (lng) => {
  document.documentElement.lang = lng
  try {
    localStorage.setItem(STORAGE_KEY, lng)
  } catch {
    // Seçim bu oturumla sınırlı kalır.
  }
})

export default i18n
