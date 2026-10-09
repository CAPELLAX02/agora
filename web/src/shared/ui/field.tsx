import { useId, type ReactNode } from 'react'

import { cn } from '@/shared/lib/utils'

import { Label } from './label'

type FieldProps = {
  label: ReactNode
  /** error, alanın altında gösterilen hata mesajıdır. Doluysa alan aria-invalid olur. */
  error?: string | undefined
  /** hint, hata yokken gösterilen yardımcı metindir. */
  hint?: ReactNode
  className?: string
  /** children, alanın girdisini kimlik ve erişilebilirlik özellikleriyle çizer. */
  children: (props: {
    id: string
    'aria-invalid': boolean
    'aria-describedby': string | undefined
  }) => ReactNode
}

/**
 * Field, bir form alanının etiketini, girdisini ve hata/yardım metnini erişilebilir
 * biçimde birbirine bağlar (label for, aria-describedby, aria-invalid).
 */
export function Field({ label, error, hint, className, children }: FieldProps) {
  const id = useId()
  const messageId = `${id}-message`
  const message = error ?? hint
  return (
    <div className={cn('grid min-w-0 gap-2', className)} data-invalid={error ? true : undefined}>
      <Label htmlFor={id}>{label}</Label>
      {children({ id, 'aria-invalid': Boolean(error), 'aria-describedby': message ? messageId : undefined })}
      {message && (
        <p id={messageId} className={cn('text-sm', error ? 'text-destructive' : 'text-muted-foreground')}>
          {message}
        </p>
      )}
    </div>
  )
}
