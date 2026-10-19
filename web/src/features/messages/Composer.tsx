import { PaperclipIcon, SendIcon, XIcon } from 'lucide-react'
import { useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useSendMessageMutation, type ConversationMessage } from '@/shared/api/generated'
import { formatBytes, UploadError, useUpload } from '@/shared/api/upload'
import { useErrorMessage } from '@/shared/api/useErrorMessage'
import { Button } from '@/shared/ui/button'
import { Spinner } from '@/shared/ui/spinner'
import { Textarea } from '@/shared/ui/textarea'

const maxBody = 4000
const maxFiles = 5

type Attached = { id: string; name: string; size: number }

/**
 * Composer, ileti yazma kutusudur: Enter gönderir, Shift+Enter satır atlatır; ataşla en çok beş dosya
 * eklenir (seçilince yüklenir); yanıtlanan ileti üstte gösterilir.
 */
export function Composer({
  id,
  replyTo,
  onCancelReply,
  onSent,
}: {
  id: string
  replyTo: ConversationMessage | null
  onCancelReply: () => void
  onSent: () => void
}) {
  const { t } = useTranslation()
  const errorMessage = useErrorMessage()
  const upload = useUpload()
  const [send, sending] = useSendMessageMutation()
  const [body, setBody] = useState('')
  const [files, setFiles] = useState<Attached[]>([])
  const [uploading, setUploading] = useState(0)
  const picker = useRef<HTMLInputElement>(null)

  const choose = async (list: FileList | null) => {
    const chosen = Array.from(list ?? []).slice(0, maxFiles - files.length)
    for (const file of chosen) {
      setUploading((n) => n + 1)
      try {
        const f = await upload(file)
        setFiles((current) => [...current, { id: f.id, name: f.name, size: f.size }])
      } catch (e) {
        toast.error(e instanceof UploadError ? e.message : t('lms.upload.failed'))
      } finally {
        setUploading((n) => n - 1)
      }
    }
  }
  const submit = async () => {
    const text = body.trim()
    if ((!text && files.length === 0) || sending.isLoading || uploading > 0) {
      return
    }
    const res = await send({
      id,
      body: {
        body: text,
        reply_to_id: replyTo?.id,
        file_ids: files.length ? files.map((f) => f.id) : undefined,
      },
    })
    if (res.error) {
      toast.error(errorMessage(res.error))
      return
    }
    setBody('')
    setFiles([])
    onSent()
  }
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      void submit()
    }
  }
  return (
    <form
      className="space-y-2 border-t border-border/60 p-3"
      onSubmit={(e) => {
        e.preventDefault()
        void submit()
      }}
    >
      {replyTo && (
        <div className="flex items-center gap-2 rounded-lg border-l-2 border-sky-500 bg-sky-500/5 px-3 py-1.5 text-xs">
          <span className="min-w-0 flex-1 truncate">
            <span className="font-semibold">{t('messages.replyingTo', { name: replyTo.sender.name })}</span>{' '}
            <span className="text-muted-foreground">{replyTo.body || t('messages.attachmentOnly')}</span>
          </span>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-6"
            onClick={onCancelReply}
            aria-label={t('messages.cancelReply')}
          >
            <XIcon />
          </Button>
        </div>
      )}
      {(files.length > 0 || uploading > 0) && (
        <ul className="flex flex-wrap gap-1.5" aria-label={t('messages.attachments')}>
          {files.map((f) => (
            <li key={f.id} className="flex items-center gap-1 rounded-md bg-muted px-2.5 py-1 text-xs">
              <span className="max-w-48 truncate">{f.name}</span>
              <span className="text-muted-foreground">{formatBytes(f.size)}</span>
              <button
                type="button"
                onClick={() => setFiles((list) => list.filter((x) => x.id !== f.id))}
                aria-label={t('lms.upload.remove', { name: f.name })}
                className="rounded-full p-0.5 hover:bg-background"
              >
                <XIcon className="size-3" />
              </button>
            </li>
          ))}
          {uploading > 0 && (
            <li className="flex items-center gap-1 px-2 text-xs text-muted-foreground">
              <Spinner /> {t('messages.uploading')}
            </li>
          )}
        </ul>
      )}
      <div className="flex items-end gap-2">
        <input
          ref={picker}
          type="file"
          multiple
          hidden
          aria-label={t('messages.attach')}
          onChange={(e) => {
            void choose(e.target.files)
            e.target.value = ''
          }}
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          disabled={files.length + uploading >= maxFiles}
          onClick={() => picker.current?.click()}
          aria-label={t('messages.attach')}
        >
          <PaperclipIcon />
        </Button>
        <Textarea
          value={body}
          maxLength={maxBody}
          rows={1}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder={t('messages.placeholder')}
          aria-label={t('messages.compose')}
          className="max-h-40 min-h-10 resize-none"
        />
        <Button
          type="submit"
          size="icon"
          disabled={(!body.trim() && files.length === 0) || sending.isLoading || uploading > 0}
          aria-label={t('messages.send')}
        >
          <SendIcon />
        </Button>
      </div>
    </form>
  )
}
