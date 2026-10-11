import {
  BookOpenIcon,
  ClipboardCheckIcon,
  FileTextIcon,
  PenLineIcon,
  SparklesIcon,
  UserPlusIcon,
  type LucideIcon,
} from 'lucide-react'

import type { CalendarEventType } from '@/shared/api/generated'
import type { Tone } from '@/shared/ui/tone'

/** Takvim olayı kategorilerinin rengi ve ikonu. */
export const eventCategory: Record<CalendarEventType['category'], { tone: Tone; icon: LucideIcon }> = {
  REGISTRATION: { tone: 'violet', icon: ClipboardCheckIcon },
  INSTRUCTION: { tone: 'blue', icon: BookOpenIcon },
  EXAM: { tone: 'rose', icon: FileTextIcon },
  GRADING: { tone: 'fuchsia', icon: PenLineIcon },
  ADMISSION: { tone: 'sky', icon: UserPlusIcon },
  OTHER: { tone: 'amber', icon: SparklesIcon },
}
