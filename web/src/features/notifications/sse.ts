/** SSEEvent, olay akışından (text/event-stream) okunan bir olaydır. */
export type SSEEvent = { id: string | null; event: string; data: string }

/**
 * createSSEParser, parça parça gelen olay akışı metnini olaylara ayırır: boş satır olayı bitirir,
 * ":" ile başlayan satır yorumdur (kalp atışı). Olay adı verilmemişse "message".
 */
export function createSSEParser(onEvent: (e: SSEEvent) => void) {
  let buffer = ''
  let id: string | null = null
  let event = ''
  let data: string[] = []
  const line = (l: string) => {
    if (l === '') {
      if (data.length > 0 || event) {
        onEvent({ id, event: event || 'message', data: data.join('\n') })
      }
      id = null
      event = ''
      data = []
      return
    }
    if (l.startsWith(':')) {
      return
    }
    const colon = l.indexOf(':')
    const field = colon < 0 ? l : l.slice(0, colon)
    let value = colon < 0 ? '' : l.slice(colon + 1)
    if (value.startsWith(' ')) {
      value = value.slice(1)
    }
    if (field === 'id') {
      id = value
    } else if (field === 'event') {
      event = value
    } else if (field === 'data') {
      data.push(value)
    }
  }
  return {
    feed(chunk: string) {
      buffer += chunk
      let nl: number
      while ((nl = buffer.indexOf('\n')) >= 0) {
        line(buffer.slice(0, nl).replace(/\r$/, ''))
        buffer = buffer.slice(nl + 1)
      }
    },
  }
}
