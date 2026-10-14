const inTermColors = [
  'bg-sky-500',
  'bg-violet-500',
  'bg-teal-500',
  'bg-amber-500',
  'bg-pink-500',
  'bg-indigo-500',
]

/** componentColors, bileşenlerin rengidir: dönem içi bileşenler sırayla, final ve bütünleme gül rengi. */
export function componentColors(components: { id: string; type: { category: string } }[]) {
  const colors = new Map(
    components
      .filter((c) => c.type.category === 'IN_TERM')
      .map((c, i) => [c.id, inTermColors[i % inTermColors.length]!]),
  )
  return (c: { id: string }) => colors.get(c.id) ?? 'bg-rose-500'
}
