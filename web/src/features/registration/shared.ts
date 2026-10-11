import type { ParseKeys } from 'i18next'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { problemOf, type AnyError } from '@/shared/api/errors'
import type { RegistrationSection, RuleProblem, RuleViolation, ScheduleEntry } from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'

/** newKey, değiştirici istek için yeni bir Idempotency-Key üretir (yeniden denemede aynısı gönderilir). */
export function newKey(): string {
  return crypto.randomUUID()
}

/**
 * toEntries, şubelerin oturumlarını haftalık program bileşeninin beklediği biçime çevirir.
 * Derslik "BİNA/DERSLİK" biçiminde gelir.
 */
export function toEntries(sections: RegistrationSection[]): ScheduleEntry[] {
  return sections.flatMap((s) =>
    s.slots.map((slot, i) => {
      const [building = '', code = ''] = (slot.classroom ?? '').split('/')
      return {
        id: `${s.id}-${i}`,
        day_of_week: slot.day,
        start_time: slot.start,
        end_time: slot.end,
        session_type: slot.kind,
        classroom: slot.classroom ? { id: '', code, name: code, building_code: building, capacity: 0 } : null,
        term_id: '',
        offering_id: s.offering_id,
        section_id: s.id,
        section_code: s.section_code,
        course: {
          id: s.course.id,
          code: s.course.code,
          name_tr: s.course.name_tr,
          name_en: s.course.name_en,
        },
        instructors: [],
      }
    }),
  )
}

type Code = RuleViolation['code']

const codeKey: Record<Code, ParseKeys> = {
  HOLD: 'registration.rules.HOLD',
  PROGRAM_NOT_ACTIVE: 'registration.rules.PROGRAM_NOT_ACTIVE',
  NO_CURRICULUM: 'registration.rules.NO_CURRICULUM',
  EMPTY_BASKET: 'registration.rules.EMPTY_BASKET',
  DUPLICATE_COURSE: 'registration.rules.DUPLICATE_COURSE',
  NOT_IN_CURRICULUM: 'registration.rules.NOT_IN_CURRICULUM',
  PREREQUISITE: 'registration.rules.PREREQUISITE',
  SCHEDULE_CONFLICT: 'registration.rules.SCHEDULE_CONFLICT',
  ECTS_LIMIT: 'registration.rules.ECTS_LIMIT',
  FIRST_YEAR_COMPULSORY: 'registration.rules.FIRST_YEAR_COMPULSORY',
  REPEAT_PRIORITY: 'registration.rules.REPEAT_PRIORITY',
  MAX_DURATION: 'registration.rules.MAX_DURATION',
  REPEAT_UPGRADE: 'registration.rules.REPEAT_UPGRADE',
  ELECTIVE_SLOT_EXCEEDED: 'registration.rules.ELECTIVE_SLOT_EXCEEDED',
}

const list = (v: unknown) => (Array.isArray(v) ? v.join(', ') : typeof v === 'string' ? v : '')

/**
 * useViolationText, kural ihlalinin metnidir. Türkçede sunucunun ayrıntılı mesajı gösterilir;
 * diğer dillerde kural kodunun çevirisi, ihlalin parametreleriyle (ders kodları, AKTS...).
 */
export function useViolationText() {
  const { t, i18n } = useTranslation()
  return useCallback(
    (v: RuleViolation): string => {
      if (i18n.language.startsWith('tr')) {
        return v.message
      }
      const p = v.params
      return t(codeKey[v.code], {
        limit: p.limit ?? '',
        total: p.total ?? '',
        courses: list(p.courses ?? p.missing ?? p.sections),
        remaining: p.remaining ?? '',
        holds: list(p.holds),
        status: p.status ?? '',
        slot: p.slot ?? '',
      })
    },
    [t, i18n.language],
  )
}

/** ruleViolations, 422 RULE_VIOLATION yanıtındaki kural ihlalleridir. */
export function ruleViolations(error: AnyError): RuleViolation[] {
  const p = problemOf(error) as RuleProblem | undefined
  return p?.code === 'RULE_VIOLATION' ? p.violations : []
}

/**
 * useRegistrationError, ders seçme işlemlerinin hatasını metne çevirir: kural ihlalinde
 * ilk hata, diğer kodlarda ders seçmeye özgü açıklamalar.
 */
export function useRegistrationError() {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const text = useViolationText()
  return useCallback(
    (error: AnyError): string => {
      const vs = ruleViolations(error)
      const first = vs.find((v) => v.severity === 'ERROR') ?? vs[0]
      if (first) {
        return text(first)
      }
      return message(error, {
        SECTION_FULL: t('registration.errors.SECTION_FULL'),
        QUOTA_FULL: t('registration.errors.QUOTA_FULL'),
        SECTION_NOT_FOR_PROGRAM: t('registration.errors.SECTION_NOT_FOR_PROGRAM'),
        SECTION_NOT_OPEN: t('registration.errors.SECTION_NOT_OPEN'),
        WINDOW_CLOSED: t('registration.errors.WINDOW_CLOSED'),
        REGISTRATION_LOCKED: t('registration.errors.REGISTRATION_LOCKED'),
        INVALID_REGISTRATION_STATE: t('registration.errors.INVALID_REGISTRATION_STATE'),
        VERSION_MISMATCH: t('registration.errors.VERSION_MISMATCH'),
        SAME_SECTION: t('registration.errors.SAME_SECTION'),
        NO_ACTIVE_TERM: t('registration.errors.NO_ACTIVE_TERM'),
        ADD_DROP_OPEN: t('registration.errors.ADD_DROP_OPEN'),
        REGISTRATION_NOT_APPROVED: t('registration.errors.REGISTRATION_NOT_APPROVED'),
        INVALID_ADD_DROP_STATE: t('registration.errors.INVALID_ADD_DROP_STATE'),
      })
    },
    [t, message, text],
  )
}

type Result<T> = { data?: T; error?: AnyError }

/**
 * useRun, bir ders seçme işlemini bekler: hata olursa metni toast olarak gösterir ve false
 * döner, başarıda ok'u yanıtla çağırır ve true döner.
 */
export function useRun() {
  const errorText = useRegistrationError()
  return useCallback(
    async <T>(pending: PromiseLike<Result<T>>, ok: (data: T) => void): Promise<boolean> => {
      const res = await pending
      if (res.error !== undefined) {
        toast.error(errorText(res.error))
        return false
      }
      ok(res.data as T) // hata yoksa veri vardır
      return true
    },
    [errorText],
  )
}

/**
 * shownViolations, sepette gösterilecek kural sonuçlarıdır. Boş sepette "sepet boş" hatası
 * gösterilmez ama ders seçmeyi baştan engelleyenler (kayıt engeli, aktif olmayan kayıt,
 * ders planı yokluğu) gösterilir: öğrenci ders eklemeye çalışmadan nedenini görür.
 */
export function shownViolations(r: {
  items: unknown[]
  evaluation: { violations: RuleViolation[] }
}): RuleViolation[] {
  return r.items.length > 0
    ? r.evaluation.violations
    : r.evaluation.violations.filter((v) => v.code !== 'EMPTY_BASKET')
}
