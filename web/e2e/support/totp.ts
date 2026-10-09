import { createHmac } from 'node:crypto'

const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'

function base32(secret: string): Buffer {
  let bits = ''
  for (const c of secret.replace(/\s|=/g, '').toUpperCase()) {
    bits += alphabet.indexOf(c).toString(2).padStart(5, '0')
  }
  const bytes = bits.match(/.{8}/g) ?? []
  return Buffer.from(bytes.map((b) => parseInt(b, 2)))
}

/** totp, sırrın verilen adımdaki 6 haneli kodudur (RFC 6238, SHA-1, 30 sn). */
export function totp(secret: string, step = Math.floor(Date.now() / 30_000)): string {
  const msg = Buffer.alloc(8)
  msg.writeBigUInt64BE(BigInt(step))
  const mac = createHmac('sha1', base32(secret)).update(msg).digest()
  const offset = mac[mac.length - 1] & 0x0f
  const value = (mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000
  return value.toString().padStart(6, '0')
}

/**
 * freshTotp, daha önce kullanılmamış bir kod döndürür. Sunucu aynı adımın kodunu ikinci
 * kez kabul etmez: son kullanılan adımdan sonraki adım gelene kadar bekler.
 */
export async function freshTotp(secret: string, lastStep: { value: number }): Promise<string> {
  let step = Math.floor(Date.now() / 30_000)
  while (step <= lastStep.value) {
    await new Promise((r) => setTimeout(r, 1000))
    step = Math.floor(Date.now() / 30_000)
  }
  lastStep.value = step
  return totp(secret, step)
}
