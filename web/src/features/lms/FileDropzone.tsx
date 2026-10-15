import { CircleAlertIcon, FileIcon, UploadIcon, XIcon } from 'lucide-react'
import { useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { formatBytes, UploadError, useUpload } from '@/shared/api/upload'
import { cn } from '@/shared/lib/utils'
import { Spinner } from '@/shared/ui/spinner'

export type UploadedFile = { id: string; name: string; size: number }
type Pending = { key: string; name: string; progress: number; error?: string }

/**
 * FileDropzone, dosyaları sürükle-bırak ya da seçiciyle yükler: her dosya hemen ve ilerleme
 * çubuğuyla yüklenir, sunucunun reddi (tür, içerik, boyut) dosyanın yanında yazar.
 */
export function FileDropzone({
  files,
  onChange,
  multiple,
  max = 50,
  accept,
  hint,
}: {
  files: UploadedFile[]
  onChange: (files: UploadedFile[]) => void
  multiple: boolean
  /** max, çoklu yüklemede en çok dosya sayısıdır. */
  max?: number
  /** accept, seçicide gösterilen türlerdir (".pdf,.docx"); sunucu ayrıca denetler. */
  accept?: string
  /** hint, sınırların açıklamasıdır; verilmezse platformun genel sınırları yazar. */
  hint?: string
}) {
  const { t, i18n } = useTranslation()
  const upload = useUpload()
  const [pending, setPending] = useState<Pending[]>([])
  const [dragging, setDragging] = useState(false)
  const locale = i18n.language.startsWith('en') ? 'en-GB' : 'tr-TR'

  const add = (list: FileList | null) => {
    const chosen = Array.from(list ?? []).slice(0, multiple ? Math.max(0, max - files.length) : 1)
    let current = multiple ? files : []
    if (!multiple) {
      onChange([])
    }
    for (const file of chosen) {
      const key = `${file.name}-${file.size}-${Math.random()}`
      setPending((p) => [...p, { key, name: file.name, progress: 0 }])
      upload(file, (loaded, total) =>
        setPending((p) => p.map((x) => (x.key === key ? { ...x, progress: total ? loaded / total : 0 } : x))),
      )
        .then((info) => {
          setPending((p) => p.filter((x) => x.key !== key))
          current = [...current, { id: info.id, name: info.name, size: info.size }]
          onChange(current)
        })
        .catch((err: unknown) => {
          const message =
            err instanceof UploadError && err.code !== 'NETWORK'
              ? err.message
              : err instanceof UploadError
                ? t('errors.network')
                : t('lms.upload.failed')
          setPending((p) => p.map((x) => (x.key === key ? { ...x, error: message, progress: 0 } : x)))
        })
    }
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    add(e.dataTransfer.files)
  }

  return (
    <div className="space-y-2">
      <label
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={cn(
          'flex cursor-pointer flex-col items-center gap-1.5 rounded-xl border-2 border-dashed border-sky-400/50 bg-sky-50/50 px-4 py-5 text-center transition-colors hover:bg-sky-50 dark:bg-sky-500/5',
          dragging && 'border-sky-500 bg-sky-100/60',
        )}
      >
        <UploadIcon className="size-5 text-sky-600" aria-hidden />
        <span className="text-sm font-semibold">
          {t(multiple ? 'lms.upload.dropMany' : 'lms.upload.dropOne')}
        </span>
        <span className="text-xs text-muted-foreground">{hint ?? t('lms.upload.limits')}</span>
        <input
          type="file"
          multiple={multiple}
          accept={accept}
          className="sr-only"
          aria-label={t('lms.upload.choose')}
          onChange={(e) => {
            add(e.target.files)
            e.target.value = ''
          }}
        />
      </label>
      <ul className="space-y-1.5">
        {files.map((f) => (
          <li key={f.id} className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm">
            <FileIcon className="size-4 shrink-0 text-sky-600" aria-hidden />
            <span className="min-w-0 flex-1 truncate">{f.name}</span>
            <span className="text-xs text-muted-foreground tabular-nums">{formatBytes(f.size, locale)}</span>
            <button
              type="button"
              aria-label={t('lms.upload.remove', { name: f.name })}
              onClick={() => onChange(files.filter((x) => x.id !== f.id))}
              className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <XIcon className="size-3.5" />
            </button>
          </li>
        ))}
        {pending.map((p) => (
          <li key={p.key} className="space-y-1 rounded-lg border px-3 py-2 text-sm" aria-live="polite">
            <span className="flex items-center gap-2">
              {p.error ? (
                <CircleAlertIcon className="size-4 shrink-0 text-rose-600" aria-hidden />
              ) : (
                <Spinner className="size-4" />
              )}
              <span className="min-w-0 flex-1 truncate">{p.name}</span>
              {p.error && (
                <button
                  type="button"
                  aria-label={t('lms.upload.dismiss')}
                  onClick={() => setPending((list) => list.filter((x) => x.key !== p.key))}
                  className="rounded p-1 text-muted-foreground hover:bg-muted"
                >
                  <XIcon className="size-3.5" />
                </button>
              )}
            </span>
            {p.error ? (
              <span className="block text-xs text-rose-700 dark:text-rose-300">{p.error}</span>
            ) : (
              <span
                role="progressbar"
                aria-label={p.name}
                aria-valuenow={Math.round(p.progress * 100)}
                aria-valuemin={0}
                aria-valuemax={100}
                className="block h-1.5 overflow-hidden rounded-full bg-muted"
              >
                <span
                  className="block h-full bg-sky-500 transition-all"
                  style={{ width: `${p.progress * 100}%` }}
                />
              </span>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}
