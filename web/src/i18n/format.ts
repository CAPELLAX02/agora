import i18n from './index'

const locales: Record<string, string> = { tr: 'tr-TR', en: 'en-GB' }

function locale() {
  return locales[i18n.language] ?? 'tr-TR'
}

/** formatDateTime, tarih ve saati kullanıcının dilinde kısa biçimde gösterir. */
export function formatDateTime(value: string | Date | null | undefined): string {
  if (!value) {
    return '—'
  }
  return new Intl.DateTimeFormat(locale(), { dateStyle: 'medium', timeStyle: 'short' }).format(
    new Date(value),
  )
}

/** formatDate, sadece tarihi gösterir. */
export function formatDate(value: string | Date | null | undefined): string {
  if (!value) {
    return '—'
  }
  return new Intl.DateTimeFormat(locale(), { dateStyle: 'medium' }).format(new Date(value))
}

/** formatRelative, "3 dakika önce" gibi göreli zamanı gösterir. */
export function formatRelative(value: string | Date, now: Date = new Date()): string {
  const diff = (new Date(value).getTime() - now.getTime()) / 1000
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto' })
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['year', 31536000],
    ['month', 2592000],
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
  ]
  for (const [unit, seconds] of units) {
    if (Math.abs(diff) >= seconds) {
      return rtf.format(Math.round(diff / seconds), unit)
    }
  }
  return rtf.format(Math.round(diff), 'second')
}

/** formatDuration, saniye cinsinden süreyi "2 dakika" ya da "45 saniye" gibi gösterir. */
export function formatDuration(seconds: number): string {
  const minutes = Math.ceil(seconds / 60)
  if (seconds < 60) {
    return new Intl.NumberFormat(locale(), { style: 'unit', unit: 'second', unitDisplay: 'long' }).format(
      Math.ceil(seconds),
    )
  }
  return new Intl.NumberFormat(locale(), { style: 'unit', unit: 'minute', unitDisplay: 'long' }).format(
    minutes,
  )
}
