/** ifMatch, kaydın sürümünü If-Match başlığının beklediği ETag biçimine çevirir: "3". */
export function ifMatch(version: number): string {
  return `"${version}"`
}
