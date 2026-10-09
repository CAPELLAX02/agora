import { ChevronRightIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useSearchParams } from 'react-router'

import { useEventDetail } from '@/features/account/events'
import { formatDateTime } from '@/i18n/format'
import {
  useListAuditLogQuery,
  useListSecurityEventsQuery,
  type AuditLogEntry,
  type ListAuditLogApiArg,
  type ListSecurityEventsApiArg,
  type SecurityEventType,
} from '@/shared/api/generated'
import { useDebounced } from '@/shared/lib/useDebounced'
import { Badge } from '@/shared/ui/badge'
import { Button } from '@/shared/ui/button'
import { Card, CardContent } from '@/shared/ui/card'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page-header'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { Skeleton } from '@/shared/ui/skeleton'
import { EmptyState, ErrorState } from '@/shared/ui/states'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/ui/tabs'

const eventTypes: SecurityEventType[] = [
  'LOGIN_SUCCEEDED',
  'LOGIN_FAILED',
  'ACCOUNT_LOCKED',
  'LOGOUT',
  'REFRESH_TOKEN_REUSED',
  'PASSWORD_CHANGED',
  'PASSWORD_RESET_REQUESTED',
  'PASSWORD_RESET_COMPLETED',
  'SESSION_REVOKED',
  'OTHER_SESSIONS_REVOKED',
  'MFA_ENABLED',
  'MFA_DISABLED',
  'MFA_CHALLENGE_STARTED',
  'MFA_RECOVERY_CODES_RENEWED',
]
const ALL = 'ALL'

/** AuditPage, denetim izini ve güvenlik olaylarını gösterir (sistem yöneticisi ve denetçi). */
export function AuditPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') === 'security' ? 'security' : 'log'
  return (
    <>
      <PageHeader title={t('audit.title')} description={t('audit.description')} />
      <Tabs value={tab} onValueChange={(v) => setParams({ tab: v }, { replace: true })} className="gap-4">
        <TabsList>
          <TabsTrigger value="log">{t('audit.tabs.log')}</TabsTrigger>
          <TabsTrigger value="security">{t('audit.tabs.security')}</TabsTrigger>
        </TabsList>
        <TabsContent value="log">
          <AuditLog />
        </TabsContent>
        <TabsContent value="security">
          <SecurityEvents />
        </TabsContent>
      </Tabs>
    </>
  )
}

/** usePages, imleçli bir listenin yüklenmiş sayfalarıdır. Süzgeç değişince baştan başlar. */
function usePages(filterKey: string) {
  const [pages, setPages] = useState({ key: filterKey, cursors: [''] })
  const cursors = pages.key === filterKey ? pages.cursors : ['']
  return { cursors, more: (next: string) => setPages({ key: filterKey, cursors: [...cursors, next] }) }
}

function AuditLog() {
  const { t } = useTranslation()
  const [action, setAction] = useState('')
  const debounced = useDebounced(action.trim())
  const filter: ListAuditLogApiArg = { limit: 25, ...(debounced && { action: debounced }) }
  const key = JSON.stringify(filter)
  const { cursors, more } = usePages(key)

  return (
    <Card className="gap-0 py-0">
      <CardContent className="border-b p-4">
        <Input
          value={action}
          onChange={(e) => setAction(e.target.value)}
          placeholder={t('audit.actionFilter')}
          aria-label={t('audit.action')}
          className="font-mono sm:max-w-xs"
        />
      </CardContent>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-8" />
            <TableHead>{t('audit.when')}</TableHead>
            <TableHead>{t('audit.actor')}</TableHead>
            <TableHead>{t('audit.action')}</TableHead>
            <TableHead>{t('audit.entity')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {cursors.map((cursor, i) => (
            <AuditRows
              key={`${key}:${cursor}`}
              filter={filter}
              cursor={cursor}
              last={i === cursors.length - 1}
              onMore={more}
            />
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

function AuditRows({
  filter,
  cursor,
  last,
  onMore,
}: {
  filter: ListAuditLogApiArg
  cursor: string
  last: boolean
  onMore: (next: string) => void
}) {
  const { t } = useTranslation()
  const { data, error, refetch, isFetching } = useListAuditLogQuery({ ...filter, ...(cursor && { cursor }) })
  return (
    <PageRows
      span={5}
      data={data}
      error={error}
      retry={() => void refetch()}
      first={cursor === ''}
      emptyText={t('audit.empty')}
    >
      {data?.items.map((e) => (
        <AuditRow key={e.id} entry={e} />
      ))}
      {last && data?.next_cursor && (
        <MoreRow span={5} loading={isFetching} onClick={() => onMore(data.next_cursor ?? '')} />
      )}
    </PageRows>
  )
}

function AuditRow({ entry: e }: { entry: AuditLogEntry }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const hasChanges = e.before != null || e.after != null
  return (
    <>
      <TableRow>
        <TableCell>
          {hasChanges && (
            <Button
              variant="ghost"
              size="icon-sm"
              aria-expanded={open}
              aria-label={t('audit.changes')}
              onClick={() => setOpen((v) => !v)}
            >
              <ChevronRightIcon
                className={open ? 'rotate-90 transition-transform' : 'transition-transform'}
              />
            </Button>
          )}
        </TableCell>
        <TableCell className="text-muted-foreground">{formatDateTime(e.occurred_at)}</TableCell>
        <TableCell>
          {e.actor_user_id ? (
            <Link
              to={`/yonetim/kullanicilar/${e.actor_user_id}`}
              className="font-mono text-xs hover:underline"
            >
              {e.actor_username ?? e.actor_user_id.slice(0, 8)}
            </Link>
          ) : (
            <Badge variant="muted">{t('audit.system')}</Badge>
          )}
        </TableCell>
        <TableCell>
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{e.action}</code>
        </TableCell>
        <TableCell className="text-xs">
          {e.entity_type === 'user' && e.entity_id ? (
            <Link to={`/yonetim/kullanicilar/${e.entity_id}`} className="font-mono hover:underline">
              user/{e.entity_id.slice(0, 8)}
            </Link>
          ) : (
            <span className="font-mono">
              {e.entity_type}
              {e.entity_id && `/${e.entity_id.slice(0, 8)}`}
            </span>
          )}
        </TableCell>
      </TableRow>
      {open && (
        <TableRow className="bg-muted/30 hover:bg-muted/30">
          <TableCell />
          <TableCell colSpan={4} className="whitespace-normal">
            <div className="grid gap-3 py-1 md:grid-cols-2">
              <JsonBlock label={t('audit.before')} value={e.before as unknown} />
              <JsonBlock label={t('audit.after')} value={e.after as unknown} />
            </div>
          </TableCell>
        </TableRow>
      )}
    </>
  )
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-muted-foreground uppercase">{label}</p>
      <pre className="max-h-56 overflow-auto rounded-md border bg-background p-2 font-mono text-xs">
        {value == null ? '—' : JSON.stringify(value, null, 2)}
      </pre>
    </div>
  )
}

function SecurityEvents() {
  const { t } = useTranslation()
  const [type, setType] = useState<SecurityEventType | ''>('')
  const filter: ListSecurityEventsApiArg = { limit: 25, ...(type && { type }) }
  const key = JSON.stringify(filter)
  const { cursors, more } = usePages(key)
  return (
    <Card className="gap-0 py-0">
      <CardContent className="border-b p-4">
        <Select value={type || ALL} onValueChange={(v) => setType(v === ALL ? '' : (v as SecurityEventType))}>
          <SelectTrigger className="sm:w-72" aria-label={t('events.event')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t('events.allTypes')}</SelectItem>
            {eventTypes.map((et) => (
              <SelectItem key={et} value={et}>
                {t(`events.types.${et}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </CardContent>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('events.when')}</TableHead>
            <TableHead>{t('events.user')}</TableHead>
            <TableHead>{t('events.event')}</TableHead>
            <TableHead>{t('events.ip')}</TableHead>
            <TableHead>{t('events.details')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {cursors.map((cursor, i) => (
            <EventRows
              key={`${key}:${cursor}`}
              filter={filter}
              cursor={cursor}
              last={i === cursors.length - 1}
              onMore={more}
            />
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

function EventRows({
  filter,
  cursor,
  last,
  onMore,
}: {
  filter: ListSecurityEventsApiArg
  cursor: string
  last: boolean
  onMore: (next: string) => void
}) {
  const { t } = useTranslation()
  const detail = useEventDetail()
  const { data, error, refetch, isFetching } = useListSecurityEventsQuery({
    ...filter,
    ...(cursor && { cursor }),
  })
  return (
    <PageRows
      span={5}
      data={data}
      error={error}
      retry={() => void refetch()}
      first={cursor === ''}
      emptyText={t('events.empty')}
    >
      {data?.items.map((e) => (
        <TableRow key={e.id}>
          <TableCell className="text-muted-foreground">{formatDateTime(e.occurred_at)}</TableCell>
          <TableCell className="font-mono text-xs">
            {e.user_id ? (
              <Link to={`/yonetim/kullanicilar/${e.user_id}`} className="hover:underline">
                {e.username ?? e.user_id.slice(0, 8)}
              </Link>
            ) : (
              (e.username_attempted ?? '—')
            )}
          </TableCell>
          <TableCell>{t(`events.types.${e.type}`)}</TableCell>
          <TableCell className="font-mono text-xs">{e.ip ?? '—'}</TableCell>
          <TableCell className="text-muted-foreground">{detail(e)}</TableCell>
        </TableRow>
      ))}
      {last && data?.next_cursor && (
        <MoreRow span={5} loading={isFetching} onClick={() => onMore(data.next_cursor ?? '')} />
      )}
    </PageRows>
  )
}

/** PageRows, bir sayfanın yükleniyor, hata ve boş hallerini tablo satırı olarak çizer. */
function PageRows({
  span,
  data,
  error,
  retry,
  first,
  emptyText,
  children,
}: {
  span: number
  data: { items: unknown[] } | undefined
  error: Parameters<typeof ErrorState>[0]['error']
  retry: () => void
  first: boolean
  emptyText: string
  children: React.ReactNode
}) {
  if (error) {
    return (
      <TableRow>
        <TableCell colSpan={span}>
          <ErrorState error={error} onRetry={retry} />
        </TableCell>
      </TableRow>
    )
  }
  if (!data) {
    return (
      <TableRow>
        <TableCell colSpan={span}>
          <Skeleton className="h-24" />
        </TableCell>
      </TableRow>
    )
  }
  if (data.items.length === 0 && first) {
    return (
      <TableRow className="hover:bg-transparent">
        <TableCell colSpan={span}>
          <EmptyState>{emptyText}</EmptyState>
        </TableCell>
      </TableRow>
    )
  }
  return <>{children}</>
}

function MoreRow({ span, loading, onClick }: { span: number; loading: boolean; onClick: () => void }) {
  const { t } = useTranslation()
  return (
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={span} className="text-center">
        <Button variant="ghost" size="sm" loading={loading} onClick={onClick}>
          {t('common.loadMore')}
        </Button>
      </TableCell>
    </TableRow>
  )
}
