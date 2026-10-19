import { CheckIcon, MessageSquarePlusIcon, SearchIcon, UsersRoundIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'

import { usePermissions } from '@/features/auth/usePermissions'
import {
  useListMessagingContactsQuery,
  useStartConversationMutation,
  type MessagingPerson,
} from '@/shared/api/generated'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { cn } from '@/shared/lib/utils'
import { Alert, AlertDescription } from '@/shared/ui/alert'
import { Avatar } from '@/shared/ui/avatar'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Spinner } from '@/shared/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/shared/ui/tabs'

import { splitName } from './names'

/** NewConversationButton, yeni konuşma penceresini açar. */
export function NewConversationButton() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        <MessageSquarePlusIcon />
        {t('messages.new')}
      </Button>
      {open && <NewConversationDialog onClose={() => setOpen(false)} />}
    </>
  )
}

/** useDebounced, değerin yazma durduktan sonraki hâlidir (aramada her tuşa istek atılmasın). */
function useDebounced<T>(value: T, ms = 300) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

/**
 * NewConversationDialog, yazılabilecek kişilerden birebir konuşma açar; personel başlık ve üyelerle
 * grup da kurar. Öğrenciye kişiler (öğretim elemanları, danışman) arama yapmadan listelenir.
 */
function NewConversationDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const errorMessage = useErrorMessage()
  const { has } = usePermissions()
  const student = has('registration:manage_own')
  const [mode, setMode] = useState<'direct' | 'group'>('direct')
  const [query, setQuery] = useState('')
  const q = useDebounced(query.trim())
  const { data, isFetching } = useListMessagingContactsQuery({ q: q || undefined })
  const [start, starting] = useStartConversationMutation()
  const [title, setTitle] = useState('')
  const [picked, setPicked] = useState<MessagingPerson[]>([])
  const [error, setError] = useState<string | null>(null)

  const open = async (body: Parameters<typeof start>[0]['body']) => {
    setError(null)
    const res = await start({ body })
    if (res.error) {
      setError(errorMessage(res.error))
      return
    }
    onClose()
    void navigate(`/mesajlar/${res.data.id}`)
  }
  const toggle = (p: MessagingPerson) =>
    setPicked((list) => (list.some((x) => x.id === p.id) ? list.filter((x) => x.id !== p.id) : [...list, p]))

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent closeLabel={t('common.close')}>
        <DialogHeader icon={MessageSquarePlusIcon} tone="sky">
          <DialogTitle>{t('messages.new')}</DialogTitle>
          <DialogDescription>
            {student ? t('messages.newHintStudent') : t('messages.newHintStaff')}
          </DialogDescription>
        </DialogHeader>
        {!student && (
          <Tabs value={mode} onValueChange={(v) => setMode(v as 'direct' | 'group')}>
            <TabsList>
              <TabsTrigger value="direct">{t('messages.direct')}</TabsTrigger>
              <TabsTrigger value="group">{t('messages.group')}</TabsTrigger>
            </TabsList>
          </Tabs>
        )}
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        {mode === 'group' && (
          <div className="space-y-2">
            <Label htmlFor="group-title">{t('messages.groupTitle')}</Label>
            <Input
              id="group-title"
              value={title}
              maxLength={120}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
        )}
        <div className="relative">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={student ? t('messages.filterContacts') : t('messages.searchPeople')}
            aria-label={student ? t('messages.filterContacts') : t('messages.searchPeople')}
            className="pl-8"
            autoFocus
          />
        </div>
        <ul aria-label={t('messages.contacts')} className="max-h-72 space-y-0.5 overflow-y-auto">
          {isFetching && !data ? (
            <li className="flex justify-center py-6">
              <Spinner />
            </li>
          ) : (data?.items ?? []).length === 0 ? (
            <li className="py-6 text-center text-sm text-muted-foreground">
              {!student && q.length < 2 ? t('messages.typeToSearch') : t('messages.noContacts')}
            </li>
          ) : (
            data!.items.map((p) => {
              const [first, last] = splitName(p.name)
              const selected = picked.some((x) => x.id === p.id)
              return (
                <li key={p.id}>
                  <button
                    type="button"
                    disabled={starting.isLoading}
                    onClick={() =>
                      mode === 'direct' ? void open({ kind: 'DIRECT', user_id: p.id }) : toggle(p)
                    }
                    aria-pressed={mode === 'group' ? selected : undefined}
                    className={cn(
                      'flex w-full items-center gap-3 rounded-xl px-2.5 py-2 text-left transition-colors outline-none hover:bg-muted/60 focus-visible:ring-4 focus-visible:ring-ring/30',
                      selected && 'bg-sky-500/10',
                    )}
                  >
                    <Avatar firstName={first} lastName={last} className="size-9" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold">{p.name}</span>
                      <span className="block text-xs text-muted-foreground">
                        {p.number}
                        {p.is_staff && ` · ${t('messages.staff')}`}
                      </span>
                    </span>
                    {selected && <CheckIcon className="size-4 text-sky-600" />}
                  </button>
                </li>
              )
            })
          )}
        </ul>
        {mode === 'group' && (
          <div className="flex items-center justify-between gap-3 border-t border-border/60 pt-3">
            <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <UsersRoundIcon className="size-4" />
              {t('messages.picked', { count: picked.length })}
            </span>
            <Button
              disabled={!title.trim() || picked.length === 0 || starting.isLoading}
              onClick={() =>
                void open({ kind: 'GROUP', title: title.trim(), member_ids: picked.map((p) => p.id) })
              }
            >
              {starting.isLoading && <Spinner />}
              {t('messages.createGroup')}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
