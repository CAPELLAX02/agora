import { SearchIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useSearchInstructorsQuery } from '@/shared/api/generated'
import { useDebounced } from '@/shared/lib/useDebounced'
import { useLocalized } from '@/shared/lib/localized'
import { cn } from '@/shared/lib/utils'
import { Input } from '@/shared/ui/input'

/** InstructorSearch, görevdeki akademik personeli adına ya da sicil numarasına göre arar. */
export function InstructorSearch({
  value,
  onChange,
}: {
  value: string
  onChange: (staffId: string) => void
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [query, setQuery] = useState('')
  const q = useDebounced(query.trim())
  const { data } = useSearchInstructorsQuery({ q, limit: 8 }, { skip: q.length < 2 })

  return (
    <div className="w-full space-y-2 sm:max-w-md">
      <div className="relative">
        <SearchIcon className="absolute top-2.5 left-3 size-4 text-muted-foreground" aria-hidden />
        <Input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('schedule.instructorPlaceholder')}
          aria-label={t('schedule.views.instructor')}
          className="pl-9"
        />
      </div>
      {q.length >= 2 && data && (
        <ul className="rounded-md border" aria-label={t('catalog.search.results')}>
          {data.items.length === 0 && (
            <li className="px-3 py-2 text-sm text-muted-foreground">{t('schedule.noInstructor')}</li>
          )}
          {data.items.map((s) => (
            <li key={s.staff_id}>
              <button
                type="button"
                aria-pressed={s.staff_id === value}
                className={cn(
                  'flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none',
                  s.staff_id === value && 'bg-accent',
                )}
                onClick={() => {
                  onChange(s.staff_id)
                  setQuery('')
                }}
              >
                <span>{[s.title, s.first_name, s.last_name].filter(Boolean).join(' ')}</span>
                {s.department && (
                  <span className="text-xs text-muted-foreground">{localized(s.department)}</span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
