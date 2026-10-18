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
                    path: 'bildirimler',
                    lazy: async () => {
                      const { NotificationsPage } = await import('@/features/notifications/NotificationsPage')
                      return { element: <NotificationsPage /> }
                    },
                  },
                  {
                    path: 'bildirimler/tercihler',
                    lazy: async () => {
                      const { NotificationPreferencesPage } =
                        await import('@/features/notifications/NotificationPreferencesPage')
                      return { element: <NotificationPreferencesPage /> }
                    },
                  },
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
                    path: 'notlarim',
                    lazy: async () => {
                      const { GradesPage } = await import('@/features/grades/GradesPage')
                      return {
                        element: (
                          <RequirePermission permission="grade:read_own">
                            <GradesPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'transkript/:programId?',
                    lazy: async () => {
                      const { TranscriptPage } = await import('@/features/transcript/TranscriptPage')
                      return {
                        element: (
                          <RequirePermission permission={['transcript:read_own', 'transcript:read_scoped']}>
                            <TranscriptPage />
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
                    path: 'program',
                    lazy: async () => {
                      const { SchedulePage } = await import('@/features/schedule/SchedulePage')
                      return {
                        element: (
                          <RequirePermission permission="course:read">
                            <SchedulePage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'verdigim-dersler',
                    lazy: async () => {
                      const { MyTeachingPage } = await import('@/features/schedule/MyTeachingPage')
                      return {
                        element: (
                          <RequirePermission permission="assessment_plan:manage">
                            <MyTeachingPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  // Ders alanında yetki sunucuda, alan başına (üyelik) denetlenir.
                  {
                    path: 'derslerim',
                    lazy: async () => {
                      const { MySpacesPage } = await import('@/features/lms/MySpacesPage')
                      return { element: <MySpacesPage /> }
                    },
                  },
                  {
                    path: 'derslerim/:id',
                    lazy: async () => {
                      const { SpacePage } = await import('@/features/lms/SpacePage')
                      return { element: <SpacePage /> }
                    },
                  },
                  {
                    path: 'derslerim/:id/etkinlik/:activityId',
                    lazy: async () => {
                      const { ActivityPage } = await import('@/features/lms/ActivityPage')
                      return { element: <ActivityPage /> }
                    },
                  },
                  {
                    path: 'derslerim/:id/etkinlik/:activityId/konu/:threadId',
                    lazy: async () => {
                      const { ThreadPage } = await import('@/features/lms/ThreadPage')
                      return { element: <ThreadPage /> }
                    },
                  },
                  {
                    path: 'not-duzeltme',
                    lazy: async () => {
                      const { GradeChangesPage } = await import('@/features/gradechanges/GradeChangesPage')
                      return {
                        element: (
                          <RequirePermission permission="grade_change:request">
                            <GradeChangesPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'not-duzeltme-onaylari',
                    lazy: async () => {
                      const { ReviewPage } = await import('@/features/gradechanges/ReviewPage')
                      return {
                        element: (
                          <RequirePermission permission="grade_change:approve">
                            <ReviewPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ders-acma',
                    lazy: async () => {
                      const { OfferingsPage } = await import('@/features/offering/OfferingsPage')
                      return {
                        element: (
                          <RequirePermission permission="offering:manage">
                            <OfferingsPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ders-acma/:id',
                    lazy: async () => {
                      const { OfferingDetailPage } = await import('@/features/offering/OfferingDetailPage')
                      return {
                        element: (
                          <RequirePermission permission="offering:manage">
                            <OfferingDetailPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'subeler/:id/degerlendirme',
                    lazy: async () => {
                      const { AssessmentPlanPage } = await import('@/features/grading/AssessmentPlanPage')
                      return {
                        element: (
                          <RequirePermission permission="course:read">
                            <AssessmentPlanPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'subeler/:id/notlar',
                    lazy: async () => {
                      const { GradebookPage } = await import('@/features/grading/GradebookPage')
                      return {
                        element: (
                          <RequirePermission permission="course:read">
                            <GradebookPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'subeler/:id/harf-notlari',
                    lazy: async () => {
                      const { LetterGradesPage } = await import('@/features/grading/LetterGradesPage')
                      return {
                        element: (
                          <RequirePermission permission="course:read">
                            <LetterGradesPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'not-olcegi',
                    lazy: async () => {
                      const { GradeScalePage } = await import('@/features/grading/GradeScalePage')
                      return {
                        element: (
                          <RequirePermission permission="curriculum:read">
                            <GradeScalePage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ders-secimi',
                    lazy: async () => {
                      const { RegistrationPage } = await import('@/features/registration/RegistrationPage')
                      return {
                        element: (
                          <RequirePermission permission="registration:manage_own">
                            <RegistrationPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ders-secimi/belge',
                    lazy: async () => {
                      const { RegistrationReportPage } =
                        await import('@/features/registration/RegistrationReportPage')
                      return {
                        element: (
                          <RequirePermission permission="registration:manage_own">
                            <RegistrationReportPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'danismanlik',
                    lazy: async () => {
                      const { AdviseeRegistrationsPage } =
                        await import('@/features/registration/advisor/AdviseeRegistrationsPage')
                      return {
                        element: (
                          <RequirePermission permission="registration:approve">
                            <AdviseeRegistrationsPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'danismanlik/kayitlar/:id',
                    lazy: async () => {
                      const { RegistrationReviewPage } =
                        await import('@/features/registration/advisor/RegistrationReviewPage')
                      return {
                        element: (
                          <RequirePermission
                            permission={[
                              'registration:approve',
                              'registration:override',
                              'registration:approve_add_drop',
                              'person:read',
                            ]}
                          >
                            <RegistrationReviewPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ekle-birak-onaylari',
                    lazy: async () => {
                      const { AddDropQueuePage } =
                        await import('@/features/registration/advisor/AddDropQueuePage')
                      return {
                        element: (
                          <RequirePermission
                            permission={['registration:approve', 'registration:approve_add_drop']}
                          >
                            <AddDropQueuePage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ogrenciler',
                    lazy: async () => {
                      const { StudentsPage } = await import('@/features/students/StudentsPage')
                      return {
                        element: (
                          <RequirePermission permission="person:read">
                            <StudentsPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'ogrenciler/:id',
                    lazy: async () => {
                      const { StudentDetailPage } = await import('@/features/students/StudentDetailPage')
                      return {
                        element: (
                          <RequirePermission permission="person:read">
                            <StudentDetailPage />
                          </RequirePermission>
                        ),
                      }
                    },
                  },
                  {
                    path: 'kayit-engelleri',
                    lazy: async () => {
                      const { HoldsPage } = await import('@/features/students/HoldsPage')
                      return {
                        element: (
                          <RequirePermission permission="hold:manage">
                            <HoldsPage />
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
