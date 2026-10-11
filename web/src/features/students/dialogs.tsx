import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ifMatch } from '@/shared/api/etag'
import {
  useChangeStudentProgramStatusMutation,
  usePlaceHoldMutation,
  useReleaseHoldMutation,
  type EnrollmentStatus,
  type Hold,
  type HoldBlock,
  type HoldType,
  type StudentProgram,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Button } from '@/shared/ui/button'
import { Checkbox } from '@/shared/ui/checkbox'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Textarea } from '@/shared/ui/textarea'

const holdTypes: HoldType[] = ['TUITION_DEBT', 'DISCIPLINE_SUSPENSION', 'MISSING_DOCUMENT', 'LIBRARY_DEBT']
const blocks: HoldBlock[] = ['REGISTRATION', 'DOCUMENT', 'GRADUATION']
const defaults: Record<HoldType, HoldBlock[]> = {
  TUITION_DEBT: ['REGISTRATION', 'DOCUMENT'],
  DISCIPLINE_SUSPENSION: ['REGISTRATION'],
  MISSING_DOCUMENT: ['GRADUATION'],
  LIBRARY_DEBT: ['DOCUMENT', 'GRADUATION'],
}

/** PlaceHoldDialog, program kaydına engel koyar: tür, kapattığı işlemler (türün varsayılanı) ve gerekçe. */
export function PlaceHoldDialog({ program, onClose }: { program: StudentProgram; onClose: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [place, { isLoading }] = usePlaceHoldMutation()
  const [type, setType] = useState<HoldType>('TUITION_DEBT')
  const [chosen, setChosen] = useState<HoldBlock[]>(defaults.TUITION_DEBT)
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)
  return (
    <FormDialog
      title={t('students.holds.place')}
      description={t('students.holds.placeFor', {
        name: `${program.student.first_name} ${program.student.last_name}`,
        program: program.program.code,
      })}
      error={error}
      onClose={onClose}
      onSubmit={async () => {
        if (!reason.trim() || chosen.length === 0) {
          setError(t('students.holds.required'))
          return
        }
        const res = await place({
          id: program.id,
          body: { hold_type: type, blocks: chosen, reason: reason.trim() },
        })
        if (res.error) {
          setError(message(res.error, { HOLD_EXISTS: t('students.holds.exists') }))
          return
        }
        toast.success(t('students.holds.placed'))
        onClose()
      }}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('students.holds.place')}
        </Button>
      }
    >
      <div className="space-y-2">
        <Label htmlFor="hold-type">{t('students.holds.type')}</Label>
        <Select
          value={type}
          onValueChange={(v) => {
            setType(v as HoldType)
            setChosen(defaults[v as HoldType])
          }}
        >
          <SelectTrigger id="hold-type" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {holdTypes.map((h) => (
              <SelectItem key={h} value={h}>
                {t(`students.holds.types.${h}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{t('students.holds.blocks')}</legend>
        {blocks.map((b) => (
          <div key={b} className="flex items-center gap-2">
            <Checkbox
              id={`block-${b}`}
              checked={chosen.includes(b)}
              onCheckedChange={(v) => setChosen(v ? [...chosen, b] : chosen.filter((c) => c !== b))}
            />
            <Label htmlFor={`block-${b}`} className="font-normal">
              {t(`students.holds.blockLabels.${b}`)}
            </Label>
          </div>
        ))}
      </fieldset>
      <div className="space-y-2">
        <Label htmlFor="hold-reason">{t('common.reason')}</Label>
        <Textarea
          id="hold-reason"
          value={reason}
          maxLength={500}
          onChange={(e) => setReason(e.target.value)}
        />
      </div>
    </FormDialog>
  )
}

/** ReleaseHoldDialog, engeli gerekçeyle kaldırır. */
export function ReleaseHoldDialog({ hold, onClose }: { hold: Hold; onClose: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [release, { isLoading }] = useReleaseHoldMutation()
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)
  return (
    <FormDialog
      title={t('students.holds.release')}
      description={t('students.holds.releaseFor', {
        type: t(`students.holds.types.${hold.hold_type}`),
        name: `${hold.student.first_name} ${hold.student.last_name}`,
      })}
      error={error}
      onClose={onClose}
      onSubmit={async () => {
        if (!reason.trim()) {
          setError(t('students.holds.required'))
          return
        }
        const res = await release({ id: hold.id, body: { reason: reason.trim() } })
        if (res.error) {
          setError(message(res.error))
          return
        }
        toast.success(t('students.holds.released'))
        onClose()
      }}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('students.holds.release')}
        </Button>
      }
    >
      <div className="space-y-2">
        <Label htmlFor="release-reason">{t('common.reason')}</Label>
        <Textarea
          id="release-reason"
          value={reason}
          maxLength={500}
          onChange={(e) => setReason(e.target.value)}
        />
      </div>
    </FormDialog>
  )
}

/** Durumdan geçilebilecek durumlar (sunucudaki kurallarla aynı). */
const transitions: Record<EnrollmentStatus, EnrollmentStatus[]> = {
  PREP: ['ACTIVE', 'WITHDRAWN', 'DISMISSED'],
  ACTIVE: ['FROZEN', 'SUSPENDED', 'GRADUATED', 'WITHDRAWN', 'DISMISSED'],
  FROZEN: ['ACTIVE', 'WITHDRAWN', 'DISMISSED'],
  SUSPENDED: ['ACTIVE', 'WITHDRAWN', 'DISMISSED'],
  GRADUATED: [],
  WITHDRAWN: [],
  DISMISSED: [],
}

/** StatusDialog, program kaydının durumunu gerekçe ve karar numarasıyla değiştirir. */
export function StatusDialog({ program, onClose }: { program: StudentProgram; onClose: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const [change, { isLoading }] = useChangeStudentProgramStatusMutation()
  const options = transitions[program.status]
  const [status, setStatus] = useState<EnrollmentStatus>(options[0] ?? 'ACTIVE')
  const [reason, setReason] = useState('')
  const [decision, setDecision] = useState('')
  const [graduatedOn, setGraduatedOn] = useState('')
  const [error, setError] = useState<string | null>(null)
  return (
    <FormDialog
      title={t('students.statusChange.title')}
      description={t('students.statusChange.description', { status: t(`students.status.${program.status}`) })}
      error={error}
      onClose={onClose}
      onSubmit={async () => {
        if (!reason.trim() || (status === 'GRADUATED' && !graduatedOn)) {
          setError(t('students.holds.required'))
          return
        }
        const res = await change({
          id: program.id,
          'If-Match': ifMatch(program.version),
          body: {
            status,
            reason: reason.trim(),
            decision_ref: decision.trim() || null,
            graduated_on: status === 'GRADUATED' ? graduatedOn : null,
          },
        })
        if (res.error) {
          setError(message(res.error))
          return
        }
        toast.success(t('students.statusChange.done'))
        onClose()
      }}
      submit={
        <Button type="submit" loading={isLoading}>
          {t('common.save')}
        </Button>
      }
    >
      <div className="space-y-2">
        <Label htmlFor="new-status">{t('students.statusChange.to')}</Label>
        <Select value={status} onValueChange={(v) => setStatus(v as EnrollmentStatus)}>
          <SelectTrigger id="new-status" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {options.map((s) => (
              <SelectItem key={s} value={s}>
                {t(`students.status.${s}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {status === 'GRADUATED' && (
        <div className="space-y-2">
          <Label htmlFor="graduated-on">{t('students.statusChange.graduatedOn')}</Label>
          <Input
            id="graduated-on"
            type="date"
            value={graduatedOn}
            onChange={(e) => setGraduatedOn(e.target.value)}
          />
        </div>
      )}
      <div className="space-y-2">
        <Label htmlFor="decision-ref">{t('students.statusChange.decisionRef')}</Label>
        <Input
          id="decision-ref"
          value={decision}
          maxLength={100}
          onChange={(e) => setDecision(e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="status-reason">{t('common.reason')}</Label>
        <Textarea
          id="status-reason"
          value={reason}
          maxLength={500}
          onChange={(e) => setReason(e.target.value)}
        />
      </div>
    </FormDialog>
  )
}
