// Kayıtlı temayı (light, dark, system) sayfa çizilmeden önce uygular.
// Anahtar ve değerler src/app/theme.tsx ile aynı olmalı.
;(function () {
  var theme = 'system'
  try {
    theme = localStorage.getItem('agora.theme') || 'system'
  } catch (e) {
    // Gizli pencere ya da engellenmiş depolama: sistem teması kullanılır.
  }
  var dark = theme === 'dark' || (theme === 'system' && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
})()
