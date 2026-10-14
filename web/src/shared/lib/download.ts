/** saveFile, bir Blob'u tarayıcıda dosya olarak indirir. */
export function saveFile(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  // İndirme başladıktan sonra adres serbest bırakılır.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
