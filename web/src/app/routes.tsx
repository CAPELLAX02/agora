import { Outlet, type RouteObject } from 'react-router'

import { SecurityPage } from '@/features/account/SecurityPage'
import { ProfilePage } from '@/features/account/ProfilePage'
import { AuthGate } from '@/features/auth/AuthGate'
import { ForcePasswordChangePage } from '@/features/auth/ForcePasswordChangePage'
import { ForgotPasswordPage } from '@/features/auth/ForgotPasswordPage'
import { GuestOnly, RequireAuth, RequirePasswordChanged, RequirePermission } from '@/features/auth/guards'
import { LoginPage } from '@/features/auth/LoginPage'
import { ResetPasswordPage } from '@/features/auth/ResetPasswordPage'
import { DashboardPage } from '@/features/dashboard/DashboardPage'
import { AppLayout } from '@/features/shell/AppLayout'
import { Spinner } from '@/shared/ui/spinner'

import { CrashPage, NotFoundPage } from './ErrorPages'

/**
 * routes, uygulamanın sayfa ağacıdır. Yönetim sayfaları ayrı paketlere bölünür (lazy):
 * çoğu kullanıcı onları hiç indirmez.
 */
export const routes: RouteObject[] = [
  {
    element: (
      <AuthGate>
        <Outlet />
      </AuthGate>
    ),
    errorElement: <CrashPage />,
    // Doğrudan bir yönetim sayfası açılınca o sayfanın paketi inene kadar gösterilir.
    hydrateFallbackElement: (
      <div className="grid min-h-svh place-items-center">
        <Spinner className="size-6 text-muted-foreground" />
      </div>
    ),
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
                  {
                    path: 'takvim',
                    lazy: async () => {
                      const { CalendarPage } = await import('@/features/calendar/CalendarPage')
                      return {
                        element: (
                          <RequirePermission permission="calendar:read">
                            <CalendarPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'dersler',
                    lazy: async () => {
                      const { CoursesPage } = await import('@/features/catalog/CoursesPage')
                      return {
                        element: (
                          <RequirePermission permission="course:read">
                            <CoursesPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'dersler/:id',
                    lazy: async () => {
                      const { CourseDetailPage } = await import('@/features/catalog/CourseDetailPage')
                      return {
                        element: (
                          <RequirePermission permission="course:read">
                            <CourseDetailPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ders-planim',
                    lazy: async () => {
                      const { MyCurriculumPage } = await import('@/features/curriculum/MyCurriculumPage')
                      return {
                        element: (
                          <RequirePermission permission="curriculum:read">
                            <MyCurriculumPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ders-planlari',
                    lazy: async () => {
                      const { CurriculaPage } = await import('@/features/curriculum/CurriculaPage')
                      return {
                        element: (
                          <RequirePermission permission="curriculum:read">
                            <CurriculaPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'yonetim/kullanicilar',
                    lazy: async () => {
                      const { UsersPage } = await import('@/features/admin/UsersPage')
                      return {
                        element: (
                          <RequirePermission permission="user:read">
                            <UsersPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'yonetim/kullanicilar/:id',
                    lazy: async () => {
                      const { UserDetailPage } = await import('@/features/admin/UserDetailPage')
                      return {
                        element: (
                          <RequirePermission permission="user:read">
                            <UserDetailPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'yonetim/denetim',
                    lazy: async () => {
                      const { AuditPage } = await import('@/features/admin/AuditPage')
                      return {
                        element: (
                          <RequirePermission permission="audit:read">
                            <AuditPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
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
