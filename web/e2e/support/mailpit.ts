import { expect } from '@playwright/test'

const mailpit = process.env.MAILPIT_URL ?? 'http://localhost:8025'

type Summary = { ID: string; Created: string; To: { Address: string }[] }

/**
 * linkFromLatestEmail, adrese gelen ve since'ten sonra oluşturulan son e-postadaki
 * Agora bağlantısını döndürür. E-postalar worker üzerinden gider: birkaç saniye beklenir.
 */
export async function linkFromLatestEmail(to: string, since: Date): Promise<string> {
  let link = ''
  await expect
    .poll(
      async () => {
        const res = await fetch(`${mailpit}/api/v1/search?query=${encodeURIComponent(`to:${to}`)}&limit=5`)
        const { messages } = (await res.json()) as { messages: Summary[] }
        const fresh = messages.find((m) => new Date(m.Created) >= since)
        if (!fresh) {
          return ''
        }
        const msg = (await (await fetch(`${mailpit}/api/v1/message/${fresh.ID}`)).json()) as { Text: string }
        link = /https?:\/\/\S+#token=[\w-]+/.exec(msg.Text)?.[0] ?? ''
        return link
      },
      { timeout: 20_000, intervals: [500] },
    )
    .not.toBe('')
  return link
}
