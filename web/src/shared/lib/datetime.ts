/** toLocalInput, ISO zamanı datetime-local alanının değerine çevirir (yerel saat); boşsa ''. */
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) {
    return ''
  }
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** fromLocalInput, datetime-local değerini ISO zamana çevirir; boşsa null. */
export function fromLocalInput(v: string): string | null {
  return v ? new Date(v).toISOString() : null
}
