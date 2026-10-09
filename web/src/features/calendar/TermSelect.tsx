import { useTranslation } from 'react-i18next'

import { useListTermsQuery } from '@/shared/api/generated'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'

import { termLabel } from './term'

/** TermSelect, akademik dönem seçicisidir. */
export function TermSelect({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const { t } = useTranslation()
  const terms = useListTermsQuery()
  return (
    <Select value={value} onValueChange={onChange} disabled={!terms.data}>
      <SelectTrigger className="sm:w-64" aria-label={t('calendar.term')}>
        <SelectValue placeholder={t('common.loading')} />
      </SelectTrigger>
      <SelectContent>
        {terms.data?.items.map((x) => (
          <SelectItem key={x.id} value={x.id}>
            {termLabel(t, x)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
