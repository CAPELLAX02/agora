/** hours, dersin haftalık teorik ve uygulama saatini ders planlarındaki gibi gösterir: "3-2". */
export function hours(c: { theory_hours: number; practice_hours: number }): string {
  return `${c.theory_hours}-${c.practice_hours}`
}
