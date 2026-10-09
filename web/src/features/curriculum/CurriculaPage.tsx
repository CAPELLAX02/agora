import { ArchiveIcon, CheckCircle2Icon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { toast } from 'sonner'

import { usePermissions } from '@/features/auth/usePermissions'
import { OrgPicker } from '@/features/org/OrgPicker'
import type { OrgSelection } from '@/features/org/orgSelection'
import {
  useActivateCurriculumMutation,
  useArchiveCurriculumMutation,
  useDeleteCurriculumItemMutation,
  useDeleteCurriculumMutation,
  useGetCurriculumQuery,
  useListProgramCurriculaQuery,
  type Curriculum,
  type CurriculumItem,
} from '@/shared/api/generated'
import type { AnyError } from '@/shared/api/errors'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { Card, CardContent } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { PageHeader } from '@/shared/ui/page-header'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'

import { CurriculumStatusBadge, YearRange } from './badges'
import { ItemDialog, VersionDialog } from './CurriculumDialogs'
import { CurriculumView } from './CurriculumView'

type Confirm =
  | { kind: 'activate' | 'archive' | 'delete'; curriculum: Curriculum }
  | { kind: 'remove'; item: CurriculumItem }

/**
 * CurriculaPage, bir programın ders planı sürümlerini gezdirir: hangi giriş yıllarının
 * hangi sürümü izlediği, sürümün yarıyıl yerleşimi ve mezuniyet özeti. Müfredat
 * yöneticileri taslak sürüm açar, satır ekler/çıkarır ve sürümü yürürlüğe koyar ya da arşivler.
 */
export function CurriculaPage() {
  const { t } = useTranslation()
  const { has } = usePermissions()
  const [params, setParams] = useSearchParams()
  const org: OrgSelection = {
    facultyId: params.get('birim') ?? '',
    departmentId: params.get('bolum') ?? '',
    programId: params.get('program') ?? '',
  }
  const canManage = has('curriculum:manage')
  const [dialog, setDialog] = useState<'new' | 'edit' | 'item' | null>(null)

  const setOrg = (v: OrgSelection) => {
    const next = new URLSearchParams()
    if (v.facultyId) next.set('birim', v.facultyId)
    if (v.departmentId) next.set('bolum', v.departmentId)
    if (v.programId) next.set('program', v.programId)
    setParams(next, { replace: true })
  }
  const select = (id: string) => {
    const next = new URLSearchParams(params)
    next.set('surum', id)
    setParams(next, { replace: true })
  }

  const versions = useListProgramCurriculaQuery({ id: org.programId }, { skip: !org.programId })
  const list = versions.data?.items ?? []
  // Varsayılan: yeni girişlere açık (en güncel) yürürlükteki sürüm.
  const selectedId = params.get('surum') ?? list.find((v) => v.status === 'ACTIVE')?.id ?? list[0]?.id ?? ''
  const selected = list.find((v) => v.id === selectedId)

  return (
    <>
      <PageHeader
        title={t('curriculum.title')}
        description={t('curriculum.description')}
        actions={
          canManage &&
          org.programId && (
            <Button onClick={() => setDialog('new')}>
              <PlusIcon />
              {t('curriculum.newVersion')}
            </Button>
          )
        }
      />
      <div className="grid gap-6 xl:grid-cols-[18rem_minmax(0,1fr)]">
        <div className="min-w-0 space-y-4">
          <Card>
            <CardContent>
              <OrgPicker depth="program" value={org} onChange={setOrg} />
            </CardContent>
          </Card>
          {org.programId &&
            (versions.error ? (
              <ErrorState error={versions.error} onRetry={() => void versions.refetch()} />
            ) : !versions.data ? (
              <Skeleton className="h-32" />
            ) : list.length === 0 ? (
              <EmptyState>{t('curriculum.noVersions')}</EmptyState>
            ) : (
              <nav aria-label={t('curriculum.versions')}>
                <ul className="space-y-2">
                  {list.map((v) => (
                    <li key={v.id}>
                      <VersionCard version={v} selected={v.id === selectedId} onSelect={() => select(v.id)} />
                    </li>
                  ))}
                </ul>
              </nav>
            ))}
        </div>
        <div className="min-w-0">
          {!org.programId ? (
            <EmptyState>{t('curriculum.chooseProgram')}</EmptyState>
          ) : (
            selected && (
              <Version
                summary={selected}
                canManage={canManage}
                onEdit={() => setDialog('edit')}
                onAddItem={() => setDialog('item')}
              />
            )
          )}
        </div>
      </div>
      {dialog === 'new' && (
        <VersionDialog
          programId={org.programId}
          versions={list}
          onClose={() => setDialog(null)}
          onCreated={select}
        />
      )}
      {dialog === 'edit' && selected && (
        <VersionDialog
          programId={org.programId}
          versions={list}
          editing={selected}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog === 'item' && selected && (
        <ItemDialog curriculumId={selected.id} onClose={() => setDialog(null)} />
      )}
    </>
  )
}

function VersionCard({
  version,
  selected,
  onSelect,
}: {
  version: Curriculum
  selected: boolean
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={selected ? 'true' : undefined}
      className={cn(
        'w-full rounded-md border p-3 text-left transition-colors hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none',
        selected && 'border-primary bg-accent',
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{localized(version)}</span>
        <CurriculumStatusBadge status={version.status} />
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        <YearRange from={version.effective_from_year} to={version.effective_to_year} />
      </p>
      <p className="text-xs text-muted-foreground">
        {t('curriculum.studentCount', { count: version.student_count })}
      </p>
    </button>
  )
}

function Version({
  summary,
  canManage,
  onEdit,
  onAddItem,
}: {
  summary: Curriculum
  canManage: boolean
  onEdit: () => void
  onAddItem: () => void
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const message = useErrorMessage()
  const { data, error, refetch } = useGetCurriculumQuery({ id: summary.id })
  const [activate] = useActivateCurriculumMutation()
  const [archive] = useArchiveCurriculumMutation()
  const [remove] = useDeleteCurriculumMutation()
  const [removeItem] = useDeleteCurriculumItemMutation()
  const [confirm, setConfirm] = useState<Confirm | null>(null)

  if (error) {
    return <ErrorState error={error} onRetry={() => void refetch()} />
  }
  if (!data) {
    return <Skeleton className="h-96" />
  }
  const draft = data.status === 'DRAFT'

  const run = async (action: () => Promise<{ error?: AnyError }>, success: string) => {
    const res = await action()
    if (res.error) {
      toast.error(message(res.error))
      return
    }
    toast.success(success)
    setConfirm(null)
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            {localized(data)} <CurriculumStatusBadge status={data.status} />
          </h2>
          <p className="text-sm text-muted-foreground">
            <YearRange from={data.effective_from_year} to={data.effective_to_year} />
            {data.decision_ref && ` · ${data.decision_ref}`}
          </p>
        </div>
        {canManage && data.status !== 'ARCHIVED' && (
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={onEdit}>
              <PencilIcon />
              {t('curriculum.actions.edit')}
            </Button>
            {draft && (
              <>
                <Button variant="outline" size="sm" onClick={onAddItem}>
                  <PlusIcon />
                  {t('curriculum.edit.add')}
                </Button>
                <Button size="sm" onClick={() => setConfirm({ kind: 'activate', curriculum: data })}>
                  <CheckCircle2Icon />
                  {t('curriculum.actions.activate')}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setConfirm({ kind: 'delete', curriculum: data })}
                >
                  <Trash2Icon />
                  {t('curriculum.actions.delete')}
                </Button>
              </>
            )}
            {data.status === 'ACTIVE' && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setConfirm({ kind: 'archive', curriculum: data })}
              >
                <ArchiveIcon />
                {t('curriculum.actions.archive')}
              </Button>
            )}
          </div>
        )}
      </div>
      <CurriculumView
        curriculum={data}
        {...(canManage &&
          draft && { onRemove: (item: CurriculumItem) => setConfirm({ kind: 'remove', item }) })}
      />
      {confirm?.kind === 'activate' && (
        <ConfirmDialog
          title={t('curriculum.confirm.activateTitle')}
          description={t('curriculum.confirm.activate', { name: localized(confirm.curriculum) })}
          confirm={t('curriculum.actions.activate')}
          onClose={() => setConfirm(null)}
          onConfirm={() =>
            run(() => activate({ id: confirm.curriculum.id }), t('curriculum.confirm.activated'))
          }
        />
      )}
      {confirm?.kind === 'archive' && (
        <ConfirmDialog
          title={t('curriculum.confirm.archiveTitle')}
          description={t('curriculum.confirm.archive', { name: localized(confirm.curriculum) })}
          confirm={t('curriculum.actions.archive')}
          onClose={() => setConfirm(null)}
          onConfirm={() =>
            run(() => archive({ id: confirm.curriculum.id }), t('curriculum.confirm.archived'))
          }
        />
      )}
      {confirm?.kind === 'delete' && (
        <ConfirmDialog
          title={t('curriculum.confirm.deleteTitle')}
          description={t('curriculum.confirm.delete', { name: localized(confirm.curriculum) })}
          confirm={t('curriculum.actions.delete')}
          destructive
          onClose={() => setConfirm(null)}
          onConfirm={() => run(() => remove({ id: confirm.curriculum.id }), t('curriculum.confirm.deleted'))}
        />
      )}
      {confirm?.kind === 'remove' && (
        <ConfirmDialog
          title={t('curriculum.confirm.removeTitle')}
          description={t('curriculum.confirm.remove', { code: confirm.item.code })}
          confirm={t('curriculum.confirm.removeConfirm')}
          destructive
          onClose={() => setConfirm(null)}
          onConfirm={() =>
            run(() => removeItem({ id: data.id, itemId: confirm.item.id }), t('curriculum.confirm.removed'))
          }
        />
      )}
    </div>
  )
}
