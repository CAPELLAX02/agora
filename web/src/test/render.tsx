import { render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Provider } from 'react-redux'
import { createMemoryRouter, RouterProvider } from 'react-router'

import { routes } from '@/app/routes'
import { setupStore } from '@/app/store'
import { ThemeProvider } from '@/app/ThemeProvider'
import { Toaster } from '@/shared/ui/sonner'
import { TooltipProvider } from '@/shared/ui/tooltip'

/** renderApp, bütün uygulamayı verilen adresle (bellek içi router) çizer. */
export function renderApp(path = '/') {
  const store = setupStore()
  const router = createMemoryRouter(routes, { initialEntries: [path] })
  const user = userEvent.setup()
  const view = render(
    <Provider store={store}>
      <ThemeProvider>
        <TooltipProvider>
          <RouterProvider router={router} />
          <Toaster />
        </TooltipProvider>
      </ThemeProvider>
    </Provider>,
  )
  return { ...view, store, router, user }
}
