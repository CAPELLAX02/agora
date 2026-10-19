import { InfoIcon, Link2Icon, SaveIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { formatNumber } from '@/i18n/format'
import {
  useGetAssignmentLinkQuery,
  useSetAssignmentLinkMutation,
  type AssignmentLink,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Button } from '@/shared/ui/button'
import { IconChip } from '@/shared/ui/icon-chip'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'

const none = 'none'

/**
 * AssignmentLinkCard, ödevin not tablosu bağlantısıdır: ödev şubenin dönem içi bir bileşenini
 * ağırlıkla besler, yayımlanan puanlar bileşenin notunu üretir (kaynak LMS).
 */
export function AssignmentLinkCard({ activityId, canEdit }: { activityId: string; canEdit: boolean }) {
  const { data } = useGetAssignmentLinkQuery({ id: activityId })
  if (!data) {
    return <Skeleton className="h-28" />
  }
  return (
    <LinkForm
      key={`${data.link?.component_id ?? none}-${data.link?.weight ?? 0}`}
      activityId={activityId}
      data={data}
      canEdit={canEdit}
    />
  )
}

function LinkForm({
  activityId,
  data,
  canEdit,
}: {
  activityId: string
  data: AssignmentLink
  canEdit: boolean
}) {
  const { t, i18n } = useTranslation()
  const message = useErrorMessage()
  const [save, saving] = useSetAssignmentLinkMutation()
  const [componentId, setComponentId] = useState(data.link?.component_id ?? none)
  const [weight, setWeight] = useState(String(data.link?.weight ?? 1).replace('.', ','))
  const en = i18n.language.startsWith('en')
  const label = (c: AssignmentLink['components'][number]) => (en ? c.label_en : c.label_tr)
  const linked = data.components.find((c) => c.id === data.link?.component_id)
  const parsedWeight = Number(weight.replace(',', '.'))
  const dirty =
    componentId !== (data.link?.component_id ?? none) ||
    (componentId !== none && parsedWeight !== (data.link?.weight ?? 1))

  const submit = async () => {
    const res = await save({
      id: activityId,
      body:
        componentId === none ? { component_id: null } : { component_id: componentId, weight: parsedWeight },
    })
    if (res.error) {
      toast.error(message(res.error))
      return
    }
    toast.success(t(`lms.assignment.link.saved.${res.data.sync_state ?? 'NOT_LINKED'}`))
  }

  return (
    <section
      aria-labelledby="link-title"
      className="space-y-3 rounded-xl border border-indigo-200/70 bg-indigo-50/30 p-4 dark:border-indigo-500/20 dark:bg-indigo-500/5"
    >
      <h3 id="link-title" className="flex items-center gap-2 text-sm font-bold">
        <IconChip icon={Link2Icon} tone="indigo" size="sm" />
        {t('lms.assignment.link.title')}
      </h3>
      <p className="text-sm">
        {linked
          ? t('lms.assignment.link.current', {
              component: label(linked),
              weight: formatNumber(data.link!.weight),
            })
          : t('lms.assignment.link.none')}
      </p>
      {data.components.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('lms.assignment.link.noComponents')}</p>
      ) : (
        <>
          {!data.plan_locked && (
            <Alert>
              <InfoIcon />
              <AlertDescription>{t('lms.assignment.link.planNotLocked')}</AlertDescription>
            </Alert>
          )}
          {data.grades_published && (
            <Alert>
              <InfoIcon />
              <AlertDescription>{t('lms.assignment.link.frozen')}</AlertDescription>
            </Alert>
          )}
          {canEdit && (
            <div className="grid gap-3 sm:grid-cols-[1fr_120px_auto] sm:items-end">
              <div className="space-y-1.5">
                <Label htmlFor="link-component">{t('lms.assignment.link.component')}</Label>
                <Select value={componentId} onValueChange={setComponentId}>
                  <SelectTrigger id="link-component" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={none}>{t('lms.assignment.link.noneOption')}</SelectItem>
                    {data.components.map((c) => (
                      <SelectItem key={c.id} value={c.id}>
                        {label(c)} · %{formatNumber(c.weight)}
                        {c.linked_assignments > 0 &&
                          ` · ${t('lms.assignment.link.linkedCount', { count: c.linked_assignments })}`}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="link-weight">{t('lms.assignment.link.weight')}</Label>
                <Input
                  id="link-weight"
                  inputMode="decimal"
                  value={weight}
                  disabled={componentId === none}
                  onChange={(e) => setWeight(e.target.value)}
                />
              </div>
              <Button
                onClick={() => void submit()}
                disabled={
                  !dirty ||
                  saving.isLoading ||
                  (componentId !== none && !(parsedWeight > 0 && parsedWeight <= 100))
                }
              >
                <SaveIcon />
                {t('lms.assignment.link.save')}
              </Button>
            </div>
          )}
          <p className="text-xs text-muted-foreground">{t('lms.assignment.link.hint')}</p>
        </>
      )}
    </section>
  )
}
