import { CornerUpLeftIcon, EllipsisIcon, FileIcon, PencilIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useAppDispatch } from '@/app/hooks'
import { formatDateTime } from '@/i18n/format'
import {
  agoraApi,
  useDeleteMessageMutation,
  useEditMessageMutation,
  type ConversationMessage,
} from '@/shared/api/generated'
import { formatBytes } from '@/shared/api/upload'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { openDownload } from '@/shared/lib/download'
import { cn } from '@/shared/lib/utils'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/shared/ui/dropdown-menu'
import { Textarea } from '@/shared/ui/textarea'

/**
 * Bubble, bir iletidir: yazar (grupta), alıntılanan ileti, metin, ekler ve zaman. Menüden yanıtlanır;
 * yazarı düzenler ve siler, konuşmanın yöneticisi de siler.
 */
export function Bubble({
  m,
  group,
  moderator,
  quote,
  onReply,
}: {
  m: ConversationMessage
  group: boolean
  moderator: boolean
  quote?: ConversationMessage
  onReply: (m: ConversationMessage) => void
}) {
  const { t } = useTranslation()
  const dispatch = useAppDispatch()
  const errorMessage = useErrorMessage()
  const [edit, editing] = useEditMessageMutation()
  const [remove] = useDeleteMessageMutation()
  const [draft, setDraft] = useState<string | null>(null)
  const [confirm, setConfirm] = useState(false)
  const canDelete = !m.deleted && (m.mine || moderator)

  const download = async (fileId: string) => {
    try {
      const res = await dispatch(
        agoraApi.endpoints.getMessageAttachmentUrl.initiate(
          { id: m.id, fileId },
          { subscribe: false, forceRefetch: true },
        ),
      ).unwrap()
      openDownload(res.url)
    } catch (e) {
      toast.error(errorMessage(e as never))
    }
  }
  const save = async () => {
    const res = await edit({ id: m.id, body: { body: draft ?? '' } })
    if (res.error) {
      toast.error(errorMessage(res.error))
      return
    }
    setDraft(null)
  }

  return (
    <li className={cn('group/bubble flex items-start gap-1', m.mine ? 'flex-row-reverse' : 'flex-row')}>
      <div
        className={cn(
          'max-w-[80%] rounded-2xl px-3.5 py-2 text-sm shadow-xs',
          m.mine
            ? 'rounded-br-md bg-gradient-to-br from-sky-500 to-blue-600 text-white'
            : 'rounded-bl-md border border-border/70 bg-muted/40',
        )}
      >
        {group && !m.mine && (
          <p
            className={cn(
              'mb-0.5 text-xs font-bold',
              m.sender.is_staff ? 'text-violet-600 dark:text-violet-300' : 'text-sky-700 dark:text-sky-300',
            )}
          >
            {m.sender.name}
          </p>
        )}
        {quote && (
          <p
            className={cn(
              'mb-1.5 line-clamp-2 border-l-2 pl-2 text-xs',
              m.mine ? 'border-white/60 text-white/85' : 'border-sky-500 text-muted-foreground',
            )}
          >
            <span className="font-semibold">{quote.sender.name}: </span>
            {quote.deleted ? t('messages.deleted') : quote.body || t('messages.attachmentOnly')}
          </p>
        )}
        {m.deleted ? (
          <p className="italic opacity-75">{t('messages.deleted')}</p>
        ) : draft !== null ? (
          <div className="space-y-2">
            <Textarea
              value={draft}
              maxLength={4000}
              onChange={(e) => setDraft(e.target.value)}
              aria-label={t('messages.editLabel')}
              className="min-w-64 bg-background text-foreground"
              autoFocus
            />
            <div className="flex justify-end gap-2">
              <Button size="sm" variant="secondary" onClick={() => setDraft(null)}>
                {t('common.cancel')}
              </Button>
              <Button size="sm" variant="secondary" disabled={editing.isLoading} onClick={() => void save()}>
                {t('messages.save')}
              </Button>
            </div>
          </div>
        ) : (
          m.body && <p className="break-words whitespace-pre-wrap">{m.body}</p>
        )}
        {!m.deleted && m.attachments.length > 0 && (
          <ul className="mt-1.5 space-y-1">
            {m.attachments.map((a) => (
              <li key={a.id}>
                <button
                  type="button"
                  onClick={() => void download(a.id)}
                  aria-label={t('messages.download', { name: a.name })}
                  className={cn(
                    'flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition-colors',
                    m.mine ? 'bg-white/15 hover:bg-white/25' : 'bg-background/70 hover:bg-background',
                  )}
                >
                  <FileIcon className="size-4 shrink-0" />
                  <span className="min-w-0 flex-1 truncate font-medium">{a.name}</span>
                  <span className="shrink-0 opacity-75">{formatBytes(a.size)}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        <p className={cn('mt-1 text-right text-[10px]', m.mine ? 'text-white/75' : 'text-muted-foreground')}>
          {formatDateTime(m.created_at)}
          {m.edited_at && ` · ${t('messages.edited')}`}
        </p>
      </div>
      {!m.deleted && draft === null && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-7 opacity-0 group-hover/bubble:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100"
              aria-label={t('messages.actions')}
            >
              <EllipsisIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align={m.mine ? 'end' : 'start'}>
            <DropdownMenuItem onSelect={() => onReply(m)}>
              <CornerUpLeftIcon />
              {t('messages.reply')}
            </DropdownMenuItem>
            {m.mine && (
              <DropdownMenuItem onSelect={() => setDraft(m.body)}>
                <PencilIcon />
                {t('messages.edit')}
              </DropdownMenuItem>
            )}
            {canDelete && (
              <DropdownMenuItem
                onSelect={() => setConfirm(true)}
                className="text-rose-600 focus:text-rose-600"
              >
                <Trash2Icon />
                {t('messages.delete')}
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      {confirm && (
        <ConfirmDialog
          destructive
          title={t('messages.deleteTitle')}
          description={t('messages.deleteDescription')}
          confirm={t('messages.delete')}
          onClose={() => setConfirm(false)}
          onConfirm={async () => {
            const res = await remove({ id: m.id })
            if (res.error) {
              toast.error(errorMessage(res.error))
              return
            }
            setConfirm(false)
          }}
        />
      )}
    </li>
  )
}
