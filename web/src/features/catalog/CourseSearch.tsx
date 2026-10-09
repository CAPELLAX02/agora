import { CheckIcon, SearchIcon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useListCoursesQuery, type Course } from '@/shared/api/generated'
import { useDebounced } from '@/shared/lib/useDebounced'
import { useLocalized } from '@/shared/lib/localized'
import { Button } from '@/shared/ui/button'
import { Input } from '@/shared/ui/input'

/**
 * CourseSearch, katalogdan kod ya da adla ders seçtirir. Seçilen ders kutunun yerine
 * gösterilir; değiştirmek için seçim kaldırılır.
 */
export function CourseSearch({
  id,
  value,
  onChange,
  invalid,
  describedBy,
}: {
  id?: string
  value: Course | null
  onChange: (course: Course | null) => void
  invalid?: boolean
  describedBy?: string | undefined
}) {
  const { t } = useTranslation()
  const localized = useLocalized()
  const [query, setQuery] = useState('')
  const q = useDebounced(query.trim())
  const { data, isFetching } = useListCoursesQuery({ q, limit: 8 }, { skip: q.length < 2 || value !== null })

  if (value) {
    return (
      <div className="flex items-center justify-between gap-2 rounded-md border px-3 py-2 text-sm">
        <span>
          <span className="font-mono text-xs text-muted-foreground">{value.code}</span> {localized(value)}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-7"
          onClick={() => onChange(null)}
          aria-label={t('catalog.search.clear')}
        >
          <XIcon />
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="relative">
        <SearchIcon className="absolute top-2.5 left-3 size-4 text-muted-foreground" aria-hidden />
        <Input
          id={id}
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('catalog.searchPlaceholder')}
          className="pl-9"
          aria-invalid={invalid}
          aria-describedby={describedBy}
          autoComplete="off"
        />
      </div>
      {q.length >= 2 && (
        <ul className="max-h-56 overflow-y-auto rounded-md border" aria-label={t('catalog.search.results')}>
          {data?.items.length === 0 && !isFetching && (
            <li className="px-3 py-2 text-sm text-muted-foreground">{t('catalog.empty')}</li>
          )}
          {data?.items.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                onClick={() => onChange(c)}
              >
                <CheckIcon className="size-3.5 opacity-0" aria-hidden />
                <span className="font-mono text-xs text-muted-foreground">{c.code}</span>
                <span className="flex-1">{localized(c)}</span>
                <span className="text-xs text-muted-foreground">
                  {c.theory_hours}-{c.practice_hours} · {c.ects} {t('catalog.ects')}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
