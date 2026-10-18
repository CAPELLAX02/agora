import { describe, expect, it } from 'vitest'

import { createSSEParser, type SSEEvent } from './sse'

describe('createSSEParser', () => {
  it('parçalara bölünmüş olayları, kimliklerini ve yorumları ayırır', () => {
    const events: SSEEvent[] = []
    const p = createSSEParser((e) => events.push(e))
    p.feed('retry: 5000\n: bağlandı\n\nid: 7\nevent: notifi')
    p.feed('cation\ndata: {"id":7}\n\n: ping\n\n')
    p.feed('data: satır 1\r\ndata: satır 2\r\n\r\n')
    expect(events).toEqual([
      { id: '7', event: 'notification', data: '{"id":7}' },
      { id: null, event: 'message', data: 'satır 1\nsatır 2' },
    ])
  })
})
