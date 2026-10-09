import { HistoryIcon, KeyRoundIcon, MonitorSmartphoneIcon, ShieldCheckIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'

import { PageHeader } from '@/shared/ui/page-header'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/ui/tabs'

import { HistorySection } from './HistorySection'
import { MfaSection } from './MfaSection'
import { PasswordSection } from './PasswordSection'
import { SessionsSection } from './SessionsSection'

const tabs = ['password', 'mfa', 'sessions', 'history'] as const
type Tab = (typeof tabs)[number]
const icons = {
  password: KeyRoundIcon,
  mfa: ShieldCheckIcon,
  sessions: MonitorSmartphoneIcon,
  history: HistoryIcon,
}

/** SecurityPage, hesap güvenliği ayarlarıdır. Seçili sekme adreste (?tab=) tutulur: bağlantıyla açılabilir. */
export function SecurityPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const requested = params.get('tab') as Tab | null
  const tab: Tab = requested && tabs.includes(requested) ? requested : 'password'

  return (
    <>
      <PageHeader title={t('security.title')} description={t('security.description')} />
      <Tabs value={tab} onValueChange={(v) => setParams({ tab: v }, { replace: true })} className="gap-6">
        <TabsList className="h-auto w-full flex-wrap justify-start sm:w-fit">
          {tabs.map((key) => {
            const Icon = icons[key]
            return (
              <TabsTrigger key={key} value={key} className="flex-none">
                <Icon />
                {t(`security.tabs.${key}`)}
              </TabsTrigger>
            )
          })}
        </TabsList>
        <TabsContent value="password">
          <PasswordSection />
        </TabsContent>
        <TabsContent value="mfa">
          <MfaSection />
        </TabsContent>
        <TabsContent value="sessions">
          <SessionsSection />
        </TabsContent>
        <TabsContent value="history">
          <HistorySection />
        </TabsContent>
      </Tabs>
    </>
  )
}
