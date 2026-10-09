import { describe, expect, it } from 'vitest'

import enJson from './locales/en.json'
import trJson from './locales/tr.json'

type Tree = { [key: string]: string | Tree }

const tr: Tree = trJson
const en: Tree = enJson

function keys(tree: Tree, prefix = ''): string[] {
  return Object.entries(tree).flatMap(([k, v]) =>
    typeof v === 'string' ? [prefix + k] : keys(v, `${prefix}${k}.`),
  )
}

function lookup(tree: Tree, key: string): string {
  let node: string | Tree | undefined = tree
  for (const part of key.split('.')) {
    node = typeof node === 'object' ? node[part] : undefined
  }
  return typeof node === 'string' ? node : ''
}

const placeholders = (s: string) => [...s.matchAll(/{{(\w+)}}/g)].map((m) => m[1]).sort()

describe('çeviriler', () => {
  // Kodda kullanılan anahtarlar tip kontrolüyle Türkçe dosyaya karşı denetlenir.
  // Burada İngilizce dosyanın aynı anahtarlara sahip olduğu denetlenir.
  it('Türkçe ve İngilizce aynı anahtarlara sahip', () => {
    expect(keys(en).sort()).toEqual(keys(tr).sort())
  })

  it('boş çeviri yok', () => {
    for (const k of keys(tr)) {
      expect(lookup(tr, k).trim(), `tr: ${k}`).not.toBe('')
      expect(lookup(en, k).trim(), `en: ${k}`).not.toBe('')
    }
  })

  it('yer tutucular iki dilde aynı', () => {
    for (const k of keys(tr)) {
      expect(placeholders(lookup(en, k)), k).toEqual(placeholders(lookup(tr, k)))
    }
  })
})
