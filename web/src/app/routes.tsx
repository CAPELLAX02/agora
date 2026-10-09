import { Outlet, type RouteObject } from 'react-router'

import { SecurityPage } from '@/features/account/SecurityPage'
import { ProfilePage } from '@/features/account/ProfilePage'
import { AuthGate } from '@/features/auth/AuthGate'
import { ForcePasswordChangePage } from '@/features/auth/ForcePasswordChangePage'
import { ForgotPasswordPage } from '@/features/auth/ForgotPasswordPage'
import { GuestOnly, RequireAuth, RequirePasswordChanged } from '@/features/auth/guards'
import { LoginPage } from '@/features/auth/LoginPage'
import { ResetPasswordPage } from '@/features/auth/ResetPasswordPage'
import { DashboardPage } from '@/features/dashboard/DashboardPage'
import { AppLayout } from '@/features/shell/AppLayout'

import { CrashPage, NotFoundPage } from './ErrorPages'

/** routes, uygulamanın sayfa ağacıdır. */
export const routes: RouteObject[] = [
  {
    element: (
      <AuthGate>
        <Outlet />
      </AuthGate>
    ),
    errorElement: <CrashPage />,
    children: [
      {
        element: <GuestOnly />,
        children: [
          { path: 'giris', element: <LoginPage /> },
          { path: 'sifremi-unuttum', element: <ForgotPasswordPage /> },
        ],
      },
      // Bağlantı oturum açıkken de açılabilir (ör. ortak bilgisayarda başka biri açıkken).
      { path: 'sifre-sifirla', element: <ResetPasswordPage /> },
      {
        element: <RequireAuth />,
        children: [
          { path: 'parola-degistir', element: <ForcePasswordChangePage /> },
          {
            element: <RequirePasswordChanged />,
            children: [
              {
                element: <AppLayout />,
                children: [
                  { index: true, element: <DashboardPage /> },
                  { path: 'profil', element: <ProfilePage /> },
                  { path: 'guvenlik', element: <SecurityPage /> },
                  { path: '*', element: <NotFoundPage /> },
                ],
              },
            ],
          },
        ],
      },
    ],
  },
]
