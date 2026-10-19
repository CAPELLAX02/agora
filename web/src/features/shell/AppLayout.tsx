import { MenuIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Outlet } from 'react-router'

import { Brand } from '@/shared/ui/brand'
import { Button } from '@/shared/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetTitle, SheetTrigger } from '@/shared/ui/sheet'

import { NotificationBell } from '@/features/notifications/NotificationBell'

import { Footer } from './Footer'
import { MfaBanner } from './MfaBanner'
import { PreferenceButtons } from './Preferences'
import { Sidebar, SidebarNav } from './Sidebar'
import { UserMenu } from './UserMenu'

/** AppLayout, oturum açmış kullanıcının uygulama kabuğudur: yan menü, üst çubuk, içerik, altbilgi. */
export function AppLayout() {
  const { t } = useTranslation()
  const [menuOpen, setMenuOpen] = useState(false)
  return (
    <div className="flex min-h-svh">
      <a
        href="#main"
        className="sr-only z-50 rounded-md bg-primary px-3 py-2 text-primary-foreground focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
      >
        {t('nav.skipToContent')}
      </a>
      <div className="contents print:hidden">
        <Sidebar />
      </div>
      <div className="flex min-w-0 flex-1 flex-col app-glow">
        <header className="sticky top-0 z-30 flex h-16 items-center gap-2 border-b border-border/60 bg-background/75 px-4 backdrop-blur-xl md:px-6 print:hidden">
          <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
            <SheetTrigger asChild>
              <Button variant="ghost" size="icon" className="lg:hidden" aria-label={t('nav.openMenu')}>
                <MenuIcon />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" closeLabel={t('common.close')} className="bg-sidebar py-5">
              <SheetTitle className="px-6">
                <Brand />
              </SheetTitle>
              <SheetDescription className="sr-only">{t('nav.main')}</SheetDescription>
              <SidebarNav onNavigate={() => setMenuOpen(false)} />
            </SheetContent>
          </Sheet>
          <Brand compact className="lg:hidden" />
          <div className="flex-1" />
          <PreferenceButtons />
          <NotificationBell />
          <UserMenu />
        </header>
        <div className="print:hidden">
          <MfaBanner />
        </div>
        <main id="main" tabIndex={-1} className="flex-1 px-4 py-7 outline-none md:px-6 lg:px-8">
          <div className="mx-auto max-w-6xl">
            <Outlet />
          </div>
        </main>
        <Footer />
      </div>
    </div>
  )
}
