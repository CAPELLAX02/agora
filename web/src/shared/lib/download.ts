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

/**
 * openDownload, imzalı bir indirme adresini açar. Sunucu dosyayı her zaman ek (attachment)
 * olarak verdiği için sayfadan çıkılmaz; dosya tarayıcı belleğine alınmadan iner.
 */
export function openDownload(url: string) {
  const a = document.createElement('a')
  a.href = url
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
}
