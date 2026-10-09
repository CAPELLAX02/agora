import { PencilIcon, PlusIcon, Trash2Icon, UsersIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { usePermissions } from '@/features/auth/usePermissions'
import { day, roomLabel } from '@/features/schedule/layout'
import {
  useDeleteScheduleSlotMutation,
  useDeleteSectionMutation,
  type ScheduleSlot,
  type Section,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { useLocalized } from '@/shared/lib/localized'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/shared/ui/card'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

import { InstructorsDialog, QuotasDialog, SectionDialog, SlotDialog } from './dialogs'

type Dialog = 'edit' | 'slot' | 'instructors' | 'quotas' | null

/** SectionCard, bir şubenin kontenjanını, öğretim elemanlarını, program kontenjanlarını ve oturumlarını gösterir. */
export function SectionCard({ section: s, editable }: { section: Section; editable: boolean }) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const { has } = usePermissions()
  const message = useErrorMessage()
  const [dialog, setDialog] = useState<Dialog>(null)
  const [deleting, setDeleting] = useState<ScheduleSlot | 'section' | null>(null)
  const [deleteSlot] = useDeleteScheduleSlotMutation()
  const [deleteSection] = useDeleteSectionMutation()
  const active = editable && s.status === 'ACTIVE'
  const canSection = active && has('section:manage')
  const canSchedule = active && has('schedule:manage')
  const canQuota = active && has('quota:manage')

  return (
    <Card
      className="gap-0 py-0"
      role="region"
      aria-label={t('offering.section.title', { code: s.section_code })}
    >
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3 border-b py-4">
        <div className="space-y-1">
          <CardTitle className="flex flex-wrap items-center gap-2">
            {t('offering.section.title', { code: s.section_code })}
            {s.status === 'CANCELLED' && (
              <Badge variant="destructive">{t('offering.section.cancelled')}</Badge>
            )}
            {s.quota_mode === 'RESERVED' && (
              <Badge variant="secondary">{t('offering.section.reserved')}</Badge>
            )}
          </CardTitle>
          <p className="text-sm text-muted-foreground">
            {t('offering.section.summary', {
              enrolled: s.enrolled_count,
              capacity: s.capacity,
              language: t(`catalog.languages.${s.language}`),
              mode: t(`offering.section.modes.${s.instruction_mode}`),
            })}
          </p>
        </div>
        {canSection && (
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => setDialog('edit')}>
              <PencilIcon />
              {t('offering.section.edit')}
            </Button>
            {s.enrolled_count === 0 && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setDeleting('section')}
                aria-label={t('offering.section.delete', { code: s.section_code })}
              >
                <Trash2Icon />
              </Button>
            )}
          </div>
        )}
      </CardHeader>
      <CardContent className="grid gap-6 py-4 lg:grid-cols-2">
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">{t('offering.section.instructors')}</h3>
            {canSection && (
              <Button variant="ghost" size="sm" onClick={() => setDialog('instructors')}>
                <UsersIcon />
                {t('offering.section.editInstructors')}
              </Button>
            )}
          </div>
          {s.instructors.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('offering.section.noInstructors')}</p>
          ) : (
            <ul className="space-y-1 text-sm">
              {s.instructors.map((i) => (
                <li key={i.staff_id} className="flex items-center gap-2">
                  {[i.title, i.first_name, i.last_name].filter(Boolean).join(' ')}
                  <Badge variant="outline">{t(`teaching.roles.${i.role}`)}</Badge>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">{t('offering.section.quotas')}</h3>
            {canQuota && (
              <Button variant="ghost" size="sm" onClick={() => setDialog('quotas')}>
                <PencilIcon />
                {t('offering.section.editQuotas')}
              </Button>
            )}
          </div>
          {s.quotas.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('offering.section.openToAll')}</p>
          ) : (
            <ul className="space-y-1 text-sm">
              {s.quotas.map((q) => (
                <li key={q.program.id} className="flex justify-between gap-2">
                  <span>{localized(q.program)}</span>
                  <span className="text-muted-foreground tabular-nums">
                    {q.enrolled} / {q.quota}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="space-y-2 lg:col-span-2">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-medium">{t('offering.section.slots')}</h3>
            {canSchedule && (
              <Button variant="ghost" size="sm" onClick={() => setDialog('slot')}>
                <PlusIcon />
                {t('offering.slot.add')}
              </Button>
            )}
          </div>
          {s.slots.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('offering.section.noSlots')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('offering.slot.day')}</TableHead>
                  <TableHead>{t('schedule.agenda.time')}</TableHead>
                  <TableHead>{t('schedule.classroom')}</TableHead>
                  <TableHead>{t('offering.slot.type')}</TableHead>
                  {canSchedule && <TableHead className="w-10" />}
                </TableRow>
              </TableHeader>
              <TableBody>
                {s.slots.map((sl) => (
                  <TableRow key={sl.id}>
                    <TableCell>{t(`schedule.days.${day(sl.day_of_week)}`)}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {sl.start_time}–{sl.end_time}
                    </TableCell>
                    <TableCell>{roomLabel(sl) ?? t('schedule.online')}</TableCell>
                    <TableCell>{t(`schedule.sessions.${sl.session_type}`)}</TableCell>
                    {canSchedule && (
                      <TableCell className="text-right">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => setDeleting(sl)}
                          aria-label={t('offering.slot.delete', {
                            day: t(`schedule.days.${day(sl.day_of_week)}`),
                            time: sl.start_time,
                          })}
                        >
                          <Trash2Icon />
                        </Button>
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      </CardContent>
      {dialog === 'edit' && <SectionDialog section={s} onClose={() => setDialog(null)} />}
      {dialog === 'slot' && <SlotDialog section={s} onClose={() => setDialog(null)} />}
      {dialog === 'instructors' && <InstructorsDialog section={s} onClose={() => setDialog(null)} />}
      {dialog === 'quotas' && <QuotasDialog section={s} onClose={() => setDialog(null)} />}
      {deleting && (
        <ConfirmDialog
          title={t(deleting === 'section' ? 'offering.section.deleteTitle' : 'offering.slot.deleteTitle')}
          description={t(
            deleting === 'section' ? 'offering.section.deleteDescription' : 'offering.slot.deleteDescription',
          )}
          confirm={t('offering.transitions.delete')}
          destructive
          onClose={() => setDeleting(null)}
          onConfirm={async () => {
            const res =
              deleting === 'section'
                ? await deleteSection({ id: s.id })
                : await deleteSlot({ id: deleting.id })
            if (res.error) {
              toast.error(message(res.error))
              return
            }
            setDeleting(null)
          }}
        />
      )}
    </Card>
  )
}
