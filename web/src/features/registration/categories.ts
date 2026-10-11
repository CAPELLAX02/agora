import {
  CalendarCheckIcon,
  CircleArrowUpIcon,
  RotateCcwIcon,
  SparklesIcon,
  TrendingUpIcon,
  type LucideIcon,
} from 'lucide-react'

import type { RegistrationOffer } from '@/shared/api/generated'
import type { Tone } from '@/shared/ui/tone'

type Category = RegistrationOffer['items'][number]['category']

/** Kategorilerin rengi ve ikonu: öğrenci listede nerede olduğunu renkten anlar. */
export const categoryStyle: Record<Category, { tone: Tone; icon: LucideIcon }> = {
  REPEAT: { tone: 'rose', icon: RotateCcwIcon },
  CURRENT: { tone: 'sky', icon: CalendarCheckIcon },
  ELECTIVE: { tone: 'violet', icon: SparklesIcon },
  UPPER: { tone: 'amber', icon: CircleArrowUpIcon },
  UPGRADE: { tone: 'emerald', icon: TrendingUpIcon },
}
