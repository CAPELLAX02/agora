import {
  AlarmClockIcon,
  BookOpenCheckIcon,
  ClipboardCheckIcon,
  MegaphoneIcon,
  MessageCircleIcon,
  MonitorPlayIcon,
  ShieldCheckIcon,
  type LucideIcon,
} from 'lucide-react'

import type { Notification, NotificationCategory } from '@/shared/api/generated'
import type { Tone } from '@/shared/ui/tone'

/** categoryMeta, bildirim kategorisinin ikonu ve rengidir. */
export const categoryMeta: Record<NotificationCategory, { icon: LucideIcon; tone: Tone }> = {
  SECURITY: { icon: ShieldCheckIcon, tone: 'rose' },
  REGISTRATION: { icon: ClipboardCheckIcon, tone: 'indigo' },
  GRADES: { icon: BookOpenCheckIcon, tone: 'pink' },
  COURSES: { icon: MonitorPlayIcon, tone: 'emerald' },
  ANNOUNCEMENTS: { icon: MegaphoneIcon, tone: 'amber' },
  MESSAGES: { icon: MessageCircleIcon, tone: 'sky' },
  REMINDERS: { icon: AlarmClockIcon, tone: 'violet' },
}

/** notificationText, bildirimin arayüz dilindeki başlığı ve metnidir. */
export function notificationText(n: Notification, en: boolean) {
  return en ? { title: n.title_en, body: n.body_en } : { title: n.title_tr, body: n.body_tr }
}
