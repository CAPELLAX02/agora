import { useState } from 'react'
import { Provider } from 'react-redux'
import { createBrowserRouter } from 'react-router'
import { RouterProvider } from 'react-router/dom'

import { Toaster } from '@/shared/ui/sonner'
import { TooltipProvider } from '@/shared/ui/tooltip'

import { routes } from './routes'
import { setupStore } from './store'
import { ThemeProvider } from './ThemeProvider'

export function App() {
  const [store] = useState(setupStore)
  const [router] = useState(() => createBrowserRouter(routes))
  return (
    <Provider store={store}>
      <ThemeProvider>
        <TooltipProvider delayDuration={300}>
          <RouterProvider router={router} />
          {/* Sağ altta: üst çubuktaki hesap menüsünün ve dil/tema düğmelerinin üstünü örtmez. */}
          <Toaster position="bottom-right" richColors closeButton />
        </TooltipProvider>
      </ThemeProvider>
    </Provider>
  )
}
