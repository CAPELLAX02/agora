import type { Conversation } from '@/shared/api/generated'

/** splitName, "Ad Soyad" biçimindeki adı avatar için ikiye ayırır. */
export function splitName(name: string): [string, string] {
  const i = name.lastIndexOf(' ')
  return i < 0 ? [name, ''] : [name.slice(0, i), name.slice(i + 1)]
}

/** conversationTitle, konuşmanın gösterilen adıdır: birebirde karşı taraf. */
export function conversationTitle(c: Conversation) {
  return c.kind === 'DIRECT' ? (c.peer?.name ?? '') : c.title
}
