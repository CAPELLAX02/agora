import {
  FileIcon,
  FolderIcon,
  LinkIcon,
  MessagesSquareIcon,
  NotebookPenIcon,
  StickyNoteIcon,
  TextIcon,
  type LucideIcon,
} from 'lucide-react'

import type {
  Activity,
  ActivityInput,
  ActivityType,
  AssignmentInput,
  AssignmentSettings,
} from '@/shared/api/generated'
import type { Tone } from '@/shared/ui/tone'

/** activityMeta, etkinlik türünün ikonu ve rengidir. */
export const activityMeta: Record<ActivityType, { icon: LucideIcon; tone: Tone }> = {
  RESOURCE: { icon: FileIcon, tone: 'sky' },
  FOLDER: { icon: FolderIcon, tone: 'amber' },
  URL: { icon: LinkIcon, tone: 'teal' },
  PAGE: { icon: StickyNoteIcon, tone: 'violet' },
  LABEL: { icon: TextIcon, tone: 'slate' },
  ASSIGNMENT: { icon: NotebookPenIcon, tone: 'rose' },
  FORUM: { icon: MessagesSquareIcon, tone: 'emerald' },
}

/** editableTypes, ders alanında etkinlik penceresiyle eklenen türlerdir (forum kendi akışıyla). */
export const editableTypes = ['ASSIGNMENT', 'RESOURCE', 'FOLDER', 'URL', 'PAGE', 'LABEL'] as const
export type EditableType = (typeof editableTypes)[number]

/** toLocalInput, ISO zamanı datetime-local alanının değerine çevirir (yerel saat). */
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) {
    return ''
  }
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** fromLocalInput, datetime-local değerini ISO zamana çevirir; boşsa null. */
export function fromLocalInput(v: string): string | null {
  return v ? new Date(v).toISOString() : null
}

/** hostOf, bağlantının ana bilgisayarıdır (gösterim için). */
export function hostOf(url: string | null): string {
  try {
    return url ? new URL(url).host : ''
  } catch {
    return ''
  }
}

/** inputOf, etkinliğin güncelleme gövdesidir; sayfanın gövdesi ayrıntıdan verilir. */
export function inputOf(a: Activity, body?: string): ActivityInput {
  return {
    type: a.type as ActivityInput['type'],
    title: a.title,
    description_html: a.description_html,
    is_visible: a.is_visible,
    available_from: a.available_from,
    available_until: a.available_until,
    file_ids: a.files.map((f) => f.id),
    url: a.url ?? undefined,
    open_in_new_tab: a.open_in_new_tab ?? undefined,
    body_html: body ?? a.body_html ?? undefined,
    assignment: a.assignment ? assignmentInputOf(a.assignment) : undefined,
  }
}

/** assignmentInputOf, ödevin ayarlarının yazılabilir alanlarıdır. */
export function assignmentInputOf(s: AssignmentSettings): AssignmentInput {
  return {
    opens_at: s.opens_at,
    due_at: s.due_at,
    cutoff_at: s.cutoff_at,
    max_attempts: s.max_attempts,
    submission_types: s.submission_types,
    max_files: s.max_files,
    max_file_size_bytes: s.max_file_size_bytes,
    allowed_extensions: s.allowed_extensions,
    max_score: s.max_score,
    late_policy: s.late_policy,
    late_penalty_pct_per_day: s.late_penalty_pct_per_day,
  }
}
