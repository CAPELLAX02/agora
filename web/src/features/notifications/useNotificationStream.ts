import { useEffect, useRef } from 'react'
import { useStore } from 'react-redux'

import type { AuthState } from '@/features/auth/authSlice'
import { refreshSession } from '@/shared/api/baseQuery'
import type { Notification } from '@/shared/api/generated'

import { createSSEParser } from './sse'

const url = '/api/v1/me/notifications/stream'
// Bağlantı koparsa yeniden bağlanma aralığı; sunucu akışı açamıyorsa (sınır, kesinti) daha seyrek.
const retryDelay = 5_000
const unavailableDelay = 60_000
// Sekme bu kadar gizli kalırsa akış kapatılır, görünür olunca kaçırılanlarla yeniden açılır.
const hiddenGrace = 60_000

function open(token: string | null, lastId: string | null, signal: AbortSignal) {
  const headers: Record<string, string> = { Accept: 'text/event-stream', 'X-Agora-Client': 'web' }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  if (lastId) {
    headers['Last-Event-ID'] = lastId
  }
  return fetch(url, { headers, signal, credentials: 'same-origin' })
}

/** StreamHandlers, canlı akışın olaylarıdır: yeni bildirim ve konuşmada yeni ileti. */
export type StreamHandlers = {
  onNotification: (n: Notification) => void
  onMessage?: (conversationId: string) => void
}

/**
 * useNotificationStream, canlı akışı açık tutar: her yeni bildirimde onNotification, konuşmada yeni
 * ileti olunca onMessage çağrılır. EventSource yerine fetch akışı: access token Authorization
 * başlığında gider. Süresi dolmuş token sessizce yenilenir; kopan akış son alınan bildirim
 * kimliğiyle (Last-Event-ID) yeniden açılır, böylece aradaki bildirimler kaçmaz.
 */
export function useNotificationStream(handlers: StreamHandlers) {
  const store = useStore<{ auth: AuthState }>()
  const handler = useRef(handlers)
  useEffect(() => {
    handler.current = handlers
  })

  useEffect(() => {
    let stopped = false
    let controller: AbortController | null = null
    let lastId: string | null = null
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let hideTimer: ReturnType<typeof setTimeout> | undefined

    const schedule = (ms: number) => {
      clearTimeout(retryTimer)
      if (!stopped && !document.hidden) {
        retryTimer = setTimeout(() => void connect(), ms)
      }
    }

    const connect = async () => {
      if (stopped || controller) {
        return
      }
      const current = new AbortController()
      controller = current
      let delay = retryDelay
      try {
        let res = await open(store.getState().auth.accessToken, lastId, current.signal)
        if (res.status === 401 && (await refreshSession(store.dispatch, { retry: true })) === 'ok') {
          res = await open(store.getState().auth.accessToken, lastId, current.signal)
        }
        if (!res.ok || !res.body) {
          delay = res.status === 401 ? unavailableDelay * 5 : unavailableDelay
          return
        }
        const parser = createSSEParser((e) => {
          if (e.id) {
            lastId = e.id
          }
          if (e.event === 'notification') {
            handler.current.onNotification(JSON.parse(e.data) as Notification)
          } else if (e.event === 'message') {
            handler.current.onMessage?.((JSON.parse(e.data) as { conversation_id: string }).conversation_id)
          }
        })
        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        for (;;) {
          const { value, done } = await reader.read()
          if (done) {
            break
          }
          parser.feed(decoder.decode(value, { stream: true }))
        }
      } catch {
        // Ağ hatası ya da kapatma: aşağıda yeniden denenir (kapatıldıysa denenmez).
      } finally {
        if (controller === current) {
          controller = null
        }
        if (!current.signal.aborted) {
          schedule(delay)
        }
      }
    }

    const onVisibility = () => {
      if (document.hidden) {
        hideTimer = setTimeout(() => {
          controller?.abort()
          controller = null
        }, hiddenGrace)
        return
      }
      clearTimeout(hideTimer)
      if (!controller) {
        clearTimeout(retryTimer)
        void connect()
      }
    }

    document.addEventListener('visibilitychange', onVisibility)
    void connect()
    return () => {
      stopped = true
      clearTimeout(retryTimer)
      clearTimeout(hideTimer)
      document.removeEventListener('visibilitychange', onVisibility)
      controller?.abort()
    }
  }, [store])
}
