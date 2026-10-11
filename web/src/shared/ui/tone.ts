/**
 * Renk tonları: kategorileri (alttan ders, seçmeli, sınav ...) ve kişileri/dersleri
 * birbirinden ayırmak için. Sınıflar Tailwind'in tarayabilmesi için burada tam yazılır.
 */
export type Tone =
  | 'indigo'
  | 'violet'
  | 'fuchsia'
  | 'pink'
  | 'rose'
  | 'orange'
  | 'amber'
  | 'lime'
  | 'emerald'
  | 'teal'
  | 'sky'
  | 'blue'
  | 'slate'

/** Yumuşak zemin + koyu metin: ikon kutuları, rozetler. */
export const toneSoft: Record<Tone, string> = {
  indigo: 'bg-indigo-500/12 text-indigo-700 dark:bg-indigo-400/15 dark:text-indigo-300',
  violet: 'bg-violet-500/12 text-violet-700 dark:bg-violet-400/15 dark:text-violet-300',
  fuchsia: 'bg-fuchsia-500/12 text-fuchsia-700 dark:bg-fuchsia-400/15 dark:text-fuchsia-300',
  pink: 'bg-pink-500/12 text-pink-700 dark:bg-pink-400/15 dark:text-pink-300',
  rose: 'bg-rose-500/12 text-rose-700 dark:bg-rose-400/15 dark:text-rose-300',
  orange: 'bg-orange-500/12 text-orange-700 dark:bg-orange-400/15 dark:text-orange-300',
  amber: 'bg-amber-500/15 text-amber-800 dark:bg-amber-400/15 dark:text-amber-300',
  lime: 'bg-lime-500/15 text-lime-800 dark:bg-lime-400/15 dark:text-lime-300',
  emerald: 'bg-emerald-500/12 text-emerald-700 dark:bg-emerald-400/15 dark:text-emerald-300',
  teal: 'bg-teal-500/12 text-teal-700 dark:bg-teal-400/15 dark:text-teal-300',
  sky: 'bg-sky-500/12 text-sky-700 dark:bg-sky-400/15 dark:text-sky-300',
  blue: 'bg-blue-500/12 text-blue-700 dark:bg-blue-400/15 dark:text-blue-300',
  slate: 'bg-slate-500/10 text-slate-700 dark:bg-slate-400/15 dark:text-slate-300',
}

/** Dolgun zemin + beyaz metin: avatarlar, öne çıkan rozetler. */
export const toneSolid: Record<Tone, string> = {
  indigo: 'bg-gradient-to-br from-indigo-500 to-indigo-600 text-white',
  violet: 'bg-gradient-to-br from-violet-500 to-purple-600 text-white',
  fuchsia: 'bg-gradient-to-br from-fuchsia-500 to-purple-600 text-white',
  pink: 'bg-gradient-to-br from-pink-500 to-rose-500 text-white',
  rose: 'bg-gradient-to-br from-rose-500 to-red-500 text-white',
  orange: 'bg-gradient-to-br from-orange-400 to-orange-600 text-white',
  amber: 'bg-gradient-to-br from-amber-400 to-orange-500 text-white',
  lime: 'bg-gradient-to-br from-lime-500 to-green-600 text-white',
  emerald: 'bg-gradient-to-br from-emerald-500 to-teal-600 text-white',
  teal: 'bg-gradient-to-br from-teal-500 to-cyan-600 text-white',
  sky: 'bg-gradient-to-br from-sky-500 to-blue-600 text-white',
  blue: 'bg-gradient-to-br from-blue-500 to-indigo-600 text-white',
  slate: 'bg-gradient-to-br from-slate-500 to-slate-700 text-white',
}

/** Sol kenar vurgusu: kategori kartları. */
export const toneEdge: Record<Tone, string> = {
  indigo: 'before:bg-indigo-500',
  violet: 'before:bg-violet-500',
  fuchsia: 'before:bg-fuchsia-500',
  pink: 'before:bg-pink-500',
  rose: 'before:bg-rose-500',
  orange: 'before:bg-orange-500',
  amber: 'before:bg-amber-500',
  lime: 'before:bg-lime-500',
  emerald: 'before:bg-emerald-500',
  teal: 'before:bg-teal-500',
  sky: 'before:bg-sky-500',
  blue: 'before:bg-blue-500',
  slate: 'before:bg-slate-400',
}

/** İlerleme çubuğu dolgusu. */
export const toneFill: Record<Tone, string> = {
  indigo: 'bg-gradient-to-r from-indigo-500 to-violet-500',
  violet: 'bg-gradient-to-r from-violet-500 to-fuchsia-500',
  fuchsia: 'bg-gradient-to-r from-fuchsia-500 to-pink-500',
  pink: 'bg-gradient-to-r from-pink-500 to-rose-500',
  rose: 'bg-gradient-to-r from-rose-500 to-red-500',
  orange: 'bg-gradient-to-r from-orange-400 to-orange-500',
  amber: 'bg-gradient-to-r from-amber-400 to-orange-400',
  lime: 'bg-gradient-to-r from-lime-400 to-green-500',
  emerald: 'bg-gradient-to-r from-emerald-400 to-teal-500',
  teal: 'bg-gradient-to-r from-teal-400 to-cyan-500',
  sky: 'bg-gradient-to-r from-sky-400 to-blue-500',
  blue: 'bg-gradient-to-r from-blue-500 to-indigo-500',
  slate: 'bg-slate-400',
}

/** Haftalık programdaki ders blokları: açık zemin, renkli kenar ve metin. */
export const toneBlock: Record<Tone, string> = {
  indigo: 'border-indigo-400/60 bg-indigo-50 text-indigo-900 dark:bg-indigo-500/15 dark:text-indigo-100',
  violet: 'border-violet-400/60 bg-violet-50 text-violet-900 dark:bg-violet-500/15 dark:text-violet-100',
  fuchsia:
    'border-fuchsia-400/60 bg-fuchsia-50 text-fuchsia-900 dark:bg-fuchsia-500/15 dark:text-fuchsia-100',
  pink: 'border-pink-400/60 bg-pink-50 text-pink-900 dark:bg-pink-500/15 dark:text-pink-100',
  rose: 'border-rose-400/60 bg-rose-50 text-rose-900 dark:bg-rose-500/15 dark:text-rose-100',
  orange: 'border-orange-400/60 bg-orange-50 text-orange-900 dark:bg-orange-500/15 dark:text-orange-100',
  amber: 'border-amber-400/70 bg-amber-50 text-amber-900 dark:bg-amber-500/15 dark:text-amber-100',
  lime: 'border-lime-500/60 bg-lime-50 text-lime-900 dark:bg-lime-500/15 dark:text-lime-100',
  emerald:
    'border-emerald-400/60 bg-emerald-50 text-emerald-900 dark:bg-emerald-500/15 dark:text-emerald-100',
  teal: 'border-teal-400/60 bg-teal-50 text-teal-900 dark:bg-teal-500/15 dark:text-teal-100',
  sky: 'border-sky-400/60 bg-sky-50 text-sky-900 dark:bg-sky-500/15 dark:text-sky-100',
  blue: 'border-blue-400/60 bg-blue-50 text-blue-900 dark:bg-blue-500/15 dark:text-blue-100',
  slate: 'border-slate-300 bg-slate-50 text-slate-800 dark:bg-slate-500/15 dark:text-slate-100',
}

/** Renkli metin: istatistik değerleri. */
export const toneText: Record<Tone, string> = {
  indigo: 'text-indigo-700 dark:text-indigo-300',
  violet: 'text-violet-700 dark:text-violet-300',
  fuchsia: 'text-fuchsia-700 dark:text-fuchsia-300',
  pink: 'text-pink-700 dark:text-pink-300',
  rose: 'text-rose-700 dark:text-rose-300',
  orange: 'text-orange-700 dark:text-orange-300',
  amber: 'text-amber-700 dark:text-amber-300',
  lime: 'text-lime-700 dark:text-lime-300',
  emerald: 'text-emerald-700 dark:text-emerald-300',
  teal: 'text-teal-700 dark:text-teal-300',
  sky: 'text-sky-700 dark:text-sky-300',
  blue: 'text-blue-700 dark:text-blue-300',
  slate: 'text-slate-700 dark:text-slate-300',
}

/** Ada göre dağıtılan tonlar (gri hariç): aynı ders ya da kişi hep aynı renkte görünür. */
const palette: Tone[] = [
  'indigo',
  'violet',
  'sky',
  'emerald',
  'amber',
  'pink',
  'teal',
  'orange',
  'fuchsia',
  'blue',
  'rose',
  'lime',
]

/** toneFor, bir anahtarı (ders kodu, kişi adı) kararlı biçimde bir tona eşler. */
export function toneFor(key: string): Tone {
  let h = 0
  for (let i = 0; i < key.length; i++) {
    h = (h * 31 + key.charCodeAt(i)) >>> 0
  }
  return palette[h % palette.length]!
}
