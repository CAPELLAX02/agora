import type { ScheduleEntry } from '@/shared/api/generated'

/** minutes, "09:50" biçimindeki saati gün başından dakikaya çevirir. */
export function minutes(clock: string): number {
  const [h = 0, m = 0] = clock.split(':').map(Number)
  return h * 60 + m
}

/** roomLabel, oturumun dersliğini "MUH-A A103" biçiminde döndürür; çevrim içi oturumda boş. */
export function roomLabel(e: Pick<ScheduleEntry, 'classroom'>): string | null {
  return e.classroom ? `${e.classroom.building_code} ${e.classroom.code}` : null
}

export type Placed = { entry: ScheduleEntry; lane: number; lanes: number }

/**
 * placeEntries, aynı gün zaman olarak çakışan oturumları yan yana şeritlere yerleştirir.
 * Birbirine dokunan (çakışan) oturumlar bir küme oluşturur; kümenin şerit sayısı en çok
 * kaç oturumun aynı anda sürdüğüdür, her oturum boş ilk şeride oturur.
 */
export function placeEntries(entries: ScheduleEntry[]): Placed[] {
  const out: Placed[] = []
  const byDay = new Map<number, ScheduleEntry[]>()
  for (const e of entries) {
    byDay.set(e.day_of_week, [...(byDay.get(e.day_of_week) ?? []), e])
  }
  for (const list of byDay.values()) {
    const sorted = [...list].sort((a, b) => minutes(a.start_time) - minutes(b.start_time))
    let cluster: Placed[] = []
    let laneEnds: number[] = []
    let clusterEnd = -1
    const flush = () => {
      for (const p of cluster) {
        p.lanes = laneEnds.length
      }
      out.push(...cluster)
      cluster = []
      laneEnds = []
    }
    for (const e of sorted) {
      const start = minutes(e.start_time)
      if (start >= clusterEnd) {
        flush()
      }
      let lane = laneEnds.findIndex((end) => end <= start)
      if (lane === -1) {
        lane = laneEnds.length
        laneEnds.push(0)
      }
      laneEnds[lane] = minutes(e.end_time)
      clusterEnd = Math.max(clusterEnd, minutes(e.end_time))
      cluster.push({ entry: e, lane, lanes: 0 })
    }
    flush()
  }
  return out
}

/** instructorNames, oturumun öğretim elemanlarını unvanlarıyla sıralar (sorumlu önce). */
export function instructorNames(e: Pick<ScheduleEntry, 'instructors'>): string {
  return e.instructors
    .filter((i) => i.role !== 'ASSISTANT')
    .map((i) => [i.title, i.first_name, i.last_name].filter(Boolean).join(' '))
    .join(', ')
}

export type Day = 1 | 2 | 3 | 4 | 5 | 6 | 7

/** day, ISO gün numarasını (1 pazartesi ... 7 pazar) çeviri anahtarlarında kullanılacak tipe daraltır. */
export function day(n: number): Day {
  return (n >= 1 && n <= 7 ? n : 1) as Day
}
