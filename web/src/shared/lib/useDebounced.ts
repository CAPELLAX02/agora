import { useEffect, useState } from 'react'

/** useDebounced, değer delay süresince değişmeyince güncellenen bir kopya döndürür (arama kutuları için). */
export function useDebounced<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(id)
  }, [value, delay])
  return debounced
}
