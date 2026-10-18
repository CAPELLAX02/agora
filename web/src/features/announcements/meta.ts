import {
  ClipboardListIcon,
  GraduationCapIcon,
  InfoIcon,
  PartyPopperIcon,
  SirenIcon,
  type LucideIcon,
} from 'lucide-react'

import type { Announcement, AnnouncementAudience, AnnouncementType } from '@/shared/api/generated'
import type { Tone } from '@/shared/ui/tone'

/** typeMeta, duyuru türünün ikonu ve rengidir. */
export const typeMeta: Record<AnnouncementType, { icon: LucideIcon; tone: Tone }> = {
  GENERAL: { icon: InfoIcon, tone: 'sky' },
  ACADEMIC: { icon: GraduationCapIcon, tone: 'indigo' },
  EXAM: { icon: ClipboardListIcon, tone: 'violet' },
  URGENT: { icon: SirenIcon, tone: 'rose' },
  EVENT: { icon: PartyPopperIcon, tone: 'emerald' },
}

export const announcementTypes: AnnouncementType[] = ['GENERAL', 'ACADEMIC', 'EXAM', 'URGENT', 'EVENT']

/** Phase, duyurunun yayımlayana gösterilen durumudur (yayın penceresiyle). */
export type Phase = 'DRAFT' | 'SCHEDULED' | 'LIVE' | 'EXPIRED' | 'ARCHIVED'

export function phaseOf(a: Announcement, now = new Date()): Phase {
  if (a.status !== 'PUBLISHED') {
    return a.status
  }
  if (new Date(a.publish_from) > now) {
    return 'SCHEDULED'
  }
  if (a.publish_until && new Date(a.publish_until) <= now) {
    return 'EXPIRED'
  }
  return 'LIVE'
}

export const phaseBadge: Record<Phase, 'muted' | 'info' | 'success' | 'secondary' | 'warning'> = {
  DRAFT: 'muted',
  SCHEDULED: 'info',
  LIVE: 'success',
  EXPIRED: 'secondary',
  ARCHIVED: 'warning',
}

/** audienceKey, hedefin seçicide ve listede ayırt edici anahtarıdır. */
export const audienceKey = (a: AnnouncementAudience) => `${a.scope_type}:${a.scope_id ?? ''}:${a.people}`
