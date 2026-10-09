import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { fieldErrors } from '@/shared/api/errors'
import {
  useAssignRoleMutation,
  useEndRoleAssignmentMutation,
  useListRolesQuery,
  useResetUserMfaMutation,
  useSetUserStatusMutation,
  type Assignment,
  type Profile,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Button } from '@/shared/ui/button'
import { Field } from '@/shared/ui/field'
import { FormDialog } from '@/shared/ui/form-dialog'
import { Input } from '@/shared/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Textarea } from '@/shared/ui/textarea'

import { useRoleName } from '@/features/account/scope'

import { useAccountErrors } from './accountErrors'
import { OrgPicker } from '@/features/org/OrgPicker'
import { emptyOrg, type OrgSelection } from '@/features/org/orgSelection'

function ReasonField({
  value,
  onChange,
  error,
}: {
  value: string
  onChange: (v: string) => void
  error?: string
}) {
  const { t } = useTranslation()
  return (
    <Field label={t('common.reason')} hint={t('common.reasonHint')} error={error}>
      {(p) => (
        <Textarea {...p} value={value} onChange={(e) => onChange(e.target.value)} maxLength={500} rows={3} />
      )}
    </Field>
  )
}

export function StatusDialog({ user, onClose }: { user: Profile; onClose: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const errors = useAccountErrors()
  const [setStatus, { error, isLoading }] = useSetUserStatusMutation()
  const [status, setValue] = useState<'ACTIVE' | 'SUSPENDED' | 'DISABLED'>(
    user.status === 'ACTIVE' ? 'SUSPENDED' : 'ACTIVE',
  )
  const [reason, setReason] = useState('')

  return (
    <FormDialog
      title={t('userDetail.statusDialog.title')}
      description={t('userDetail.statusDialog.description')}
      error={error ? message(error, errors) : null}
      onClose={onClose}
      onSubmit={() =>
        void setStatus({ id: user.id, body: { status, reason: reason.trim() } }).then((res) => {
          if (!res.error) {
            toast.success(t('userDetail.statusDialog.success'))
            onClose()
          }
        })
      }
      submit={
        <Button type="submit" loading={isLoading} disabled={!reason.trim() || status === user.status}>
          {t('common.save')}
        </Button>
      }
    >
      <Field label={t('userDetail.statusDialog.newStatus')}>
        {(p) => (
          <Select value={status} onValueChange={(v) => setValue(v as typeof status)}>
            <SelectTrigger {...p} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(['ACTIVE', 'SUSPENDED', 'DISABLED'] as const).map((s) => (
                <SelectItem key={s} value={s}>
                  {t(`status.${s}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </Field>
      <ReasonField value={reason} onChange={setReason} />
    </FormDialog>
  )
}

export function ResetMfaDialog({ userId, onClose }: { userId: string; onClose: () => void }) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const errors = useAccountErrors()
  const [resetMfa, { error, isLoading }] = useResetUserMfaMutation()
  const [reason, setReason] = useState('')
  return (
    <FormDialog
      title={t('userDetail.mfaDialog.title')}
      description={t('userDetail.mfaDialog.description')}
      error={error ? message(error, errors) : null}
      onClose={onClose}
      onSubmit={() =>
        void resetMfa({ id: userId, body: { reason: reason.trim() } }).then((res) => {
          if (!res.error) {
            toast.success(t('userDetail.mfaDialog.success'))
            onClose()
          }
        })
      }
      submit={
        <Button type="submit" variant="destructive" loading={isLoading} disabled={!reason.trim()}>
          {t('userDetail.actions.resetMfa')}
        </Button>
      }
    >
      <ReasonField value={reason} onChange={setReason} />
    </FormDialog>
  )
}

/** toISO, datetime-local girdisini (yerel saat) RFC 3339'a çevirir. Boşsa undefined. */
function toISO(local: string): string | undefined {
  return local ? new Date(local).toISOString() : undefined
}

export function AssignRoleDialog({ userId, onClose }: { userId: string; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const message = useErrorMessage()
  const errors = useAccountErrors()
  const roles = useListRolesQuery()
  const [assign, { error, isLoading }] = useAssignRoleMutation()
  const [role, setRole] = useState('')
  const [org, setOrg] = useState<OrgSelection>(emptyOrg)
  const [validFrom, setValidFrom] = useState('')
  const [validUntil, setValidUntil] = useState('')
  const [reason, setReason] = useState('')

  const def = roles.data?.items.find((r) => r.code === role)
  const scopeType = def?.scope_type
  const scopeId =
    scopeType === 'FACULTY'
      ? org.facultyId
      : scopeType === 'DEPARTMENT'
        ? org.departmentId
        : scopeType === 'PROGRAM'
          ? org.programId
          : ''
  const needsScope = scopeType === 'FACULTY' || scopeType === 'DEPARTMENT' || scopeType === 'PROGRAM'
  const fields = fieldErrors(error)
  const general = error && Object.keys(fields).length === 0 ? message(error, errors) : null

  return (
    <FormDialog
      title={t('userDetail.roles.assignDialog.title')}
      description={t('userDetail.roles.assignDialog.description')}
      error={general}
      onClose={onClose}
      onSubmit={() => {
        const from = toISO(validFrom)
        const until = toISO(validUntil)
        void assign({
          id: userId,
          body: {
            role,
            reason: reason.trim(),
            ...(scopeId && { scope_id: scopeId }),
            ...(from && { valid_from: from }),
            ...(until && { valid_until: until }),
          },
        }).then((res) => {
          if (!res.error) {
            toast.success(t('userDetail.roles.assignDialog.success'))
            onClose()
          }
        })
      }}
      submit={
        <Button
          type="submit"
          loading={isLoading}
          disabled={!role || !reason.trim() || (needsScope && !scopeId)}
        >
          {t('userDetail.roles.assignDialog.submit')}
        </Button>
      }
    >
      <Field label={t('userDetail.roles.assignDialog.role')} error={fields.role?.[0]?.message}>
        {(p) => (
          <Select
            value={role}
            onValueChange={(v) => {
              setRole(v)
              setOrg(emptyOrg)
            }}
          >
            <SelectTrigger {...p} className="w-full">
              <SelectValue placeholder={t('userDetail.roles.assignDialog.rolePlaceholder')} />
            </SelectTrigger>
            <SelectContent>
              {roles.data?.items.map((r) => (
                <SelectItem key={r.code} value={r.code}>
                  {i18n.language === 'en' ? r.name_en : r.name_tr}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </Field>
      {def?.description && <p className="-mt-2 text-sm text-muted-foreground">{def.description}</p>}
      {needsScope && (
        <OrgPicker
          depth={scopeType === 'FACULTY' ? 'faculty' : scopeType === 'DEPARTMENT' ? 'department' : 'program'}
          value={org}
          onChange={setOrg}
          error={fields.scope_id?.[0]?.message}
        />
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('userDetail.roles.assignDialog.validFrom')} error={fields.valid_from?.[0]?.message}>
          {(p) => (
            <Input
              {...p}
              type="datetime-local"
              value={validFrom}
              onChange={(e) => setValidFrom(e.target.value)}
            />
          )}
        </Field>
        <Field label={t('userDetail.roles.assignDialog.validUntil')} error={fields.valid_until?.[0]?.message}>
          {(p) => (
            <Input
              {...p}
              type="datetime-local"
              value={validUntil}
              onChange={(e) => setValidUntil(e.target.value)}
            />
          )}
        </Field>
      </div>
      <ReasonField value={reason} onChange={setReason} error={fields.reason?.[0]?.message} />
    </FormDialog>
  )
}

export function EndRoleDialog({
  userId,
  assignment,
  onClose,
}: {
  userId: string
  assignment: Assignment
  onClose: () => void
}) {
  const { t } = useTranslation()
  const message = useErrorMessage()
  const errors = useAccountErrors()
  const [end, { error, isLoading }] = useEndRoleAssignmentMutation()
  const roleName = useRoleName()
  const [reason, setReason] = useState('')
  return (
    <FormDialog
      title={t('userDetail.roles.endDialog.title')}
      description={t('userDetail.roles.endDialog.description', { role: roleName(assignment) })}
      error={error ? message(error, errors) : null}
      onClose={onClose}
      onSubmit={() =>
        void end({ id: userId, assignment: assignment.id, body: { reason: reason.trim() } }).then((res) => {
          if (!res.error) {
            toast.success(t('userDetail.roles.endDialog.success'))
            onClose()
          }
        })
      }
      submit={
        <Button type="submit" variant="destructive" loading={isLoading} disabled={!reason.trim()}>
          {t('userDetail.roles.end')}
        </Button>
      }
    >
      <ReasonField value={reason} onChange={setReason} />
    </FormDialog>
  )
}
