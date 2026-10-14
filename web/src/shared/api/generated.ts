import { baseApi as api } from './baseApi'
export const addTagTypes = [
  'Sa\u011Fl\u0131k',
  'Kimlik do\u011Frulama',
  'Hesap',
  'Kullan\u0131c\u0131lar',
  'Denetim',
  'Akademik takvim',
  'Ders katalo\u011Fu',
  'M\u00FCfredat',
  'Not \u00F6l\u00E7e\u011Fi ve y\u00F6netmelik',
  'Ders a\u00E7ma',
  'De\u011Ferlendirme',
  'Organizasyon',
  '\u00D6\u011Frenciler',
  'Ders se\u00E7me',
] as const
const injectedRtkApi = api
  .enhanceEndpoints({
    addTagTypes,
  })
  .injectEndpoints({
    endpoints: (build) => ({
      healthz: build.query<HealthzApiResponse, HealthzApiArg>({
        query: () => ({ url: `/healthz` }),
        providesTags: ['Sa\u011Fl\u0131k'],
      }),
      readyz: build.query<ReadyzApiResponse, ReadyzApiArg>({
        query: () => ({ url: `/readyz` }),
        providesTags: ['Sa\u011Fl\u0131k'],
      }),
      login: build.mutation<LoginApiResponse, LoginApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/auth/login`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'X-Agora-Client': queryArg['X-Agora-Client'],
          },
        }),
        invalidatesTags: ['Kimlik do\u011Frulama'],
      }),
      verifyMfa: build.mutation<VerifyMfaApiResponse, VerifyMfaApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/auth/mfa/verify`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'X-Agora-Client': queryArg['X-Agora-Client'],
          },
        }),
        invalidatesTags: ['Kimlik do\u011Frulama'],
      }),
      refresh: build.mutation<RefreshApiResponse, RefreshApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/auth/refresh`,
          method: 'POST',
          body: queryArg.refreshTokenBody,
          headers: {
            'X-Agora-Client': queryArg['X-Agora-Client'],
          },
        }),
        invalidatesTags: ['Kimlik do\u011Frulama'],
      }),
      logout: build.mutation<LogoutApiResponse, LogoutApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/auth/logout`,
          method: 'POST',
          body: queryArg.refreshTokenBody,
          headers: {
            'X-Agora-Client': queryArg['X-Agora-Client'],
          },
        }),
        invalidatesTags: ['Kimlik do\u011Frulama'],
      }),
      forgotPassword: build.mutation<ForgotPasswordApiResponse, ForgotPasswordApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/auth/password/forgot`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Kimlik do\u011Frulama'],
      }),
      verifyResetToken: build.mutation<VerifyResetTokenApiResponse, VerifyResetTokenApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/auth/password/reset/verify`,
          method: 'POST',
          body: queryArg.resetTokenBody,
        }),
        invalidatesTags: ['Kimlik do\u011Frulama'],
      }),
      resetPassword: build.mutation<ResetPasswordApiResponse, ResetPasswordApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/auth/password/reset`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Kimlik do\u011Frulama'],
      }),
      getMe: build.query<GetMeApiResponse, GetMeApiArg>({
        query: () => ({ url: `/api/v1/me` }),
        providesTags: ['Hesap'],
      }),
      getMyMfa: build.query<GetMyMfaApiResponse, GetMyMfaApiArg>({
        query: () => ({ url: `/api/v1/me/mfa` }),
        providesTags: ['Hesap'],
      }),
      startMfaSetup: build.mutation<StartMfaSetupApiResponse, StartMfaSetupApiArg>({
        query: () => ({ url: `/api/v1/me/mfa/setup`, method: 'POST' }),
        invalidatesTags: ['Hesap'],
      }),
      enableMfa: build.mutation<EnableMfaApiResponse, EnableMfaApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/me/mfa/enable`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Hesap'],
      }),
      disableMfa: build.mutation<DisableMfaApiResponse, DisableMfaApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/me/mfa/disable`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Hesap'],
      }),
      regenerateRecoveryCodes: build.mutation<
        RegenerateRecoveryCodesApiResponse,
        RegenerateRecoveryCodesApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/me/mfa/recovery-codes`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Hesap'],
      }),
      changePassword: build.mutation<ChangePasswordApiResponse, ChangePasswordApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/me/password`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Hesap'],
      }),
      listMySessions: build.query<ListMySessionsApiResponse, ListMySessionsApiArg>({
        query: () => ({ url: `/api/v1/me/sessions` }),
        providesTags: ['Hesap'],
      }),
      endOtherSessions: build.mutation<EndOtherSessionsApiResponse, EndOtherSessionsApiArg>({
        query: () => ({ url: `/api/v1/me/sessions`, method: 'DELETE' }),
        invalidatesTags: ['Hesap'],
      }),
      endSession: build.mutation<EndSessionApiResponse, EndSessionApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/me/sessions/${queryArg.id}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['Hesap'],
      }),
      listMySecurityEvents: build.query<ListMySecurityEventsApiResponse, ListMySecurityEventsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/me/security-events`,
          params: {
            type: queryArg['type'],
            from: queryArg['from'],
            to: queryArg.to,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['Hesap'],
      }),
      getMyPermissions: build.query<GetMyPermissionsApiResponse, GetMyPermissionsApiArg>({
        query: () => ({ url: `/api/v1/me/permissions` }),
        providesTags: ['Hesap'],
      }),
      listUsers: build.query<ListUsersApiResponse, ListUsersApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/users`,
          params: {
            q: queryArg.q,
            status: queryArg.status,
            kind: queryArg.kind,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['Kullan\u0131c\u0131lar'],
      }),
      createUser: build.mutation<CreateUserApiResponse, CreateUserApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/users`,
          method: 'POST',
          body: queryArg.createUserRequest,
        }),
        invalidatesTags: ['Kullan\u0131c\u0131lar'],
      }),
      setUserStatus: build.mutation<SetUserStatusApiResponse, SetUserStatusApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/users/${queryArg.id}/status`,
          method: 'PUT',
          body: queryArg.body,
        }),
        invalidatesTags: ['Kullan\u0131c\u0131lar'],
      }),
      resendActivationEmail: build.mutation<ResendActivationEmailApiResponse, ResendActivationEmailApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/users/${queryArg.id}/activation-email`,
          method: 'POST',
        }),
        invalidatesTags: ['Kullan\u0131c\u0131lar'],
      }),
      sendPasswordResetEmail: build.mutation<SendPasswordResetEmailApiResponse, SendPasswordResetEmailApiArg>(
        {
          query: (queryArg) => ({
            url: `/api/v1/users/${queryArg.id}/password-reset-email`,
            method: 'POST',
          }),
          invalidatesTags: ['Kullan\u0131c\u0131lar'],
        },
      ),
      resetUserMfa: build.mutation<ResetUserMfaApiResponse, ResetUserMfaApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/users/${queryArg.id}/mfa/reset`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Kullan\u0131c\u0131lar'],
      }),
      listRoles: build.query<ListRolesApiResponse, ListRolesApiArg>({
        query: () => ({ url: `/api/v1/roles` }),
        providesTags: ['Kullan\u0131c\u0131lar'],
      }),
      listUserRoles: build.query<ListUserRolesApiResponse, ListUserRolesApiArg>({
        query: (queryArg) => ({ url: `/api/v1/users/${queryArg.id}/roles` }),
        providesTags: ['Kullan\u0131c\u0131lar'],
      }),
      assignRole: build.mutation<AssignRoleApiResponse, AssignRoleApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/users/${queryArg.id}/roles`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Kullan\u0131c\u0131lar'],
      }),
      endRoleAssignment: build.mutation<EndRoleAssignmentApiResponse, EndRoleAssignmentApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/users/${queryArg.id}/roles/${queryArg.assignment}/end`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Kullan\u0131c\u0131lar'],
      }),
      getUser: build.query<GetUserApiResponse, GetUserApiArg>({
        query: (queryArg) => ({ url: `/api/v1/users/${queryArg.id}` }),
        providesTags: ['Kullan\u0131c\u0131lar'],
      }),
      listAuditLog: build.query<ListAuditLogApiResponse, ListAuditLogApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/audit/log`,
          params: {
            actor_user_id: queryArg.actorUserId,
            action: queryArg.action,
            entity_type: queryArg.entityType,
            entity_id: queryArg.entityId,
            from: queryArg['from'],
            to: queryArg.to,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['Denetim'],
      }),
      listSecurityEvents: build.query<ListSecurityEventsApiResponse, ListSecurityEventsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/audit/security-events`,
          params: {
            user_id: queryArg.userId,
            type: queryArg['type'],
            from: queryArg['from'],
            to: queryArg.to,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['Denetim'],
      }),
      listAcademicYears: build.query<ListAcademicYearsApiResponse, ListAcademicYearsApiArg>({
        query: () => ({ url: `/api/v1/academic-years` }),
        providesTags: ['Akademik takvim'],
      }),
      createAcademicYear: build.mutation<CreateAcademicYearApiResponse, CreateAcademicYearApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/academic-years`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Akademik takvim'],
      }),
      createTerm: build.mutation<CreateTermApiResponse, CreateTermApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/academic-years/${queryArg.id}/terms`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Akademik takvim'],
      }),
      listTerms: build.query<ListTermsApiResponse, ListTermsApiArg>({
        query: () => ({ url: `/api/v1/terms` }),
        providesTags: ['Akademik takvim'],
      }),
      getCurrentTerm: build.query<GetCurrentTermApiResponse, GetCurrentTermApiArg>({
        query: () => ({ url: `/api/v1/terms/current` }),
        providesTags: ['Akademik takvim'],
      }),
      getTerm: build.query<GetTermApiResponse, GetTermApiArg>({
        query: (queryArg) => ({ url: `/api/v1/terms/${queryArg.id}` }),
        providesTags: ['Akademik takvim'],
      }),
      updateTerm: build.mutation<UpdateTermApiResponse, UpdateTermApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/terms/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.body,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Akademik takvim'],
      }),
      makeTermCurrent: build.mutation<MakeTermCurrentApiResponse, MakeTermCurrentApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/terms/${queryArg.id}/current`,
          method: 'POST',
        }),
        invalidatesTags: ['Akademik takvim'],
      }),
      listCalendarEventTypes: build.query<ListCalendarEventTypesApiResponse, ListCalendarEventTypesApiArg>({
        query: () => ({ url: `/api/v1/calendar/event-types` }),
        providesTags: ['Akademik takvim'],
      }),
      getCalendarWindows: build.query<GetCalendarWindowsApiResponse, GetCalendarWindowsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/calendar/windows`,
          params: {
            term_id: queryArg.termId,
            faculty_id: queryArg.facultyId,
            program_id: queryArg.programId,
          },
        }),
        providesTags: ['Akademik takvim'],
      }),
      listTermEvents: build.query<ListTermEventsApiResponse, ListTermEventsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/terms/${queryArg.id}/events`,
          params: {
            type: queryArg['type'],
            faculty_id: queryArg.facultyId,
            program_id: queryArg.programId,
          },
        }),
        providesTags: ['Akademik takvim'],
      }),
      createCalendarEvent: build.mutation<CreateCalendarEventApiResponse, CreateCalendarEventApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/terms/${queryArg.id}/events`,
          method: 'POST',
          body: queryArg.calendarEventRequest,
        }),
        invalidatesTags: ['Akademik takvim'],
      }),
      getCalendarEvent: build.query<GetCalendarEventApiResponse, GetCalendarEventApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/calendar-events/${queryArg.id}`,
        }),
        providesTags: ['Akademik takvim'],
      }),
      updateCalendarEvent: build.mutation<UpdateCalendarEventApiResponse, UpdateCalendarEventApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/calendar-events/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.calendarEventRequest,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Akademik takvim'],
      }),
      deleteCalendarEvent: build.mutation<DeleteCalendarEventApiResponse, DeleteCalendarEventApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/calendar-events/${queryArg.id}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['Akademik takvim'],
      }),
      listCourses: build.query<ListCoursesApiResponse, ListCoursesApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/courses`,
          params: {
            q: queryArg.q,
            department_id: queryArg.departmentId,
            kind: queryArg.kind,
            include_inactive: queryArg.includeInactive,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['Ders katalo\u011Fu'],
      }),
      createCourse: build.mutation<CreateCourseApiResponse, CreateCourseApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/courses`,
          method: 'POST',
          body: queryArg.courseRequest,
        }),
        invalidatesTags: ['Ders katalo\u011Fu'],
      }),
      getCourse: build.query<GetCourseApiResponse, GetCourseApiArg>({
        query: (queryArg) => ({ url: `/api/v1/courses/${queryArg.id}` }),
        providesTags: ['Ders katalo\u011Fu'],
      }),
      updateCourse: build.mutation<UpdateCourseApiResponse, UpdateCourseApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/courses/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.courseRequest,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders katalo\u011Fu'],
      }),
      setCoursePrerequisites: build.mutation<SetCoursePrerequisitesApiResponse, SetCoursePrerequisitesApiArg>(
        {
          query: (queryArg) => ({
            url: `/api/v1/courses/${queryArg.id}/prerequisites`,
            method: 'PUT',
            body: queryArg.body,
          }),
          invalidatesTags: ['Ders katalo\u011Fu'],
        },
      ),
      addCourseEquivalence: build.mutation<AddCourseEquivalenceApiResponse, AddCourseEquivalenceApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/courses/${queryArg.id}/equivalences`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Ders katalo\u011Fu'],
      }),
      deleteCourseEquivalence: build.mutation<
        DeleteCourseEquivalenceApiResponse,
        DeleteCourseEquivalenceApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/courses/${queryArg.id}/equivalences/${queryArg.equivalenceId}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['Ders katalo\u011Fu'],
      }),
      listElectiveGroups: build.query<ListElectiveGroupsApiResponse, ListElectiveGroupsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/elective-groups`,
          params: {
            q: queryArg.q,
            kind: queryArg.kind,
            department_id: queryArg.departmentId,
          },
        }),
        providesTags: ['Ders katalo\u011Fu'],
      }),
      createElectiveGroup: build.mutation<CreateElectiveGroupApiResponse, CreateElectiveGroupApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/elective-groups`,
          method: 'POST',
          body: queryArg.electiveGroupRequest,
        }),
        invalidatesTags: ['Ders katalo\u011Fu'],
      }),
      getElectiveGroup: build.query<GetElectiveGroupApiResponse, GetElectiveGroupApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/elective-groups/${queryArg.id}`,
        }),
        providesTags: ['Ders katalo\u011Fu'],
      }),
      updateElectiveGroup: build.mutation<UpdateElectiveGroupApiResponse, UpdateElectiveGroupApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/elective-groups/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.electiveGroupRequest,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders katalo\u011Fu'],
      }),
      addElectiveGroupCourse: build.mutation<AddElectiveGroupCourseApiResponse, AddElectiveGroupCourseApiArg>(
        {
          query: (queryArg) => ({
            url: `/api/v1/elective-groups/${queryArg.id}/courses/${queryArg.courseId}`,
            method: 'PUT',
          }),
          invalidatesTags: ['Ders katalo\u011Fu'],
        },
      ),
      removeElectiveGroupCourse: build.mutation<
        RemoveElectiveGroupCourseApiResponse,
        RemoveElectiveGroupCourseApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/elective-groups/${queryArg.id}/courses/${queryArg.courseId}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['Ders katalo\u011Fu'],
      }),
      listProgramCurricula: build.query<ListProgramCurriculaApiResponse, ListProgramCurriculaApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/programs/${queryArg.id}/curricula`,
        }),
        providesTags: ['M\u00FCfredat'],
      }),
      createCurriculum: build.mutation<CreateCurriculumApiResponse, CreateCurriculumApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/programs/${queryArg.id}/curricula`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['M\u00FCfredat'],
      }),
      getCurriculum: build.query<GetCurriculumApiResponse, GetCurriculumApiArg>({
        query: (queryArg) => ({ url: `/api/v1/curricula/${queryArg.id}` }),
        providesTags: ['M\u00FCfredat'],
      }),
      updateCurriculum: build.mutation<UpdateCurriculumApiResponse, UpdateCurriculumApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/curricula/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.curriculumRequest,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['M\u00FCfredat'],
      }),
      deleteCurriculum: build.mutation<DeleteCurriculumApiResponse, DeleteCurriculumApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/curricula/${queryArg.id}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['M\u00FCfredat'],
      }),
      activateCurriculum: build.mutation<ActivateCurriculumApiResponse, ActivateCurriculumApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/curricula/${queryArg.id}/activate`,
          method: 'POST',
        }),
        invalidatesTags: ['M\u00FCfredat'],
      }),
      archiveCurriculum: build.mutation<ArchiveCurriculumApiResponse, ArchiveCurriculumApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/curricula/${queryArg.id}/archive`,
          method: 'POST',
        }),
        invalidatesTags: ['M\u00FCfredat'],
      }),
      addCurriculumItem: build.mutation<AddCurriculumItemApiResponse, AddCurriculumItemApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/curricula/${queryArg.id}/items`,
          method: 'POST',
          body: queryArg.curriculumItemRequest,
        }),
        invalidatesTags: ['M\u00FCfredat'],
      }),
      updateCurriculumItem: build.mutation<UpdateCurriculumItemApiResponse, UpdateCurriculumItemApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/curricula/${queryArg.id}/items/${queryArg.itemId}`,
          method: 'PUT',
          body: queryArg.curriculumItemRequest,
        }),
        invalidatesTags: ['M\u00FCfredat'],
      }),
      deleteCurriculumItem: build.mutation<DeleteCurriculumItemApiResponse, DeleteCurriculumItemApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/curricula/${queryArg.id}/items/${queryArg.itemId}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['M\u00FCfredat'],
      }),
      myCurricula: build.query<MyCurriculaApiResponse, MyCurriculaApiArg>({
        query: () => ({ url: `/api/v1/me/curricula` }),
        providesTags: ['M\u00FCfredat'],
      }),
      listGradeScales: build.query<ListGradeScalesApiResponse, ListGradeScalesApiArg>({
        query: () => ({ url: `/api/v1/grade-scales` }),
        providesTags: ['Not \u00F6l\u00E7e\u011Fi ve y\u00F6netmelik'],
      }),
      createGradeScale: build.mutation<CreateGradeScaleApiResponse, CreateGradeScaleApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-scales`,
          method: 'POST',
          body: queryArg.gradeScaleRequest,
        }),
        invalidatesTags: ['Not \u00F6l\u00E7e\u011Fi ve y\u00F6netmelik'],
      }),
      getGradeScale: build.query<GetGradeScaleApiResponse, GetGradeScaleApiArg>({
        query: (queryArg) => ({ url: `/api/v1/grade-scales/${queryArg.id}` }),
        providesTags: ['Not \u00F6l\u00E7e\u011Fi ve y\u00F6netmelik'],
      }),
      updateGradeScale: build.mutation<UpdateGradeScaleApiResponse, UpdateGradeScaleApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-scales/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.gradeScaleRequest,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Not \u00F6l\u00E7e\u011Fi ve y\u00F6netmelik'],
      }),
      listRegulationParameters: build.query<
        ListRegulationParametersApiResponse,
        ListRegulationParametersApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/regulation-parameters`,
          params: {
            at: queryArg.at,
          },
        }),
        providesTags: ['Not \u00F6l\u00E7e\u011Fi ve y\u00F6netmelik'],
      }),
      getRegulationParameterHistory: build.query<
        GetRegulationParameterHistoryApiResponse,
        GetRegulationParameterHistoryApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/regulation-parameters/${queryArg.key}`,
        }),
        providesTags: ['Not \u00F6l\u00E7e\u011Fi ve y\u00F6netmelik'],
      }),
      setRegulationParameter: build.mutation<SetRegulationParameterApiResponse, SetRegulationParameterApiArg>(
        {
          query: (queryArg) => ({
            url: `/api/v1/regulation-parameters/${queryArg.key}`,
            method: 'POST',
            body: queryArg.body,
          }),
          invalidatesTags: ['Not \u00F6l\u00E7e\u011Fi ve y\u00F6netmelik'],
        },
      ),
      listOfferings: build.query<ListOfferingsApiResponse, ListOfferingsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/terms/${queryArg.id}/offerings`,
          params: {
            department_id: queryArg.departmentId,
            q: queryArg.q,
            status: queryArg.status,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['Ders a\u00E7ma'],
      }),
      createOffering: build.mutation<CreateOfferingApiResponse, CreateOfferingApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/terms/${queryArg.id}/offerings`,
          method: 'POST',
          body: queryArg.offeringCreateRequest,
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      getOffering: build.query<GetOfferingApiResponse, GetOfferingApiArg>({
        query: (queryArg) => ({ url: `/api/v1/offerings/${queryArg.id}` }),
        providesTags: ['Ders a\u00E7ma'],
      }),
      updateOffering: build.mutation<UpdateOfferingApiResponse, UpdateOfferingApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/offerings/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.offeringUpdateRequest,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      deleteOffering: build.mutation<DeleteOfferingApiResponse, DeleteOfferingApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/offerings/${queryArg.id}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      createSection: build.mutation<CreateSectionApiResponse, CreateSectionApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/offerings/${queryArg.id}/sections`,
          method: 'POST',
          body: queryArg.sectionRequest,
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      getSection: build.query<GetSectionApiResponse, GetSectionApiArg>({
        query: (queryArg) => ({ url: `/api/v1/sections/${queryArg.id}` }),
        providesTags: ['Ders a\u00E7ma'],
      }),
      updateSection: build.mutation<UpdateSectionApiResponse, UpdateSectionApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.sectionRequest,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      deleteSection: build.mutation<DeleteSectionApiResponse, DeleteSectionApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      setSectionInstructors: build.mutation<SetSectionInstructorsApiResponse, SetSectionInstructorsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/instructors`,
          method: 'PUT',
          body: queryArg.body,
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      setSectionQuotas: build.mutation<SetSectionQuotasApiResponse, SetSectionQuotasApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/quotas`,
          method: 'PUT',
          body: queryArg.body,
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      addScheduleSlot: build.mutation<AddScheduleSlotApiResponse, AddScheduleSlotApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/slots`,
          method: 'POST',
          body: queryArg.scheduleSlotRequest,
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      updateScheduleSlot: build.mutation<UpdateScheduleSlotApiResponse, UpdateScheduleSlotApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/schedule-slots/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.scheduleSlotRequest,
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      deleteScheduleSlot: build.mutation<DeleteScheduleSlotApiResponse, DeleteScheduleSlotApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/schedule-slots/${queryArg.id}`,
          method: 'DELETE',
        }),
        invalidatesTags: ['Ders a\u00E7ma'],
      }),
      termSchedule: build.query<TermScheduleApiResponse, TermScheduleApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/terms/${queryArg.id}/schedule`,
          params: {
            department_id: queryArg.departmentId,
            classroom_id: queryArg.classroomId,
            staff_id: queryArg.staffId,
          },
        }),
        providesTags: ['Ders a\u00E7ma'],
      }),
      myTeaching: build.query<MyTeachingApiResponse, MyTeachingApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/me/teaching`,
          params: {
            term_id: queryArg.termId,
          },
        }),
        providesTags: ['Ders a\u00E7ma'],
      }),
      searchInstructors: build.query<SearchInstructorsApiResponse, SearchInstructorsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/instructors`,
          params: {
            q: queryArg.q,
            department_id: queryArg.departmentId,
            limit: queryArg.limit,
          },
        }),
        providesTags: ['Ders a\u00E7ma'],
      }),
      listAssessmentTypes: build.query<ListAssessmentTypesApiResponse, ListAssessmentTypesApiArg>({
        query: () => ({ url: `/api/v1/assessment-types` }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      getAssessmentPlan: build.query<GetAssessmentPlanApiResponse, GetAssessmentPlanApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/assessment-plan`,
        }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      setAssessmentPlan: build.mutation<SetAssessmentPlanApiResponse, SetAssessmentPlanApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/assessment-plan`,
          method: 'PUT',
          body: queryArg.body,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      lockAssessmentPlan: build.mutation<LockAssessmentPlanApiResponse, LockAssessmentPlanApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/assessment-plan/lock`,
          method: 'POST',
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      unlockAssessmentPlan: build.mutation<UnlockAssessmentPlanApiResponse, UnlockAssessmentPlanApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/assessment-plan/unlock`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      getGradebook: build.query<GetGradebookApiResponse, GetGradebookApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/gradebook`,
        }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      exportGradebook: build.query<ExportGradebookApiResponse, ExportGradebookApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/gradebook.csv`,
          params: {
            lang: queryArg.lang,
          },
        }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      setComponentScores: build.mutation<SetComponentScoresApiResponse, SetComponentScoresApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/assessment-components/${queryArg.id}/scores`,
          method: 'PUT',
          body: queryArg.body,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      previewScoreImport: build.mutation<PreviewScoreImportApiResponse, PreviewScoreImportApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/assessment-components/${queryArg.id}/scores/import/preview`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      importScores: build.mutation<ImportScoresApiResponse, ImportScoresApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/assessment-components/${queryArg.id}/scores/import`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      publishComponentScores: build.mutation<PublishComponentScoresApiResponse, PublishComponentScoresApiArg>(
        {
          query: (queryArg) => ({
            url: `/api/v1/assessment-components/${queryArg.id}/publish`,
            method: 'POST',
          }),
          invalidatesTags: ['De\u011Ferlendirme'],
        },
      ),
      setAttendance: build.mutation<SetAttendanceApiResponse, SetAttendanceApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/attendance`,
          method: 'PUT',
          body: queryArg.body,
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      getLetterGrades: build.query<GetLetterGradesApiResponse, GetLetterGradesApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/letter-grades`,
        }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      setLetterOverrides: build.mutation<SetLetterOverridesApiResponse, SetLetterOverridesApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/letter-overrides`,
          method: 'PUT',
          body: queryArg.body,
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      publishLetterGrades: build.mutation<PublishLetterGradesApiResponse, PublishLetterGradesApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/letter-grades/publish`,
          method: 'POST',
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      publishMakeup: build.mutation<PublishMakeupApiResponse, PublishMakeupApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/makeup/publish`,
          method: 'POST',
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      getSectionGradeStats: build.query<GetSectionGradeStatsApiResponse, GetSectionGradeStatsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/sections/${queryArg.id}/grade-stats`,
        }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      previewGradeChange: build.query<PreviewGradeChangeApiResponse, PreviewGradeChangeApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-change-requests/preview`,
          method: 'POST',
          body: queryArg.gradeChangeInput,
        }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      requestGradeChange: build.mutation<RequestGradeChangeApiResponse, RequestGradeChangeApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-change-requests`,
          method: 'POST',
          body: queryArg.gradeChangeInput,
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      listGradeChanges: build.query<ListGradeChangesApiResponse, ListGradeChangesApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-change-requests`,
          params: {
            view: queryArg.view,
            status: queryArg.status,
          },
        }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      getGradeChange: build.query<GetGradeChangeApiResponse, GetGradeChangeApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-change-requests/${queryArg.id}`,
        }),
        providesTags: ['De\u011Ferlendirme'],
      }),
      approveGradeChange: build.mutation<ApproveGradeChangeApiResponse, ApproveGradeChangeApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-change-requests/${queryArg.id}/approve`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      rejectGradeChange: build.mutation<RejectGradeChangeApiResponse, RejectGradeChangeApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-change-requests/${queryArg.id}/reject`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      withdrawGradeChange: build.mutation<WithdrawGradeChangeApiResponse, WithdrawGradeChangeApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/grade-change-requests/${queryArg.id}/withdraw`,
          method: 'POST',
        }),
        invalidatesTags: ['De\u011Ferlendirme'],
      }),
      listBuildings: build.query<ListBuildingsApiResponse, ListBuildingsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/buildings`,
          params: {
            campus_id: queryArg.campusId,
            include_inactive: queryArg.includeInactive,
          },
        }),
        providesTags: ['Organizasyon'],
      }),
      createBuilding: build.mutation<CreateBuildingApiResponse, CreateBuildingApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/buildings`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['Organizasyon'],
      }),
      getBuilding: build.query<GetBuildingApiResponse, GetBuildingApiArg>({
        query: (queryArg) => ({ url: `/api/v1/buildings/${queryArg.id}` }),
        providesTags: ['Organizasyon'],
      }),
      updateBuilding: build.mutation<UpdateBuildingApiResponse, UpdateBuildingApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/buildings/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.body,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Organizasyon'],
      }),
      listClassrooms: build.query<ListClassroomsApiResponse, ListClassroomsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/classrooms`,
          params: {
            building_id: queryArg.buildingId,
            campus_id: queryArg.campusId,
            room_type: queryArg.roomType,
            min_capacity: queryArg.minCapacity,
            q: queryArg.q,
            include_inactive: queryArg.includeInactive,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['Organizasyon'],
      }),
      createClassroom: build.mutation<CreateClassroomApiResponse, CreateClassroomApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/classrooms`,
          method: 'POST',
          body: queryArg.classroomInput,
        }),
        invalidatesTags: ['Organizasyon'],
      }),
      getClassroom: build.query<GetClassroomApiResponse, GetClassroomApiArg>({
        query: (queryArg) => ({ url: `/api/v1/classrooms/${queryArg.id}` }),
        providesTags: ['Organizasyon'],
      }),
      updateClassroom: build.mutation<UpdateClassroomApiResponse, UpdateClassroomApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/classrooms/${queryArg.id}`,
          method: 'PUT',
          body: queryArg.body,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Organizasyon'],
      }),
      listStudents: build.query<ListStudentsApiResponse, ListStudentsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/students`,
          params: {
            faculty_id: queryArg.facultyId,
            department_id: queryArg.departmentId,
            program_id: queryArg.programId,
            status: queryArg.status,
            class_level: queryArg.classLevel,
            q: queryArg.q,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      getStudent: build.query<GetStudentApiResponse, GetStudentApiArg>({
        query: (queryArg) => ({ url: `/api/v1/students/${queryArg.id}` }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      createStudentProgram: build.mutation<CreateStudentProgramApiResponse, CreateStudentProgramApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/students/${queryArg.id}/programs`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['\u00D6\u011Frenciler'],
      }),
      assignAdvisor: build.mutation<AssignAdvisorApiResponse, AssignAdvisorApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/advisor`,
          method: 'PUT',
          body: queryArg.body,
        }),
        invalidatesTags: ['\u00D6\u011Frenciler'],
      }),
      listAdvisorHistory: build.query<ListAdvisorHistoryApiResponse, ListAdvisorHistoryApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/advisors`,
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      changeStudentProgramStatus: build.mutation<
        ChangeStudentProgramStatusApiResponse,
        ChangeStudentProgramStatusApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/status`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['\u00D6\u011Frenciler'],
      }),
      listStudentProgramStatusHistory: build.query<
        ListStudentProgramStatusHistoryApiResponse,
        ListStudentProgramStatusHistoryApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/status-history`,
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      listStudentProgramHolds: build.query<ListStudentProgramHoldsApiResponse, ListStudentProgramHoldsApiArg>(
        {
          query: (queryArg) => ({
            url: `/api/v1/student-programs/${queryArg.id}/holds`,
          }),
          providesTags: ['\u00D6\u011Frenciler'],
        },
      ),
      placeHold: build.mutation<PlaceHoldApiResponse, PlaceHoldApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/holds`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['\u00D6\u011Frenciler'],
      }),
      listHolds: build.query<ListHoldsApiResponse, ListHoldsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/holds`,
          params: {
            faculty_id: queryArg.facultyId,
            department_id: queryArg.departmentId,
            program_id: queryArg.programId,
            type: queryArg['type'],
            active: queryArg.active,
            q: queryArg.q,
            cursor: queryArg.cursor,
            limit: queryArg.limit,
          },
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      releaseHold: build.mutation<ReleaseHoldApiResponse, ReleaseHoldApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/holds/${queryArg.id}/release`,
          method: 'POST',
          body: queryArg.body,
        }),
        invalidatesTags: ['\u00D6\u011Frenciler'],
      }),
      getTranscript: build.query<GetTranscriptApiResponse, GetTranscriptApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/transcript`,
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      simulateTranscript: build.query<SimulateTranscriptApiResponse, SimulateTranscriptApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/transcript/simulate`,
          method: 'POST',
          body: queryArg.body,
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      getStudentGrades: build.query<GetStudentGradesApiResponse, GetStudentGradesApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/grades`,
          params: {
            term_id: queryArg.termId,
          },
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      listCourseHistory: build.query<ListCourseHistoryApiResponse, ListCourseHistoryApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/history`,
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      getStudentProgress: build.query<GetStudentProgressApiResponse, GetStudentProgressApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/progress`,
        }),
        providesTags: ['Ders se\u00E7me'],
      }),
      getRegistration: build.query<GetRegistrationApiResponse, GetRegistrationApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration`,
          params: {
            term_id: queryArg.termId,
          },
        }),
        providesTags: ['Ders se\u00E7me'],
      }),
      listRegistrationOffer: build.query<ListRegistrationOfferApiResponse, ListRegistrationOfferApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/offer`,
          params: {
            term_id: queryArg.termId,
          },
        }),
        providesTags: ['Ders se\u00E7me'],
      }),
      listRegistrationEvents: build.query<ListRegistrationEventsApiResponse, ListRegistrationEventsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/events`,
          params: {
            term_id: queryArg.termId,
          },
        }),
        providesTags: ['Ders se\u00E7me'],
      }),
      addRegistrationItem: build.mutation<AddRegistrationItemApiResponse, AddRegistrationItemApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/items`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      changeRegistrationSection: build.mutation<
        ChangeRegistrationSectionApiResponse,
        ChangeRegistrationSectionApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/items/${queryArg.itemId}`,
          method: 'PUT',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      removeRegistrationItem: build.mutation<RemoveRegistrationItemApiResponse, RemoveRegistrationItemApiArg>(
        {
          query: (queryArg) => ({
            url: `/api/v1/student-programs/${queryArg.id}/registration/items/${queryArg.itemId}`,
            method: 'DELETE',
            headers: {
              'Idempotency-Key': queryArg['Idempotency-Key'],
            },
          }),
          invalidatesTags: ['Ders se\u00E7me'],
        },
      ),
      submitRegistration: build.mutation<SubmitRegistrationApiResponse, SubmitRegistrationApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/submit`,
          method: 'POST',
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
            'If-Match': queryArg['If-Match'],
          },
          params: {
            term_id: queryArg.termId,
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      withdrawRegistration: build.mutation<WithdrawRegistrationApiResponse, WithdrawRegistrationApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/withdraw`,
          method: 'POST',
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
            'If-Match': queryArg['If-Match'],
          },
          params: {
            term_id: queryArg.termId,
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      grantRegistrationExtension: build.mutation<
        GrantRegistrationExtensionApiResponse,
        GrantRegistrationExtensionApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/extension`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      listRegistrations: build.query<ListRegistrationsApiResponse, ListRegistrationsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/registrations`,
          params: {
            term_id: queryArg.termId,
            status: queryArg.status,
            faculty_id: queryArg.facultyId,
            department_id: queryArg.departmentId,
            program_id: queryArg.programId,
            q: queryArg.q,
            cursor: queryArg.cursor,
            limit: queryArg.limit,
          },
        }),
        providesTags: ['Ders se\u00E7me'],
      }),
      bulkApproveRegistrations: build.mutation<
        BulkApproveRegistrationsApiResponse,
        BulkApproveRegistrationsApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/registrations/bulk-approve`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      getRegistrationById: build.query<GetRegistrationByIdApiResponse, GetRegistrationByIdApiArg>({
        query: (queryArg) => ({ url: `/api/v1/registrations/${queryArg.id}` }),
        providesTags: ['Ders se\u00E7me'],
      }),
      approveRegistration: build.mutation<ApproveRegistrationApiResponse, ApproveRegistrationApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/registrations/${queryArg.id}/approve`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      rejectRegistration: build.mutation<RejectRegistrationApiResponse, RejectRegistrationApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/registrations/${queryArg.id}/reject`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      revokeRegistration: build.mutation<RevokeRegistrationApiResponse, RevokeRegistrationApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/registrations/${queryArg.id}/revoke`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      listAdviseeRegistrations: build.query<
        ListAdviseeRegistrationsApiResponse,
        ListAdviseeRegistrationsApiArg
      >({
        query: (queryArg) => ({
          url: `/api/v1/me/advisee-registrations`,
          params: {
            term_id: queryArg.termId,
            status: queryArg.status,
          },
        }),
        providesTags: ['Ders se\u00E7me'],
      }),
      listAddDropRequests: build.query<ListAddDropRequestsApiResponse, ListAddDropRequestsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/add-drop`,
          params: {
            term_id: queryArg.termId,
          },
        }),
        providesTags: ['Ders se\u00E7me'],
      }),
      requestAddDrop: build.mutation<RequestAddDropApiResponse, RequestAddDropApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/student-programs/${queryArg.id}/registration/add-drop`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      withdrawAddDrop: build.mutation<WithdrawAddDropApiResponse, WithdrawAddDropApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/add-drop-requests/${queryArg.id}/withdraw`,
          method: 'POST',
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      approveAddDrop: build.mutation<ApproveAddDropApiResponse, ApproveAddDropApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/add-drop-requests/${queryArg.id}/approve`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      rejectAddDrop: build.mutation<RejectAddDropApiResponse, RejectAddDropApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/add-drop-requests/${queryArg.id}/reject`,
          method: 'POST',
          body: queryArg.body,
          headers: {
            'Idempotency-Key': queryArg['Idempotency-Key'],
            'If-Match': queryArg['If-Match'],
          },
        }),
        invalidatesTags: ['Ders se\u00E7me'],
      }),
      listAddDropQueue: build.query<ListAddDropQueueApiResponse, ListAddDropQueueApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/me/add-drop-queue`,
          params: {
            term_id: queryArg.termId,
          },
        }),
        providesTags: ['Ders se\u00E7me'],
      }),
      listEligibleAdvisors: build.query<ListEligibleAdvisorsApiResponse, ListEligibleAdvisorsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/departments/${queryArg.id}/advisors`,
        }),
        providesTags: ['\u00D6\u011Frenciler'],
      }),
      listMyPrograms: build.query<ListMyProgramsApiResponse, ListMyProgramsApiArg>({
        query: () => ({ url: `/api/v1/me/programs` }),
        providesTags: ['Hesap'],
      }),
      listMyAdvisees: build.query<ListMyAdviseesApiResponse, ListMyAdviseesApiArg>({
        query: () => ({ url: `/api/v1/me/advisees` }),
        providesTags: ['Hesap'],
      }),
      listFaculties: build.query<ListFacultiesApiResponse, ListFacultiesApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/faculties`,
          params: {
            type: queryArg['type'],
            q: queryArg.q,
            include_inactive: queryArg.includeInactive,
          },
        }),
        providesTags: ['Organizasyon'],
      }),
      getFaculty: build.query<GetFacultyApiResponse, GetFacultyApiArg>({
        query: (queryArg) => ({ url: `/api/v1/faculties/${queryArg.id}` }),
        providesTags: ['Organizasyon'],
      }),
      listFacultyDepartments: build.query<ListFacultyDepartmentsApiResponse, ListFacultyDepartmentsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/faculties/${queryArg.id}/departments`,
          params: {
            include_inactive: queryArg.includeInactive,
          },
        }),
        providesTags: ['Organizasyon'],
      }),
      getDepartment: build.query<GetDepartmentApiResponse, GetDepartmentApiArg>({
        query: (queryArg) => ({ url: `/api/v1/departments/${queryArg.id}` }),
        providesTags: ['Organizasyon'],
      }),
      listPrograms: build.query<ListProgramsApiResponse, ListProgramsApiArg>({
        query: (queryArg) => ({
          url: `/api/v1/programs`,
          params: {
            faculty_id: queryArg.facultyId,
            department_id: queryArg.departmentId,
            degree_level: queryArg.degreeLevel,
            language: queryArg.language,
            education_type: queryArg.educationType,
            q: queryArg.q,
            include_inactive: queryArg.includeInactive,
            limit: queryArg.limit,
            cursor: queryArg.cursor,
          },
        }),
        providesTags: ['Organizasyon'],
      }),
      getProgram: build.query<GetProgramApiResponse, GetProgramApiArg>({
        query: (queryArg) => ({ url: `/api/v1/programs/${queryArg.id}` }),
        providesTags: ['Organizasyon'],
      }),
    }),
    overrideExisting: false,
  })
export { injectedRtkApi as agoraApi }
export type HealthzApiResponse = /** status 200 Süreç çalışıyor */ {
  status: 'ok'
  version: string
  uptime: string
}
export type HealthzApiArg = void
export type ReadyzApiResponse = /** status 200 Hazır */ Readiness
export type ReadyzApiArg = void
export type LoginApiResponse =
  | /** status 200 Giriş tamamlandı (`Tokens`) ya da ikinci adım bekleniyor (`MFAChallenge`) */ Tokens
  | MfaChallenge
export type LoginApiArg = {
  /** Refresh token'ın çerezde (web) mi gövdede (mobile) mi taşınacağını belirler. */
  'X-Agora-Client': 'web' | 'mobile'
  body: {
    username: string
    password: string
  }
}
export type VerifyMfaApiResponse = /** status 200 Giriş veya yenileme başarılı */ Tokens
export type VerifyMfaApiArg = {
  /** Refresh token'ın çerezde (web) mi gövdede (mobile) mi taşınacağını belirler. */
  'X-Agora-Client': 'web' | 'mobile'
  body: {
    mfa_token: string
    code?: string
    recovery_code?: string
  }
}
export type RefreshApiResponse = /** status 200 Giriş veya yenileme başarılı */ Tokens
export type RefreshApiArg = {
  /** Refresh token'ın çerezde (web) mi gövdede (mobile) mi taşınacağını belirler. */
  'X-Agora-Client': 'web' | 'mobile'
  /** Sadece mobil istemci gönderir. */
  refreshTokenBody: RefreshTokenBody
}
export type LogoutApiResponse = unknown
export type LogoutApiArg = {
  /** Refresh token'ın çerezde (web) mi gövdede (mobile) mi taşınacağını belirler. */
  'X-Agora-Client': 'web' | 'mobile'
  /** Sadece mobil istemci gönderir. */
  refreshTokenBody: RefreshTokenBody
}
export type ForgotPasswordApiResponse = unknown
export type ForgotPasswordApiArg = {
  body: {
    /** Öğrenci/personel numarası ya da e-posta */
    identifier: string
  }
}
export type VerifyResetTokenApiResponse = /** status 200 Bağlantı geçerli */ {
  /** ACTIVATION ise arayüz "hesabınızı etkinleştirin" metnini gösterir */
  purpose: 'RESET' | 'ACTIVATION'
  expires_at: string
}
export type VerifyResetTokenApiArg = {
  resetTokenBody: ResetTokenBody
}
export type ResetPasswordApiResponse = unknown
export type ResetPasswordApiArg = {
  body: {
    token: string
    new_password: string
  }
}
export type GetMeApiResponse = /** status 200 Profil */ Me
export type GetMeApiArg = void
export type GetMyMfaApiResponse = /** status 200 Durum */ MfaStatus
export type GetMyMfaApiArg = void
export type StartMfaSetupApiResponse = /** status 200 Sır üretildi */ MfaSetup
export type StartMfaSetupApiArg = void
export type EnableMfaApiResponse = /** status 200 MFA açıldı */ RecoveryCodes
export type EnableMfaApiArg = {
  body: {
    current_password: string
    code: string
  }
}
export type DisableMfaApiResponse = unknown
export type DisableMfaApiArg = {
  body: {
    current_password: string
    code?: string
    recovery_code?: string
  }
}
export type RegenerateRecoveryCodesApiResponse = /** status 200 Yeni kodlar */ RecoveryCodes
export type RegenerateRecoveryCodesApiArg = {
  body: {
    code: string
  }
}
export type ChangePasswordApiResponse = unknown
export type ChangePasswordApiArg = {
  body: {
    current_password: string
    new_password: string
  }
}
export type ListMySessionsApiResponse = /** status 200 Oturumlar */ {
  items: Session[]
}
export type ListMySessionsApiArg = void
export type EndOtherSessionsApiResponse = /** status 200 Kapatılan oturum sayısı */ {
  revoked: number
}
export type EndOtherSessionsApiArg = void
export type EndSessionApiResponse = unknown
export type EndSessionApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListMySecurityEventsApiResponse = /** status 200 Olaylar */ SecurityEventPage
export type ListMySecurityEventsApiArg = {
  type?: SecurityEventType
  /** Bu andan itibaren (dahil), RFC 3339 */
  from?: string
  /** Bu ana kadar (hariç), RFC 3339 */
  to?: string
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
}
export type GetMyPermissionsApiResponse = /** status 200 Yetkiler (yetki koduna ve kapsama göre sıralı) */ {
  items: Grant[]
}
export type GetMyPermissionsApiArg = void
export type ListUsersApiResponse = /** status 200 Hesaplar */ {
  items: UserSummary[]
  next_cursor?: string
}
export type ListUsersApiArg = {
  /** Kullanıcı adı, e-posta ya da ad soyadda geçen metin */
  q?: string
  status?: UserStatus
  kind?: 'STUDENT' | 'STAFF'
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
}
export type CreateUserApiResponse = /** status 201 Hesap oluşturuldu */ Profile
export type CreateUserApiArg = {
  createUserRequest: CreateUserRequest
}
export type SetUserStatusApiResponse = unknown
export type SetUserStatusApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    status: 'ACTIVE' | 'SUSPENDED' | 'DISABLED'
    reason: string
  }
}
export type ResendActivationEmailApiResponse = unknown
export type ResendActivationEmailApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type SendPasswordResetEmailApiResponse = unknown
export type SendPasswordResetEmailApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ResetUserMfaApiResponse = unknown
export type ResetUserMfaApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    reason: string
  }
}
export type ListRolesApiResponse = /** status 200 Roller (koda göre sıralı) */ {
  items: RoleDefinition[]
}
export type ListRolesApiArg = void
export type ListUserRolesApiResponse = /** status 200 Atamalar */ {
  items: Assignment[]
}
export type ListUserRolesApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type AssignRoleApiResponse = /** status 201 Atama oluşturuldu */ Assignment
export type AssignRoleApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    role: string
    scope_id?: string
    /** Boşsa hemen */
    valid_from?: string
    /** Boşsa süresiz */
    valid_until?: string
    reason: string
  }
}
export type EndRoleAssignmentApiResponse = unknown
export type EndRoleAssignmentApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  assignment: string
  body: {
    reason: string
  }
}
export type GetUserApiResponse = /** status 200 Profil */ Profile
export type GetUserApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListAuditLogApiResponse = /** status 200 Denetim kayıtları */ {
  items: AuditLogEntry[]
  next_cursor?: string
}
export type ListAuditLogApiArg = {
  actorUserId?: string
  /** kaynak.eylem, ör. role.assign */
  action?: string
  entityType?: string
  entityId?: string
  /** Bu andan itibaren (dahil), RFC 3339 */
  from?: string
  /** Bu ana kadar (hariç), RFC 3339 */
  to?: string
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
}
export type ListSecurityEventsApiResponse = /** status 200 Güvenlik olayları */ SecurityEventPage
export type ListSecurityEventsApiArg = {
  userId?: string
  type?: SecurityEventType
  /** Bu andan itibaren (dahil), RFC 3339 */
  from?: string
  /** Bu ana kadar (hariç), RFC 3339 */
  to?: string
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
}
export type ListAcademicYearsApiResponse = /** status 200 Yıllar */ {
  items: AcademicYear[]
}
export type ListAcademicYearsApiArg = void
export type CreateAcademicYearApiResponse = /** status 201 Oluşturulan yıl */ AcademicYear
export type CreateAcademicYearApiArg = {
  body: {
    start_year: number
    starts_on: string
    ends_on: string
  }
}
export type CreateTermApiResponse = /** status 201 Oluşturulan dönem */ Term
export type CreateTermApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    term_type: TermType
    starts_on: string
    ends_on: string
  }
}
export type ListTermsApiResponse = /** status 200 Dönemler */ {
  items: Term[]
}
export type ListTermsApiArg = void
export type GetCurrentTermApiResponse = /** status 200 Aktif dönem */ Term
export type GetCurrentTermApiArg = void
export type GetTermApiResponse = /** status 200 Dönem */ Term
export type GetTermApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateTermApiResponse = /** status 200 Güncel dönem */ Term
export type UpdateTermApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    starts_on: string
    ends_on: string
    status: TermStatus
  }
}
export type MakeTermCurrentApiResponse = /** status 200 Aktif dönem */ Term
export type MakeTermCurrentApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListCalendarEventTypesApiResponse = /** status 200 Türler */ {
  items: CalendarEventType[]
}
export type ListCalendarEventTypesApiArg = void
export type GetCalendarWindowsApiResponse = /** status 200 Pencereler */ CalendarWindows
export type GetCalendarWindowsApiArg = {
  termId?: string
  facultyId?: string
  programId?: string
}
export type ListTermEventsApiResponse = /** status 200 Olaylar */ {
  items: CalendarEvent[]
}
export type ListTermEventsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  type?: string
  facultyId?: string
  programId?: string
}
export type CreateCalendarEventApiResponse = /** status 201 Oluşturulan olay */ CalendarEvent
export type CreateCalendarEventApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  calendarEventRequest: CalendarEventRequest
}
export type GetCalendarEventApiResponse = /** status 200 Olay */ CalendarEvent
export type GetCalendarEventApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateCalendarEventApiResponse = /** status 200 Güncel olay */ CalendarEvent
export type UpdateCalendarEventApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  calendarEventRequest: CalendarEventRequest
}
export type DeleteCalendarEventApiResponse = unknown
export type DeleteCalendarEventApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListCoursesApiResponse = /** status 200 Dersler */ {
  items: Course[]
  /** Sonraki sayfa varsa dolu */
  next_cursor?: string
}
export type ListCoursesApiArg = {
  /** Kodda veya adda geçen metin (büyük/küçük harf ve Türkçe karakter duyarsız) */
  q?: string
  /** Sahibi bu bölüm olan dersler */
  departmentId?: string
  kind?: CourseKind
  /** Pasif kayıtlar da dönsün mü? */
  includeInactive?: boolean
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
}
export type CreateCourseApiResponse = /** status 201 Oluşturulan ders */ CourseDetail
export type CreateCourseApiArg = {
  courseRequest: CourseRequest
}
export type GetCourseApiResponse = /** status 200 Ders */ CourseDetail
export type GetCourseApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateCourseApiResponse = /** status 200 Güncel ders */ CourseDetail
export type UpdateCourseApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  courseRequest: CourseRequest
}
export type SetCoursePrerequisitesApiResponse = /** status 200 Güncel ders */ CourseDetail
export type SetCoursePrerequisitesApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    items: {
      course_id: string
      requirement?: PrerequisiteRequirement
      group_no?: number
    }[]
  }
}
export type AddCourseEquivalenceApiResponse = /** status 201 Güncel ders */ CourseDetail
export type AddCourseEquivalenceApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    equivalent_course_id: string
    /** Eski ders de yenisinin yerine sayılır mı? */
    is_bidirectional?: boolean
    /** Bu girişten itibaren geçerli */
    valid_from_year?: number
    note?: string
  }
}
export type DeleteCourseEquivalenceApiResponse = unknown
export type DeleteCourseEquivalenceApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  equivalenceId: string
}
export type ListElectiveGroupsApiResponse = /** status 200 Gruplar */ {
  items: ElectiveGroup[]
}
export type ListElectiveGroupsApiArg = {
  /** Kodda veya adda geçen metin (büyük/küçük harf ve Türkçe karakter duyarsız) */
  q?: string
  kind?: ElectiveGroupKind
  departmentId?: string
}
export type CreateElectiveGroupApiResponse = /** status 201 Oluşturulan grup */ ElectiveGroupDetail
export type CreateElectiveGroupApiArg = {
  electiveGroupRequest: ElectiveGroupRequest
}
export type GetElectiveGroupApiResponse = /** status 200 Grup */ ElectiveGroupDetail
export type GetElectiveGroupApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateElectiveGroupApiResponse = /** status 200 Güncel grup */ ElectiveGroupDetail
export type UpdateElectiveGroupApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  electiveGroupRequest: ElectiveGroupRequest
}
export type AddElectiveGroupCourseApiResponse = unknown
export type AddElectiveGroupCourseApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  courseId: string
}
export type RemoveElectiveGroupCourseApiResponse = unknown
export type RemoveElectiveGroupCourseApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  courseId: string
}
export type ListProgramCurriculaApiResponse = /** status 200 Sürümler */ {
  items: Curriculum[]
}
export type ListProgramCurriculaApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type CreateCurriculumApiResponse = /** status 201 Oluşturulan taslak */ CurriculumDetail
export type CreateCurriculumApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: CurriculumRequest & {
    /** Satırları kopyalanacak sürüm */
    copy_from_id?: string
  }
}
export type GetCurriculumApiResponse = /** status 200 Sürüm */ CurriculumDetail
export type GetCurriculumApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateCurriculumApiResponse = /** status 200 Güncel sürüm */ CurriculumDetail
export type UpdateCurriculumApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  curriculumRequest: CurriculumRequest
}
export type DeleteCurriculumApiResponse = unknown
export type DeleteCurriculumApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ActivateCurriculumApiResponse =
  /** status 200 Yürürlükteki sürüm (`student_count` bağlanan kayıtları içerir) */ CurriculumDetail
export type ActivateCurriculumApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ArchiveCurriculumApiResponse = /** status 200 Arşivlenen sürüm */ CurriculumDetail
export type ArchiveCurriculumApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type AddCurriculumItemApiResponse = /** status 201 Güncel sürüm */ CurriculumDetail
export type AddCurriculumItemApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  curriculumItemRequest: CurriculumItemRequest
}
export type UpdateCurriculumItemApiResponse = /** status 200 Güncel sürüm */ CurriculumDetail
export type UpdateCurriculumItemApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  itemId: string
  curriculumItemRequest: CurriculumItemRequest
}
export type DeleteCurriculumItemApiResponse = /** status 200 Güncel sürüm */ CurriculumDetail
export type DeleteCurriculumItemApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  itemId: string
}
export type MyCurriculaApiResponse = /** status 200 Ders planları */ {
  items: {
    student_program_id: string
    enrollment_kind: 'MAJOR' | 'DOUBLE_MAJOR' | 'MINOR'
    admission_year: number
    program: ProgramRef
    curriculum: null | CurriculumDetail
  }[]
}
export type MyCurriculaApiArg = void
export type ListGradeScalesApiResponse = /** status 200 Ölçekler */ {
  items: GradeScale[]
}
export type ListGradeScalesApiArg = void
export type CreateGradeScaleApiResponse = /** status 201 Oluşturulan ölçek */ GradeScale
export type CreateGradeScaleApiArg = {
  gradeScaleRequest: GradeScaleRequest
}
export type GetGradeScaleApiResponse = /** status 200 Ölçek */ GradeScale
export type GetGradeScaleApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateGradeScaleApiResponse = /** status 200 Güncel ölçek */ GradeScale
export type UpdateGradeScaleApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  gradeScaleRequest: GradeScaleRequest
}
export type ListRegulationParametersApiResponse = /** status 200 Parametreler */ {
  items: RegulationParameter[]
}
export type ListRegulationParametersApiArg = {
  /** Varsayılan bugün */
  at?: string
}
export type GetRegulationParameterHistoryApiResponse = /** status 200 Değerler */ {
  items: RegulationParameter[]
}
export type GetRegulationParameterHistoryApiArg = {
  key: string
}
export type SetRegulationParameterApiResponse = /** status 201 Parametrenin güncel geçmişi */ {
  items: RegulationParameter[]
}
export type SetRegulationParameterApiArg = {
  key: string
  body: {
    /** Önceki değerle aynı JSON türünde (sayı, nesne, dizi ...) */
    value: any
    /** Son değerin başlangıcından sonra olmalı */
    effective_from: string
    /** Karar bilgisi vb. */
    note?: string
  }
}
export type ListOfferingsApiResponse = /** status 200 Liste */ {
  items: Offering[]
  /** Sonraki sayfa varsa dolu */
  next_cursor?: string
}
export type ListOfferingsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Dersi açan bölüm */
  departmentId?: string
  /** Kodda veya adda geçen metin (büyük/küçük harf ve Türkçe karakter duyarsız) */
  q?: string
  status?: OfferingStatus
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
}
export type CreateOfferingApiResponse = /** status 201 Açılan ders */ OfferingDetail
export type CreateOfferingApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  offeringCreateRequest: OfferingCreateRequest
}
export type GetOfferingApiResponse = /** status 200 Açılan ders */ OfferingDetail
export type GetOfferingApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateOfferingApiResponse = /** status 200 Güncel ders */ OfferingDetail
export type UpdateOfferingApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  offeringUpdateRequest: OfferingUpdateRequest
}
export type DeleteOfferingApiResponse = unknown
export type DeleteOfferingApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type CreateSectionApiResponse = /** status 201 Güncel ders */ OfferingDetail
export type CreateSectionApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  sectionRequest: SectionRequest
}
export type GetSectionApiResponse = /** status 200 Şube */ Section
export type GetSectionApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateSectionApiResponse = /** status 200 Güncel şube */ Section
export type UpdateSectionApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  sectionRequest: SectionRequest
}
export type DeleteSectionApiResponse = unknown
export type DeleteSectionApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type SetSectionInstructorsApiResponse = /** status 200 Güncel şube */ Section
export type SetSectionInstructorsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    items: {
      staff_id: string
      role: InstructorRole
    }[]
  }
}
export type SetSectionQuotasApiResponse = /** status 200 Güncel şube */ Section
export type SetSectionQuotasApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    items: {
      program_id: string
      quota: number
    }[]
  }
}
export type AddScheduleSlotApiResponse = /** status 201 Güncel şube */ Section
export type AddScheduleSlotApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  scheduleSlotRequest: ScheduleSlotRequest
}
export type UpdateScheduleSlotApiResponse = /** status 200 Güncel şube */ Section
export type UpdateScheduleSlotApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  scheduleSlotRequest: ScheduleSlotRequest
}
export type DeleteScheduleSlotApiResponse = unknown
export type DeleteScheduleSlotApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type TermScheduleApiResponse = /** status 200 Liste */ {
  items: ScheduleEntry[]
}
export type TermScheduleApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Bölümün açtığı derslerin programı */
  departmentId?: string
  /** Dersliğin doluluğu */
  classroomId?: string
  /** Öğretim elemanının programı */
  staffId?: string
}
export type MyTeachingApiResponse = /** status 200 Liste */ {
  items: ScheduleEntry[]
}
export type MyTeachingApiArg = {
  /** Varsayılan aktif dönem */
  termId?: string
}
export type SearchInstructorsApiResponse = /** status 200 Liste */ {
  items: StaffSummary[]
}
export type SearchInstructorsApiArg = {
  /** Kodda veya adda geçen metin (büyük/küçük harf ve Türkçe karakter duyarsız) */
  q?: string
  /** Kadrosunun bulunduğu bölüm */
  departmentId?: string
  limit?: number
}
export type ListAssessmentTypesApiResponse = /** status 200 Türler */ {
  items: AssessmentType[]
}
export type ListAssessmentTypesApiArg = void
export type GetAssessmentPlanApiResponse = /** status 200 Plan */ AssessmentPlan
export type GetAssessmentPlanApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type SetAssessmentPlanApiResponse = /** status 200 Güncel plan */ AssessmentPlan
export type SetAssessmentPlanApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    components: {
      /** Değerlendirme türü kodu (MAKEUP hariç) */
      type: string
      /** Verilmezse aynı türde sırayla numaralanır */
      sequence_no?: number
      name_tr?: string
      name_en?: string
      weight: number
      /** Dönemin tarihleri içinde */
      scheduled_on?: string
    }[]
  }
}
export type LockAssessmentPlanApiResponse = /** status 200 Kilitli plan */ AssessmentPlan
export type LockAssessmentPlanApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UnlockAssessmentPlanApiResponse = /** status 200 Kilidi açılan plan */ AssessmentPlan
export type UnlockAssessmentPlanApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    reason: string
  }
}
export type GetGradebookApiResponse = /** status 200 Not tablosu */ Gradebook
export type GetGradebookApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ExportGradebookApiResponse = /** status 200 Not tablosu (CSV) */ string
export type ExportGradebookApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Sütun başlıkları ve sayı biçimi */
  lang?: 'tr' | 'en'
}
export type SetComponentScoresApiResponse = /** status 200 Güncel not tablosu */ Gradebook
export type SetComponentScoresApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    items: {
      enrollment_id: string
      score?: number | null
      absent?: boolean
    }[]
  }
}
export type PreviewScoreImportApiResponse = /** status 200 Satır satır önizleme */ ScoreImportPreview
export type PreviewScoreImportApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: string
}
export type ImportScoresApiResponse = /** status 200 Güncel not tablosu */ Gradebook
export type ImportScoresApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: string
}
export type PublishComponentScoresApiResponse = /** status 200 Güncel not tablosu */ Gradebook
export type PublishComponentScoresApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type SetAttendanceApiResponse = /** status 200 Güncel not tablosu */ Gradebook
export type SetAttendanceApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    items: {
      enrollment_id: string
      attendance_status: AttendanceStatus
    }[]
  }
}
export type GetLetterGradesApiResponse = /** status 200 Harf notları */ LetterGrades
export type GetLetterGradesApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type SetLetterOverridesApiResponse = /** status 200 Güncel harf notları */ LetterGrades
export type SetLetterOverridesApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    items: {
      enrollment_id: string
      letter: string | null
      /** Harf verilirken zorunlu, en az 5 karakter */
      reason?: string
    }[]
  }
}
export type PublishLetterGradesApiResponse = /** status 200 İlan edilmiş harf notları */ LetterGrades
export type PublishLetterGradesApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type PublishMakeupApiResponse = /** status 200 Güncel harf notları */ LetterGrades
export type PublishMakeupApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type GetSectionGradeStatsApiResponse = /** status 200 Sınıf istatistikleri */ SectionGradeStats
export type GetSectionGradeStatsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type PreviewGradeChangeApiResponse = /** status 200 Önizleme */ GradeChangePreview
export type PreviewGradeChangeApiArg = {
  gradeChangeInput: GradeChangeInput
}
export type RequestGradeChangeApiResponse = /** status 201 Talep */ GradeChangeRequest
export type RequestGradeChangeApiArg = {
  gradeChangeInput: GradeChangeInput
}
export type ListGradeChangesApiResponse = /** status 200 Talepler */ {
  items: GradeChangeRequest[]
}
export type ListGradeChangesApiArg = {
  view?: 'mine' | 'review'
  /** Virgülle ayrılmış durumlar; verilmezse hepsi */
  status?: string
}
export type GetGradeChangeApiResponse = /** status 200 Talep */ GradeChangeRequest
export type GetGradeChangeApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ApproveGradeChangeApiResponse = /** status 200 Uygulanan talep */ GradeChangeRequest
export type ApproveGradeChangeApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    /** İsteğe bağlı; verilirse en az 5 karakter */
    note?: string
  }
}
export type RejectGradeChangeApiResponse = /** status 200 Reddedilen talep */ GradeChangeRequest
export type RejectGradeChangeApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    note: string
  }
}
export type WithdrawGradeChangeApiResponse = /** status 200 Geri çekilen talep */ GradeChangeRequest
export type WithdrawGradeChangeApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListBuildingsApiResponse = /** status 200 Binalar */ {
  items: Building[]
}
export type ListBuildingsApiArg = {
  campusId?: string
  /** Pasif kayıtlar da dönsün mü? */
  includeInactive?: boolean
}
export type CreateBuildingApiResponse = /** status 201 Bina oluşturuldu */ Building
export type CreateBuildingApiArg = {
  body: {
    campus_id: string
    faculty_id?: string
    code: string
    name: string
  }
}
export type GetBuildingApiResponse = /** status 200 Bina */ Building
export type GetBuildingApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateBuildingApiResponse = /** status 200 Güncel bina */ Building
export type UpdateBuildingApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    faculty_id?: string
    name: string
    is_active: boolean
  }
}
export type ListClassroomsApiResponse = /** status 200 Derslikler */ {
  items: Classroom[]
  next_cursor?: string
}
export type ListClassroomsApiArg = {
  buildingId?: string
  campusId?: string
  roomType?: RoomType
  minCapacity?: number
  /** Kodda veya adda geçen metin (büyük/küçük harf ve Türkçe karakter duyarsız) */
  q?: string
  /** Pasif kayıtlar da dönsün mü? */
  includeInactive?: boolean
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
}
export type CreateClassroomApiResponse = /** status 201 Derslik oluşturuldu */ Classroom
export type CreateClassroomApiArg = {
  classroomInput: ClassroomInput
}
export type GetClassroomApiResponse = /** status 200 Derslik */ Classroom
export type GetClassroomApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type UpdateClassroomApiResponse = /** status 200 Güncel derslik */ Classroom
export type UpdateClassroomApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    name: string
    capacity: number
    /** Kapasiteden büyük olamaz */
    exam_capacity: number
    room_type: RoomType
    features?: ClassroomFeature[]
    is_active: boolean
  }
}
export type ListStudentsApiResponse = /** status 200 Program kayıtları */ {
  items: StudentProgram[]
  next_cursor?: string
}
export type ListStudentsApiArg = {
  facultyId?: string
  departmentId?: string
  programId?: string
  status?: EnrollmentStatus
  classLevel?: number
  /** Öğrenci numarası ya da ad soyad */
  q?: string
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
}
export type GetStudentApiResponse = /** status 200 Öğrenci */ Student
export type GetStudentApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type CreateStudentProgramApiResponse = /** status 201 Kayıt oluşturuldu */ StudentProgram
export type CreateStudentProgramApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    program_id: string
    kind?: 'MAJOR' | 'DOUBLE_MAJOR' | 'MINOR'
    admission_type: AdmissionType
    admission_year: number
    admitted_on: string
    status?: 'ACTIVE' | 'PREP'
    /** Hazırlıkta 0, diğerlerinde 1-7 (varsayılan 1) */
    class_level?: number
  }
}
export type AssignAdvisorApiResponse = /** status 200 Güncel program kaydı */ StudentProgram
export type AssignAdvisorApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    staff_id: string
    reason: string
  }
}
export type ListAdvisorHistoryApiResponse = /** status 200 En yeniden eskiye danışmanlıklar */ {
  items: AdvisorHistoryItem[]
}
export type ListAdvisorHistoryApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ChangeStudentProgramStatusApiResponse = /** status 200 Güncel program kaydı */ StudentProgram
export type ChangeStudentProgramStatusApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    status: EnrollmentStatus
    reason: string
    /** Ör. yönetim kurulu karar numarası */
    decision_ref?: string | null
    /** Sadece mezuniyette */
    graduated_on?: string | null
  }
}
export type ListStudentProgramStatusHistoryApiResponse =
  /** status 200 En yeniden eskiye durum değişiklikleri */ {
    items: StatusHistoryItem[]
  }
export type ListStudentProgramStatusHistoryApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListStudentProgramHoldsApiResponse = /** status 200 Engeller */ {
  items: Hold[]
}
export type ListStudentProgramHoldsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type PlaceHoldApiResponse = /** status 201 Engel konuldu */ Hold
export type PlaceHoldApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    hold_type: HoldType
    blocks?: HoldBlock[]
    reason: string
  }
}
export type ListHoldsApiResponse = /** status 200 Engeller */ {
  items: Hold[]
  next_cursor: string | null
}
export type ListHoldsApiArg = {
  facultyId?: string
  departmentId?: string
  programId?: string
  type?: HoldType
  /** true (varsayılan) sadece aktifler, false sadece kaldırılmışlar */
  active?: boolean
  /** Öğrenci numarası ya da ad */
  q?: string
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
  limit?: number
}
export type ReleaseHoldApiResponse = /** status 200 Kaldırılan engel */ Hold
export type ReleaseHoldApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    reason: string
  }
}
export type GetTranscriptApiResponse = /** status 200 Transkript */ Transcript
export type GetTranscriptApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type SimulateTranscriptApiResponse =
  /** status 200 Gerçek ve varsayımsal dönem sonuçları */ TranscriptSimulation
export type SimulateTranscriptApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    grades: {
      enrollment_id: string
      letter_grade: string
    }[]
  }
}
export type GetStudentGradesApiResponse = /** status 200 Notlar */ StudentGrades
export type GetStudentGradesApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Dönem; öğrencinin o dönemde dersi yoksa 404 */
  termId?: string
}
export type ListCourseHistoryApiResponse = /** status 200 Ders kayıtları */ {
  items: CourseAttempt[]
}
export type ListCourseHistoryApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type GetStudentProgressApiResponse = /** status 200 Ders durumu */ StudentProgress
export type GetStudentProgressApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type GetRegistrationApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type GetRegistrationApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
}
export type ListRegistrationOfferApiResponse =
  /** status 200 Alınabilir dersler (kategori sırasıyla) */ RegistrationOffer
export type ListRegistrationOfferApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
}
export type ListRegistrationEventsApiResponse = /** status 200 Olaylar */ {
  items: RegistrationEvent[]
}
export type ListRegistrationEventsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
}
export type AddRegistrationItemApiResponse = /** status 201 Şube sepete eklendi */ Registration
export type AddRegistrationItemApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    section_id: string
  }
}
export type ChangeRegistrationSectionApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type ChangeRegistrationSectionApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  itemId: string
  body: {
    /** Aynı dersin başka bir şubesi */
    section_id: string
  }
}
export type RemoveRegistrationItemApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type RemoveRegistrationItemApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  itemId: string
}
export type SubmitRegistrationApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type SubmitRegistrationApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
}
export type WithdrawRegistrationApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type WithdrawRegistrationApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
}
export type GrantRegistrationExtensionApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type GrantRegistrationExtensionApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    /** Verilmezse içinde bulunulan dönem */
    term_id?: string
    until: string
    reason: string
  }
}
export type ListRegistrationsApiResponse = /** status 200 Özetler */ RegistrationSummaryList
export type ListRegistrationsApiArg = {
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
  status?: RegistrationStatus
  facultyId?: string
  departmentId?: string
  programId?: string
  /** Öğrenci numarası ya da ad */
  q?: string
  /** Önceki yanıtın `next_cursor` değeri (opak) */
  cursor?: string
  limit?: number
}
export type BulkApproveRegistrationsApiResponse = /** status 200 Sonuçlar */ {
  approved: number
  failed: number
  results: {
    id: string
    ok: boolean
    /** `RULE_VIOLATION` · `VERSION_MISMATCH` · `INVALID_REGISTRATION_STATE` · `WINDOW_CLOSED` · `FORBIDDEN` · `NOT_FOUND` */
    code: string | null
    message: string | null
  }[]
}
export type BulkApproveRegistrationsApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  body: {
    items: {
      id: string
      /** Danışmanın incelediği sürüm */
      version: number
    }[]
    note?: string
  }
}
export type GetRegistrationByIdApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type GetRegistrationByIdApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ApproveRegistrationApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type ApproveRegistrationApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    /** Danışman notu ya da gerekçe (red, geri alma ve müdahalede en az 5 karakter) */
    note?: string
  }
}
export type RejectRegistrationApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type RejectRegistrationApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    /** Danışman notu ya da gerekçe (red, geri alma ve müdahalede en az 5 karakter) */
    note: string
  }
}
export type RevokeRegistrationApiResponse = /** status 200 Kaydın güncel hali */ Registration
export type RevokeRegistrationApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    /** Danışman notu ya da gerekçe (red, geri alma ve müdahalede en az 5 karakter) */
    note: string
  }
}
export type ListAdviseeRegistrationsApiResponse = /** status 200 Özetler */ RegistrationSummaryList
export type ListAdviseeRegistrationsApiArg = {
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
  status?: RegistrationStatus
}
export type ListAddDropRequestsApiResponse = /** status 200 Talepler */ {
  items: AddDropRequest[]
}
export type ListAddDropRequestsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
}
export type RequestAddDropApiResponse = /** status 201 Talep açıldı */ AddDropRequest
export type RequestAddDropApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  body: {
    term_id?: string
    /** Eklenecek şubeler */
    adds?: string[]
    /** Bırakılacak sepet satırları (kayıttaki `items[].id`) */
    drops?: string[]
    note?: string
  }
}
export type WithdrawAddDropApiResponse = /** status 200 Talebin güncel hali */ AddDropRequest
export type WithdrawAddDropApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
}
export type ApproveAddDropApiResponse = /** status 200 Talebin güncel hali */ AddDropRequest
export type ApproveAddDropApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    note?: string
  }
}
export type RejectAddDropApiResponse = /** status 200 Talebin güncel hali */ AddDropRequest
export type RejectAddDropApiArg = {
  /** İstemcinin her değiştirici istek için ürettiği anahtar (8-255 karakter). Aynı anahtarla
    tekrarlanan istek işlemi yeniden yapmaz, ilk yanıt `Idempotent-Replayed: true` başlığıyla
    döner. İlk istek sürerken gelen tekrar 409 (`IDEMPOTENCY_IN_PROGRESS`), başka bir gövdeyle
    kullanılan anahtar 422 (`IDEMPOTENCY_KEY_REUSED`) alır. Anahtarlar bir gün saklanır.
     */
  'Idempotency-Key'?: string
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Kaydı okurken alınan ETag (iyimser kilit). Kayıt bu arada değiştiyse 412 döner. */
  'If-Match': string
  body: {
    note: string
  }
}
export type ListAddDropQueueApiResponse = /** status 200 Talepler */ {
  items: AddDropRequest[]
}
export type ListAddDropQueueApiArg = {
  /** Dönem; verilmezse içinde bulunulan dönem */
  termId?: string
}
export type ListEligibleAdvisorsApiResponse = /** status 200 Danışmanlar (soyada göre sıralı) */ {
  items: EligibleAdvisor[]
}
export type ListEligibleAdvisorsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListMyProgramsApiResponse = /** status 200 Program kayıtları */ {
  items: StudentProgram[]
}
export type ListMyProgramsApiArg = void
export type ListMyAdviseesApiResponse = /** status 200 Aktif danışmanlıklar (öğrenci numarasına göre) */ {
  items: StudentProgram[]
}
export type ListMyAdviseesApiArg = void
export type ListFacultiesApiResponse = /** status 200 Birimler */ {
  items: Faculty[]
}
export type ListFacultiesApiArg = {
  type?: UnitType
  /** Kodda veya adda geçen metin (büyük/küçük harf ve Türkçe karakter duyarsız) */
  q?: string
  /** Pasif kayıtlar da dönsün mü? */
  includeInactive?: boolean
}
export type GetFacultyApiResponse = /** status 200 Birim */ Faculty
export type GetFacultyApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListFacultyDepartmentsApiResponse = /** status 200 Bölümler */ {
  items: Department[]
}
export type ListFacultyDepartmentsApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
  /** Pasif kayıtlar da dönsün mü? */
  includeInactive?: boolean
}
export type GetDepartmentApiResponse = /** status 200 Bölüm */ Department
export type GetDepartmentApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type ListProgramsApiResponse = /** status 200 Programlar */ {
  items: Program[]
  /** Sonraki sayfa varsa dolu */
  next_cursor?: string
}
export type ListProgramsApiArg = {
  facultyId?: string
  departmentId?: string
  degreeLevel?: DegreeLevel
  language?: Language
  educationType?: EducationType
  /** Kodda veya adda geçen metin (büyük/küçük harf ve Türkçe karakter duyarsız) */
  q?: string
  /** Pasif kayıtlar da dönsün mü? */
  includeInactive?: boolean
  limit?: number
  /** Önceki yanıtın `next_cursor` değeri (opak, base64url) */
  cursor?: string
}
export type GetProgramApiResponse = /** status 200 Program */ Program
export type GetProgramApiArg = {
  /** Geçerli bir UUID değilse 404 döner. */
  id: string
}
export type Readiness = {
  status: 'ready' | 'not_ready'
  checks: {
    [key: string]: 'ok' | 'unavailable'
  }
}
export type Tokens = {
  access_token: string
  token_type: 'Bearer'
  /** Access token'ın kalan ömrü (saniye) */
  expires_in: number
  /** Sadece mobil istemcide */
  refresh_token?: string
  /** Sadece mobil istemcide. Refresh token'ın kalan ömrü (saniye) */
  refresh_expires_in?: number
  session_id: string
  /** true ise istemci kullanıcıyı parola değiştirme ekranına yönlendirmeli */
  must_change_password: boolean
}
export type MfaChallenge = {
  mfa_required: true
  /** `POST /api/v1/auth/mfa/verify` ile gönderilir */
  mfa_token: string
  /** Kalan süre (saniye) */
  expires_in: number
}
export type Problem = {
  type: string
  title: string
  status: number
  detail?: string
  instance?: string
  code?: string
  request_id?: string
  errors?: {
    field: string
    /** Kullanıcıya gösterilebilir (Türkçe) */
    message: string
    /** Makine okunur kod (varsa), ör. TOO_SHORT */
    code?: string
  }[]
}
export type RefreshTokenBody = {
  refresh_token?: string
}
export type ResetTokenBody = {
  token: string
}
export type RoleAssignment = {
  role: string
  role_name: string
  scope_type: 'UNIVERSITY' | 'FACULTY' | 'DEPARTMENT' | 'PROGRAM' | 'NONE'
  /** UNIVERSITY ve NONE kapsamlarında null */
  scope_id: string | null
  scope_name: string | null
  valid_until: string | null
}
export type Profile = {
  id: string
  username: string
  email: string
  first_name: string
  last_name: string
  status: 'PENDING' | 'ACTIVE' | 'SUSPENDED' | 'DISABLED'
  must_change_password: boolean
  mfa_enabled: boolean
  last_login_at: string | null
  roles: RoleAssignment[]
}
export type Me = Profile & {
  session_id: string
  /** Kullanıcının bazı yetkileri bu oturumda iki adımlı doğrulama olmadığı için
    kullanılamıyor. İstemci MFA kurulumunu (ya da MFA ile yeniden girişi) önermeli.
     */
  mfa_required: boolean
}
export type MfaStatus = {
  enabled: boolean
  enabled_at: string | null
  recovery_codes_remaining: number
}
export type MfaSetup = {
  /** Base32 (dolgusuz). QR okutulamazsa elle girilir */
  secret: string
  /** QR kod olarak gösterilir (SHA1, 6 hane, 30 sn) */
  otpauth_uri: string
}
export type RecoveryCodes = {
  recovery_codes: string[]
}
export type Session = {
  id: string
  client: 'WEB' | 'MOBILE'
  browser: string | null
  os: string | null
  device: 'DESKTOP' | 'MOBILE' | 'TABLET' | 'UNKNOWN'
  ip: string | null
  created_at: string
  last_seen_at: string
  /** Boşta kalma ya da mutlak süreden hangisi önce dolarsa */
  expires_at: string
  /** İsteği yapan oturum mu */
  current: boolean
}
export type SecurityEventType =
  | 'LOGIN_SUCCEEDED'
  | 'LOGIN_FAILED'
  | 'ACCOUNT_LOCKED'
  | 'LOGOUT'
  | 'REFRESH_TOKEN_REUSED'
  | 'PASSWORD_CHANGED'
  | 'PASSWORD_RESET_REQUESTED'
  | 'PASSWORD_RESET_COMPLETED'
  | 'SESSION_REVOKED'
  | 'OTHER_SESSIONS_REVOKED'
  | 'MFA_ENABLED'
  | 'MFA_DISABLED'
  | 'MFA_CHALLENGE_STARTED'
  | 'MFA_RECOVERY_CODES_RENEWED'
export type SecurityEvent = {
  id: number
  occurred_at: string
  type: SecurityEventType
  user_id: string | null
  /** `user_id` hesabının kullanıcı adı */
  username: string | null
  username_attempted: string | null
  ip: string | null
  user_agent: string | null
  request_id: string | null
  /** Olay türüne göre ayrıntılar (ör. reason, session_id, locked_until) */
  details: {
    [key: string]: any
  }
}
export type SecurityEventPage = {
  items: SecurityEvent[]
  next_cursor?: string
}
export type Grant = {
  permission: string
  scope_type: 'UNIVERSITY' | 'FACULTY' | 'DEPARTMENT' | 'PROGRAM' | 'NONE'
  /** UNIVERSITY ve NONE kapsamlarında null */
  scope_id: string | null
}
export type UserStatus = 'PENDING' | 'ACTIVE' | 'SUSPENDED' | 'DISABLED'
export type UserSummary = {
  id: string
  username: string
  email: string
  first_name: string
  last_name: string
  status: UserStatus
  kind: ('STUDENT' | 'STAFF' | null) | ('STUDENT' | 'STAFF' | null)
  last_login_at: string | null
  created_at: string
}
export type CreateUserRequest = {
  kind: 'STUDENT' | 'STAFF'
  /** Öğrenci numarası (8-11 hane) ya da personel numarası (3-20 harf/rakam). Kullanıcı adı olur. */
  number: string
  first_name: string
  last_name: string
  email: string
  /** Personelde zorunlu */
  staff_type?: 'ACADEMIC' | 'ADMINISTRATIVE'
  /** Sadece akademik personelde */
  academic_title?: string
  /** Personelin bölümü (isteğe bağlı) */
  department_id?: string
}
export type RoleDefinition = {
  code: string
  name_tr: string
  name_en: string
  scope_type: 'UNIVERSITY' | 'FACULTY' | 'DEPARTMENT' | 'PROGRAM' | 'NONE'
  description: string | null
  permissions: string[]
}
export type Assignment = {
  id: string
  role: string
  role_name: string
  scope_type: 'UNIVERSITY' | 'FACULTY' | 'DEPARTMENT' | 'PROGRAM' | 'NONE'
  scope_id: string | null
  scope_name: string | null
  valid_from: string
  valid_until: string | null
  state: 'ACTIVE' | 'UPCOMING' | 'ENDED'
  /** Seed ve sistem atamalarında null */
  assigned_by: string | null
  reason: string | null
  created_at: string
}
export type AuditLogEntry = {
  id: number
  occurred_at: string
  /** Sistem işlemlerinde null */
  actor_user_id: string | null
  /** İşlemi yapanın kullanıcı adı */
  actor_username: string | null
  action: string
  entity_type: string
  entity_id: string | null
  /** Değişiklikten önceki hal (oluşturmada null) */
  before: any
  /** Değişiklikten sonraki hal (silmede null) */
  after: any
  ip: string | null
  user_agent: string | null
  request_id: string | null
}
export type TermType = 'FALL' | 'SPRING' | 'SUMMER'
export type TermStatus = 'PLANNED' | 'ACTIVE' | 'CLOSED'
export type Term = {
  id: string
  code: string
  academic_year: string
  start_year: number
  term_type: TermType
  starts_on: string
  ends_on: string
  status: TermStatus
  is_current: boolean
  version: number
}
export type AcademicYear = {
  id: string
  start_year: number
  label: string
  starts_on: string
  ends_on: string
  terms: Term[]
}
export type CalendarEventType = {
  code: string
  name_tr: string
  name_en: string
  category: 'REGISTRATION' | 'INSTRUCTION' | 'EXAM' | 'GRADING' | 'ADMISSION' | 'OTHER'
  is_action_window: boolean
}
export type CalendarScopeType = 'UNIVERSITY' | 'FACULTY' | 'PROGRAM'
export type CalendarEvent = {
  id: string
  term_id: string
  type: CalendarEventType
  /** Boşsa türün adı gösterilir */
  title_tr: string | null
  title_en: string | null
  starts_at: string
  /** Hariç */
  ends_at: string
  scope_type: CalendarScopeType
  /** Birim ya da program, üniversite kapsamında null */
  scope: {
    id: string
    name: string
  } | null
  is_published: boolean
  note: string | null
  version: number
}
export type CalendarWindow = {
  type: CalendarEventType
  /** Uygulanan olayların kapsamı, hiç olay yoksa null */
  scope_type: CalendarScopeType | null
  open: boolean
  /** Şu an açık olan olay */
  current: CalendarEvent | null
  /** Açık değilse sıradaki olay */
  next: CalendarEvent | null
  events: CalendarEvent[]
}
export type CalendarWindows = {
  term: Term
  /** Sorgunun cevaplandığı an */
  at: string
  items: CalendarWindow[]
}
export type CalendarEventRequest = {
  /** Sadece oluşturmada zorunlu, sonradan değişmez */
  type?: string
  title_tr?: string
  title_en?: string
  starts_at: string
  ends_at: string
  scope_type: CalendarScopeType
  /** Üniversite kapsamında boş */
  scope_id?: string
  is_published?: boolean
  note?: string
}
export type DepartmentRef = {
  id: string
  code: string
  name_tr: string
  name_en: string
}
export type CourseKind = 'REGULAR' | 'NON_CREDIT' | 'INTERNSHIP' | 'PROJECT' | 'PREP' | 'ACTIVITY'
export type GradingMode = 'LETTER' | 'PASS_FAIL'
export type Course = {
  id: string
  code: string
  name_tr: string
  name_en: string
  /** Boşsa üniversite ortak dersi */
  owner_department: null | DepartmentRef
  theory_hours: number
  practice_hours: number
  /** Ulusal kredi */
  national_credit: number
  ects: number
  language: 'TR' | 'EN'
  kind: CourseKind
  grading_mode: GradingMode
  description_tr: string | null
  description_en: string | null
  learning_outcomes: string[]
  is_active: boolean
  version: number
}
export type CourseRef = {
  id: string
  code: string
  name_tr: string
  name_en: string
  ects: number
}
export type PrerequisiteRequirement = 'PASSED' | 'ATTENDED'
export type ElectiveGroupKind = 'TECHNICAL' | 'UNIVERSITY_GENERAL' | 'PEDAGOGICAL' | 'SOCIAL' | 'FREE'
export type ElectiveGroupRef = {
  id: string
  code: string
  name_tr: string
  name_en: string
  kind: ElectiveGroupKind
}
export type CourseDetail = Course & {
  /** Aynı group_no içindekiler VEYA, farklı gruplar VE ile bağlanır. */
  prerequisites: {
    course: CourseRef
    requirement: PrerequisiteRequirement
    group_no: number
  }[]
  /** Bu dersi ön koşul olarak isteyen dersler */
  required_by: CourseRef[]
  equivalences: {
    id: string
    /** REPLACES: bu ders diğerinin (eski kodun) yerini alır. REPLACED_BY: bu dersin yerini diğeri almıştır. */
    relation: 'REPLACES' | 'REPLACED_BY'
    course: CourseRef
    is_bidirectional: boolean
    valid_from_year: number | null
    note: string | null
  }[]
  /** Dersin bulunduğu seçmeli havuzlar */
  elective_groups: ElectiveGroupRef[]
}
export type CourseRequest = {
  /** Oluşturmada zorunlu, büyük harfe çevrilir. Güncellemede boş bırakılır ya da aynı gönderilir. */
  code?: string
  /** Boşsa üniversite ortak dersi */
  owner_department_id?: string
  name_tr: string
  name_en: string
  theory_hours?: number
  practice_hours?: number
  national_credit?: number
  ects: number
  language: 'TR' | 'EN'
  kind?: CourseKind
  grading_mode?: GradingMode
  description_tr?: string
  description_en?: string
  learning_outcomes?: string[]
  /** Oluşturmada varsayılan true, güncellemede gönderilmezse değişmez */
  is_active?: boolean
}
export type ElectiveGroup = {
  id: string
  code: string
  name_tr: string
  name_en: string
  /** Boşsa üniversite geneli havuz */
  owner_department: null | DepartmentRef
  kind: ElectiveGroupKind
  is_active: boolean
  course_count: number
  version: number
}
export type ElectiveGroupDetail = ElectiveGroup & {
  courses: Course[]
}
export type ElectiveGroupRequest = {
  /** Oluşturmada zorunlu, büyük harfe çevrilir. Güncellemede boş bırakılır ya da aynı gönderilir. */
  code?: string
  name_tr: string
  name_en: string
  /** Boşsa üniversite geneli havuz */
  owner_department_id?: string
  kind: ElectiveGroupKind
  /** Oluşturmada varsayılan true, güncellemede gönderilmezse değişmez */
  is_active?: boolean
}
export type ProgramRef = {
  id: string
  code: string
  name_tr: string
  name_en: string
}
export type CurriculumStatus = 'DRAFT' | 'ACTIVE' | 'ARCHIVED'
export type Curriculum = {
  id: string
  program: ProgramRef
  name_tr: string
  name_en: string
  /** Bu yıl ve sonrası girişliler */
  effective_from_year: number
  /** Boşsa yeni girişlere açık */
  effective_to_year: number | null
  total_ects_required: number
  status: CurriculumStatus
  /** Senato kararı vb. */
  decision_ref: string | null
  copied_from_id: string | null
  activated_at: string | null
  archived_at: string | null
  /** Bu sürümü izleyen program kayıtları */
  student_count: number
  version: number
}
export type CurriculumItemType = 'COURSE' | 'ELECTIVE_SLOT'
export type CurriculumItem = {
  id: string
  semester_no: number
  item_type: CurriculumItemType
  /** Dersin ya da havuzun kodu */
  code: string
  name_tr: string
  name_en: string
  course: null | CourseRef
  course_kind: null | CourseKind
  has_prerequisites: boolean
  elective_group: null | ElectiveGroupRef
  theory_hours: number
  practice_hours: number
  national_credit: number
  /** Ders satırında dersin, yuvada yuvanın AKTS'si */
  ects: number
  /** Yuvada havuzdan seçilecek ders sayısı (saat, kredi ve AKTS bu derslerin toplamı); ders satırında 1 */
  course_count: number
  is_compulsory: boolean
  /** Yarıyıl içindeki sıra */
  position: number
}
export type CurriculumDetail = Curriculum & {
  items: CurriculumItem[]
  summary: {
    total_ects: number
    total_credit: number
    compulsory_ects: number
    elective_ects: number
    semesters: {
      semester_no: number
      ects: number
      national_credit: number
      item_count: number
    }[]
    /** Seçmeli grup türüne göre AKTS (teknik seçmeli, alan dışı ...) */
    elective_kinds: {
      kind: ElectiveGroupKind
      ects: number
    }[]
  }
}
export type CurriculumRequest = {
  name_tr: string
  name_en: string
  effective_from_year: number
  effective_to_year?: number | null
  total_ects_required: number
  decision_ref?: string
}
export type CurriculumItemRequest = {
  semester_no: number
  item_type: CurriculumItemType
  /** Ders satırında zorunlu */
  course_id?: string
  /** Yuvada zorunlu */
  elective_group_id?: string
  /** Sadece yuvada */
  theory_hours?: number
  /** Sadece yuvada */
  practice_hours?: number
  /** Sadece yuvada */
  national_credit?: number
  /** Sadece yuvada */
  ects?: number
  /** Sadece yuvada: havuzdan seçilecek ders sayısı */
  course_count?: number
  /** Ders satırında varsayılan true, yuva zorunlu olamaz */
  is_compulsory?: boolean
  position?: number
}
export type GradeItem = {
  letter: string
  /** 4'lük sistemde katsayı; ortalamaya girmeyen harflerde null */
  coefficient: number | null
  /** Puan aralığının alt ucu (dahil); puanla verilmeyen harflerde null */
  min_score: number | null
  /** Puan aralığının üst ucu (dahil) */
  max_score: number | null
  is_passing: boolean
  /** Ağırlıklı not ortalamasına girer mi? */
  counts_in_gpa: boolean
  /** AKTS kazandırır mı? */
  earns_ects: boolean
  /** Devamsızlıktan kalma notu (2023 ölçeğinde F4, öncekinde F1); puanla verilmez, ölçekte en fazla bir tane */
  is_attendance_fail: boolean
  /** Finale girme hakkı olduğu hâlde girmedi notu (2023 ölçeğinde F3); puanla verilmez, ölçekte en fazla bir tane. Yoksa finale girmeyen puanla verilen en düşük harfi alır */
  is_final_absent: boolean
}
export type GradeScale = {
  id: string
  code: string
  name_tr: string
  name_en: string
  effective_from_year: number
  is_default: boolean
  items: GradeItem[]
  version: number
}
export type GradeScaleRequest = {
  /** Sadece oluşturmada; büyük harfe çevrilir */
  code?: string
  name_tr: string
  name_en: string
  effective_from_year: number
  is_default?: boolean
  items: GradeItem[]
}
export type RegulationParameter = {
  key: string
  /** Parametreye göre sayı, nesne ya da dizi */
  value: any
  effective_from: string
  /** Hariç; null ise hâlâ geçerli */
  effective_to: string | null
  description_tr: string
  note: string | null
}
export type NamedRef = {
  id: string
  code: string
  name_tr: string
  name_en: string
}
export type OfferingStatus = 'PLANNED' | 'OPEN' | 'CLOSED' | 'CANCELLED'
export type Offering = {
  id: string
  term: {
    id: string
    code: string
  }
  course: NamedRef & {
    theory_hours: number
    practice_hours: number
    national_credit: number
    ects: number
    language: 'TR' | 'EN'
  }
  department: DepartmentRef
  status: OfferingStatus
  /** Eski sistemdeki ders açma numarası */
  external_ref: string | null
  note: string | null
  /** Etkin şube sayısı */
  section_count: number
  total_capacity: number
  total_enrolled: number
  version: number
}
export type InstructorRole = 'PRIMARY' | 'CO_INSTRUCTOR' | 'ASSISTANT'
export type Instructor = {
  staff_id: string
  staff_no: string
  title: string | null
  first_name: string
  last_name: string
  role: InstructorRole
}
export type ScheduleSlot = {
  id: string
  /** ISO: 1 pazartesi, 7 pazar */
  day_of_week: number
  start_time: string
  /** Hariç */
  end_time: string
  session_type: 'THEORY' | 'PRACTICE' | 'LAB'
  /** Çevrim içi oturumda null */
  classroom: null | {
    id: string
    code: string
    name: string
    building_code: string
    capacity: number
  }
}
export type Section = {
  id: string
  offering_id: string
  section_code: string
  capacity: number
  enrolled_count: number
  /** RESERVED: sadece kontenjan ayrılmış programlardan */
  quota_mode: 'OPEN' | 'RESERVED'
  instruction_mode: 'IN_PERSON' | 'ONLINE' | 'HYBRID'
  language: 'TR' | 'EN'
  status: 'ACTIVE' | 'CANCELLED'
  instructors: Instructor[]
  quotas: {
    program: ProgramRef
    quota: number
    enrolled: number
  }[]
  slots: ScheduleSlot[]
  version: number
}
export type OfferingDetail = Offering & {
  sections: Section[]
}
export type OfferingCreateRequest = {
  course_id: string
  /** Dersi açan bölüm */
  department_id: string
  external_ref?: string
  note?: string
}
export type OfferingUpdateRequest = {
  status: OfferingStatus
  external_ref?: string
  note?: string
}
export type SectionRequest = {
  /** Sadece oluşturmada (zorunlu) */
  section_code?: string
  /** Oluşturmada zorunlu */
  capacity?: number
  quota_mode?: 'OPEN' | 'RESERVED'
  instruction_mode?: 'IN_PERSON' | 'ONLINE' | 'HYBRID'
  language?: 'TR' | 'EN'
  /** Sadece güncellemede */
  status?: 'ACTIVE' | 'CANCELLED'
}
export type ScheduleSlotRequest = {
  day_of_week: number
  /** 07:00 ve sonrası */
  start_time: string
  /** Hariç; 23:00 ve öncesi */
  end_time: string
  /** Çevrim içi oturumda boş */
  classroom_id?: string
  session_type?: 'THEORY' | 'PRACTICE' | 'LAB'
}
export type ScheduleEntry = ScheduleSlot & {
  term_id: string
  offering_id: string
  section_id: string
  section_code: string
  course: NamedRef
  instructors: Instructor[]
}
export type StaffSummary = {
  staff_id: string
  staff_no: string
  title: string | null
  first_name: string
  last_name: string
  department: null | DepartmentRef
}
export type AssessmentType = {
  code: string
  name_tr: string
  name_en: string
  /** IN_TERM ve FINAL ağırlıkları toplamı 100; MAKEUP finalin yerine geçer */
  category: 'IN_TERM' | 'FINAL' | 'MAKEUP'
}
export type AssessmentPlan = {
  section_id: string
  components: {
    id: string
    type: AssessmentType
    sequence_no: number
    name_tr: string | null
    name_en: string | null
    /** Gösterilecek ad: kendi adı ya da tür adı ve (gerekirse) sıra numarası */
    label_tr: string
    label_en: string
    weight: number
    scheduled_on: string | null
  }[]
  in_term_weight: number
  final_weight: number
  /** Tek final, toplam 100 ve (varsa) finalin ağırlığı yönetmeliğin öngördüğü kadar */
  is_complete: boolean
  /** Yönetmeliğin finale (ve yerine geçen bütünlemeye) verdiği ağırlık, dönemin başında geçerli `final_exam_weight` parametresi (2023 değişikliğinden sonra 60: dönem içi değerlendirmeler toplam 40). Boşsa finalin ağırlığı serbesttir. */
  required_final_weight: number | null
  locked_at: string | null
  /** Kilitleyenin adı */
  locked_by: string | null
  /** İsteği yapan kullanıcı planı şu an düzenleyebilir mi? */
  editable: boolean
  can_unlock: boolean
  version: number
}
export type GradeEntryWindow = {
  open: boolean
  starts_at: string | null
  /** Bitiş anı hariç */
  ends_at: string | null
  next_starts_at: string | null
}
export type StudentRef = {
  id: string
  student_no: string
  first_name: string
  last_name: string
}
export type AttendanceStatus = 'CONTINUING' | 'ATTENDANCE_FAIL'
export type ComponentScore = {
  score: number | null
  /** Girmedi */
  absent: boolean
}
export type Gradebook = {
  section: {
    id: string
    section_code: string
    course: {
      id: string
      code: string
      name_tr: string
      name_en: string
      grading_mode: 'LETTER' | 'PASS_FAIL'
    }
    term: {
      id: string
      code: string
    }
    /** Notlar sadece kilitli plana girilir */
    plan_locked: boolean
    /** Harf notları ilan edildiyse: notlar artık değişmez */
    grades_published_at: string | null
    /** Bütünleme sonuçları ilan edildiyse */
    makeup_published_at: string | null
    /** İsteği yapanın şubedeki rolü */
    instructor_role:
      ('PRIMARY' | 'CO_INSTRUCTOR' | 'ASSISTANT' | null) | ('PRIMARY' | 'CO_INSTRUCTOR' | 'ASSISTANT' | null)
  }
  components: {
    id: string
    type: AssessmentType
    sequence_no: number
    label_tr: string
    label_en: string
    weight: number
    /** Not yazarken If-Match */
    version: number
    published_at: string | null
    entry_window: GradeEntryWindow
    /** İsteği yapan bu bileşene şu an not girebilir mi? */
    editable: boolean
    scored_count: number
    absent_count: number
    /** Girilen puanların ortalaması (girmedi hariç) */
    average: number | null
  }[]
  /** Soyadı ve adına göre sıralı */
  students: {
    enrollment_id: string
    student: StudentRef
    program: {
      code: string
      name_tr: string
      name_en: string
    }
    /** 1'den büyükse dersi tekrar alıyor */
    attempt_no: number
    /** İlan edilmiş harf notu */
    letter: string | null
    /** Harf notları ilan edildi ve öğrencinin bütünleme hakkı var */
    makeup_eligible: boolean
    attendance_status: AttendanceStatus
    /** Bileşen kimliğine göre girilmiş notlar; not girilmemiş bileşen yer almaz */
    scores: {
      [key: string]: ComponentScore
    }
  }[]
  /** İsteği yapan bu şubeye not girebilir: sorumlu ya da ortak öğretim elemanı, MFA'lı oturum */
  can_enter: boolean
  /** Not girebilecek ama oturum iki adımlı doğrulamayla açılmamış */
  mfa_required: boolean
}
export type ScoreImportRow = {
  /** Dosyadaki satır numarası (başlık 1. satır) */
  line: number
  /** Dosyada yazdığı gibi */
  student_no: string
  student: null | StudentRef
  enrollment_id: string | null
  /** EMPTY: puan hücresi boş, not değişmez */
  status: 'CHANGED' | 'UNCHANGED' | 'EMPTY' | 'ERROR'
  /** Şu anki not; girilmemişse boş */
  current: null | ComponentScore
  /** Dosyadaki not; boş hücrede ve hatalı satırda boş */
  next: null | ComponentScore
  /** NOT_IN_SECTION: şubenin kesin kayıtlı öğrencisi değil; DUPLICATE_STUDENT: öğrenci dosyada birden fazla kez geçiyor; NOT_ELIGIBLE: bütünleme hakkı yok */
  error_code:
    | (
        | 'MISSING_STUDENT_NO'
        | 'NOT_IN_SECTION'
        | 'DUPLICATE_STUDENT'
        | 'INVALID_SCORE'
        | 'NOT_ELIGIBLE'
        | null
      )
    | (
        | 'MISSING_STUDENT_NO'
        | 'NOT_IN_SECTION'
        | 'DUPLICATE_STUDENT'
        | 'INVALID_SCORE'
        | 'NOT_ELIGIBLE'
        | null
      )
  /** Hatanın Türkçe açıklaması */
  error: string | null
}
export type ScoreImportPreview = {
  component_id: string
  /** Uygularken If-Match: önizlemedeki not sürümü */
  version: number
  /** Puanların okunduğu sütunun başlığı; başlıksız dosyada boş */
  column: string | null
  /** Dosyadaki sırayla; tamamen boş satırlar yer almaz */
  rows: ScoreImportRow[]
  /** Şubenin dosyada olmayan öğrencileri; notları değişmez */
  missing: StudentRef[]
  summary: {
    changed: number
    unchanged: number
    empty: number
    errors: number
  }
}
export type LetterReason =
  'RANGE' | 'ATTENDANCE_FAIL' | 'FINAL_ABSENT' | 'FINAL_BELOW_MIN' | 'PASS_FAIL' | 'MANUAL'
export type LetterDistribution = {
  letter: string
  count: number
}[]
export type LetterGrades = {
  section_id: string
  grading_mode: 'LETTER' | 'PASS_FAIL'
  published_at: string | null
  /** İlan edenin adı */
  published_by: string | null
  /** İsteği yapan şimdi ilan edebilir: sorumlu öğretim elemanı, pencere açık, eksik not yok */
  can_publish: boolean
  /** İsteği yapan şimdi elle harf notu verebilir */
  can_override: boolean
  publish_window: GradeEntryWindow
  override_window: GradeEntryWindow
  makeup_published_at: string | null
  /** İsteği yapan bütünleme sonuçlarını şimdi ilan edebilir */
  can_publish_makeup: boolean
  makeup_window: GradeEntryWindow
  /** Final ya da bütünlemede en az alınması gereken puan (yönetmelik parametresi) */
  final_min_score: number
  /** Elle verilebilecek harfler, başarıdan başarısızlığa */
  scale: {
    letter: string
    min_score: number | null
    max_score: number | null
    passing: boolean
  }[]
  /** Soyadı ve adına göre sıralı */
  students: {
    enrollment_id: string
    student: StudentRef
    attendance_status: AttendanceStatus
    /** Bütün bileşenlerin notu girilmiş (ya da devamsız) */
    complete: boolean
    missing_components: string[]
    /** Ders başarı puanı (iki ondalık) */
    weighted_score: number | null
    /** Harfin bakıldığı tam sayı puan */
    rounded_score: number | null
    computed_letter: string | null
    reason: LetterReason | null
    /** Bütünleme notu finalin yerine geçti */
    used_makeup: boolean
    override: null | {
      letter: string
      reason: string
      set_by: string
      set_at: string
    }
    /** Geçerli harf: ilan edilmişse ilan edilen, yoksa elle verilen ya da hesaplanan */
    letter: string | null
    passing: boolean | null
    /** Bütünleme hakkı var (harf notları ilan edildikten sonra) */
    makeup_eligible: boolean
    /** Bütünleme ilanından önce: girilen bütünleme notuyla olacak harf */
    makeup_letter: string | null
    /** Bütünlemeyle değiştiyse bütünlemeden önceki harf */
    previous_letter: string | null
  }[]
  distribution: LetterDistribution
  summary: {
    students: number
    /** Harfi belli olan öğrenci sayısı */
    complete: number
    /** Ders başarı puanlarının ortalaması */
    mean: number | null
    median: number | null
    /** Ders başarı puanlarının standart sapması (sınıfın tamamı üzerinden) */
    stddev: number | null
    /** Harfi belli olanlardan geçenlerin oranı (0-1) */
    pass_rate: number | null
  }
}
export type SectionGradeStats = {
  section_id: string
  grading_mode: GradingMode
  /** Harf notu ilan edilen öğrenci sayısı */
  student_count: number
  /** Ders başarı puanı olan öğrenci sayısı */
  scored_count: number
  passed_count: number
  /** Geçenlerin oranı (0-1) */
  pass_rate: number | null
  mean: number | null
  median: number | null
  stddev: number | null
  distribution: LetterDistribution
  /** Son ilan ya da not düzeltmesiyle hesaplandığı an */
  computed_at: string
}
export type GradeChangeComponent = {
  id: string
  label_tr: string
  label_en: string
  weight: number
}
export type GradeValue = {
  letter: string
  weighted_score: number | null
  /** Harfin kuralı (LetterReason); elle değişiklikte MANUAL */
  reason: string
  /** Bileşen notu düzeltmesinde bileşenin puanı */
  score: number | null
  /** Bileşen notu düzeltmesinde girmedi */
  absent: boolean
}
export type GradeChangePreview = {
  kind: 'SCORE' | 'LETTER'
  component: GradeChangeComponent | null
  old: GradeValue
  new: GradeValue
}
export type GradeChangeInput = {
  enrollment_id: string
  component_id?: string | null
  score?: number | null
  absent?: boolean
  letter?: string | null
  /** Talepte zorunlu, 10-500 karakter; önizlemede verilmeyebilir */
  reason?: string
}
export type GradeChangeRequest = {
  id: string
  /** APPLIED: onaylandı ve uygulandı */
  status: 'PENDING' | 'APPLIED' | 'REJECTED' | 'WITHDRAWN'
  kind: 'SCORE' | 'LETTER'
  enrollment_id: string
  student: StudentRef
  course: {
    code: string
    name_tr: string
    name_en: string
  }
  section: {
    id: string
    code: string
  }
  term_code: string
  component: GradeChangeComponent | null
  old: GradeValue
  new: GradeValue
  reason: string
  /** Talep edenin adı */
  requested_by: string
  requested_at: string
  /** Onaylayan ya da reddedenin adı */
  reviewed_by: string | null
  review_note: string | null
  decided_at: string | null
  /** İsteği yapan bu talebi onaylayıp reddedebilir */
  can_approve: boolean
  /** İsteği yapan bu talebi geri çekebilir */
  can_withdraw: boolean
}
export type Ref = {
  id: string
  code: string
  name_tr: string
}
export type Building = {
  id: string
  code: string
  name: string
  campus: {
    id: string
    code: string
    name: string
  }
  faculty: null | Ref
  is_active: boolean
  /** Aktif derslik sayısı */
  classroom_count: number
  version: number
}
export type RoomType = 'LECTURE' | 'LAB' | 'AMPHI' | 'OFFICE' | 'ONLINE'
export type ClassroomFeature =
  'PROJECTOR' | 'SMART_BOARD' | 'COMPUTERS' | 'ACCESSIBLE' | 'AIR_CONDITIONING' | 'SOUND_SYSTEM' | 'RECORDING'
export type Classroom = {
  id: string
  code: string
  name: string
  building: {
    id: string
    code: string
    name: string
  }
  campus: {
    id: string
    code: string
    name: string
  }
  capacity: number
  exam_capacity: number
  room_type: RoomType
  features: ClassroomFeature[]
  is_active: boolean
  version: number
}
export type ClassroomInput = {
  building_id: string
  code: string
  name: string
  capacity: number
  /** Kapasiteden büyük olamaz */
  exam_capacity: number
  room_type: RoomType
  features?: ClassroomFeature[]
}
export type AdmissionType =
  'OSYS' | 'DGS' | 'YOS' | 'TRANSFER_INTERNAL' | 'TRANSFER_EXTERNAL' | 'SPECIAL_TALENT' | 'EXCHANGE'
export type EnrollmentStatus =
  'PREP' | 'ACTIVE' | 'FROZEN' | 'SUSPENDED' | 'GRADUATED' | 'WITHDRAWN' | 'DISMISSED'
export type Advisor = {
  staff_id: string
  staff_no: string
  title: string | null
  first_name: string
  last_name: string
  since: string
}
export type StudentProgram = {
  id: string
  student: {
    id: string
    student_no: string
    first_name: string
    last_name: string
  }
  program: {
    id: string
    code: string
    name_tr: string
    department: {
      id: string
      name_tr: string
    }
    faculty: {
      id: string
      name_tr: string
    }
  }
  kind: 'MAJOR' | 'DOUBLE_MAJOR' | 'MINOR'
  admission_type: AdmissionType
  admission_year: number
  admitted_on: string
  status: EnrollmentStatus
  /** 0: hazırlık */
  class_level: number
  /** Hazırlık hariç kaçıncı yarıyılda olduğu */
  current_semester: number
  /** Programın normal süresi (yarıyıl) */
  normal_semesters: number
  /** Azami süre (yarıyıl): programın azami yılı × 2 */
  max_semesters: number
  gpa: number | null
  earned_ects: number
  version: number
  advisor: null | Advisor
  /** İzlenen müfredat sürümü; henüz bağlanmamışsa null */
  curriculum: null | {
    id: string
    name_tr: string
    name_en: string
  }
}
export type Student = {
  id: string
  student_no: string
  first_name: string
  last_name: string
  email: string | null
  programs: StudentProgram[]
}
export type AdvisorHistoryItem = Advisor & {
  /** Aktif danışmanlıkta null */
  until: string | null
  assigned_by: string | null
  reason: string | null
}
export type StatusHistoryItem = {
  from_status: null | EnrollmentStatus
  to_status: EnrollmentStatus
  reason: string | null
  decision_ref: string | null
  /** Değişikliği yapanın adı */
  changed_by: string | null
  changed_at: string
}
export type HoldType = 'TUITION_DEBT' | 'DISCIPLINE_SUSPENSION' | 'MISSING_DOCUMENT' | 'LIBRARY_DEBT'
export type HoldBlock = 'REGISTRATION' | 'DOCUMENT' | 'GRADUATION'
export type Hold = {
  id: string
  student_program_id: string
  student: StudentRef
  program: {
    id: string
    code: string
    name_tr: string
  }
  hold_type: HoldType
  blocks: HoldBlock[]
  reason: string
  /** Engeli koyanın adı */
  placed_by: string | null
  placed_at: string
  released_at: string | null
  /** Engeli kaldıranın adı */
  released_by: string | null
  release_reason: string | null
  active: boolean
}
export type TermStanding = 'NORMAL' | 'HONOR' | 'HIGH_HONOR'
export type TermResult = {
  /** Dönemde notu verilen derslerin AKTS'si */
  attempted_ects: number
  /** Bunlardan kazanılan AKTS */
  earned_ects: number
  /** YANO; ortalamaya giren ders yoksa null */
  term_gpa: number | null
  /** Dönem sonunda geçerli notlarla kazanılan toplam AKTS */
  cumulative_ects: number
  /** GANO (dönem sonundaki) */
  cumulative_gpa: number | null
  standing: TermStanding
}
export type Transcript = {
  student_program: StudentProgram
  program_name_en: string
  degree_level: 'ASSOCIATE' | 'BACHELOR' | 'MASTER' | 'PHD'
  terms: {
    term: {
      id: string
      code: string
      term_type: 'FALL' | 'SPRING' | 'SUMMER'
    }
    courses: {
      enrollment_id: string
      course: {
        id: string
        code: string
        name_tr: string
        name_en: string
      }
      ects: number
      credit: number
      /** ENROLLED: dönem sürüyor, notu yok */
      status: 'ENROLLED' | 'COMPLETED'
      letter_grade: string | null
      coefficient: number | null
      is_passing: boolean
      counts_in_gpa: boolean
      attempt_no: number
      /** Tekrar alınan ders (dersin ya da eşdeğerinin önceki denemesi var) ya da kalınan seçmelinin yerine alınan ders */
      is_repeat: boolean
      /** Yerini sonraki bir deneme aldı; sonraki dönemlerin GANO'suna girmez */
      superseded: boolean
      source: 'REGISTRATION' | 'ADD_DROP' | 'IMPORT'
      /** Sanal transkriptte bu derse verilebilecek harfler (kaydın not ölçeğinde derse verilebilenler, ölçek sırasıyla) */
      letter_options: string[]
    }[]
    /** Dönem sonucu; notu verilmiş dersi olmayan dönemde null */
    result: TermResult | null
  }[]
  summary: {
    /** Güncel GANO (son dönem sonucu) */
    gpa: number | null
    earned_ects: number
    /** İzlenen müfredatın (yoksa programın) mezuniyet için istediği AKTS */
    required_ects: number
    completed_courses: number
    /** Notu henüz verilmemiş ders */
    in_progress: number
    honor_terms: number
    high_honor_terms: number
  }
  generated_at: string
}
export type TranscriptSimulation = {
  terms: {
    term: {
      id: string
      code: string
      term_type: 'FALL' | 'SPRING' | 'SUMMER'
    }
    /** Gerçek harflerle dönem sonucu; notu verilmiş ders yoksa null */
    actual: TermResult | null
    /** Varsayımsal harflerle dönem sonucu */
    simulated: TermResult | null
  }[]
  summary: {
    actual_gpa: number | null
    simulated_gpa: number | null
    actual_earned_ects: number
    simulated_earned_ects: number
  }
}
export type StudentGradesTerm = {
  id: string
  code: string
  term_type: 'FALL' | 'SPRING' | 'SUMMER'
  is_current: boolean
}
export type StudentGrades = {
  student_program_id: string
  /** Öğrencinin ders aldığı dönemler, eskiden yeniye (dönem seçici için) */
  terms: StudentGradesTerm[]
  term: StudentGradesTerm
  courses: {
    enrollment_id: string
    course: {
      id: string
      code: string
      name_tr: string
      name_en: string
    }
    section_code: string
    ects: number
    status: 'ENROLLED' | 'COMPLETED'
    attendance_status: AttendanceStatus
    letter_grade: string | null
    is_passing: boolean | null
    /** Ders başarı puanı (ilandan sonra) */
    weighted_score: number | null
    grade_published_at: string | null
    components: {
      id: string
      type_code: string
      category: 'IN_TERM' | 'FINAL' | 'MAKEUP'
      label_tr: string
      label_en: string
      weight: number
      scheduled_on: string | null
      /** Yayımlanmadıysa null; not ve ortalama görünmez */
      published_at: string | null
      score: number | null
      /** Girmedi */
      absent: boolean
      /** Sınıfın girilen puanlarının ortalaması (girmedi hariç) */
      class_average: number | null
    }[]
    /** Sınıf istatistiği (harf notları ilan edildiyse) */
    class_stats: {
      student_count: number
      mean: number | null
      median: number | null
    } | null
  }[]
}
export type CourseAttempt = {
  id: string
  term: {
    id: string
    code: string
  }
  course: {
    id: string
    code: string
    name_tr: string
    name_en: string
  }
  section_code: string
  /** Dersin (ya da eşdeğerinin) kaçıncı alınışı */
  attempt_no: number
  /** ENROLLED: dönem sürüyor, COMPLETED: notu verildi */
  status: 'ENROLLED' | 'DROPPED' | 'COMPLETED'
  letter_grade: string | null
  /** Katsayısız harflerde (BŞR, MUAF) null */
  coefficient: number | null
  is_passing: boolean
  counts_in_gpa: boolean
  /** Devamsızlıktan kaldı */
  attendance_fail: boolean
  ects: number
  credit: number
  source: 'REGISTRATION' | 'ADD_DROP' | 'IMPORT'
  /** Yerini sonraki bir deneme aldı (son not geçerli) */
  superseded: boolean
}
export type ProgressStatus = 'PASSED' | 'FAILED' | 'IN_PROGRESS' | 'PARTIAL' | 'NOT_TAKEN'
export type AttemptRef = {
  course_id: string
  course_code: string
  course_name_tr: string
  course_name_en: string
  term_code: string
  letter: string | null
  ects: number
}
export type ProgressLine = {
  item_id: string
  semester_no: number
  item_type: 'COURSE' | 'ELECTIVE_SLOT'
  /** Dersin ya da havuzun kodu */
  code: string
  name_tr: string
  name_en: string
  ects: number
  is_compulsory: boolean
  slot_course_count: number
  status: ProgressStatus
  /** Ders satırının son denemesi (dersin kendisi ya da eşdeğeri) */
  last_attempt: null | AttemptRef
  /** Seçmeli yuvasını dolduran geçilmiş dersler */
  filled: AttemptRef[]
  /** Seçmeli yuvasına bu dönem alınan dersler */
  in_progress: AttemptRef[]
  /** Seçmeli yuvasında kalan ders sayısı */
  remaining: number
}
export type StudentProgress = {
  student_program_id: string
  curriculum: null | {
    id: string
    name_tr: string
    name_en: string
  }
  current_semester: number
  gpa: number | null
  summary: {
    required_ects: number
    completed_ects: number
    in_progress_ects: number
    /** Mezuniyet için kalan ders sayısı */
    remaining_courses: number
  }
  lines: ProgressLine[]
  /** Müfredatla eşleşmeyen derslerin son denemeleri */
  extra: AttemptRef[]
}
export type RegistrationStatus = 'NOT_STARTED' | 'DRAFT' | 'SUBMITTED' | 'APPROVED' | 'REJECTED'
export type RegistrationSlot = {
  day: number
  start: string
  end: string
  kind: 'THEORY' | 'PRACTICE' | 'LAB'
  classroom: string | null
}
export type RegistrationSection = {
  id: string
  offering_id: string
  course: {
    id: string
    code: string
    name_tr: string
    name_en: string
    ects: number
    credit: number
  }
  section_code: string
  label: string
  capacity: number
  /** Ayrılmış yer (sepetlerdeki şube sayısı) */
  enrolled: number
  /** Öğrenci bu şubede yer bulabilir mi */
  available: boolean
  quota_mode: 'OPEN' | 'RESERVED'
  /** Ayrılmış kontenjanlı şubede öğrencinin programına kalan yer (ayrılmamışsa 0) */
  quota_left: number | null
  language: 'TR' | 'EN'
  /** Unvanlı adlar, sorumlu önce */
  instructors: string[]
  slots: RegistrationSlot[]
}
export type RegistrationItem = {
  id: string
  /** Kalınan dersin tekrarı, not yükseltme, kalınan seçmelinin yerine başka seçmeli */
  kind: 'NEW' | 'REPEAT_FAILED' | 'REPEAT_UPGRADE' | 'ELECTIVE_REPLACEMENT'
  /** Karşıladığı müfredat satırı */
  curriculum_item_id: string | null
  ects: number
  source: 'REGISTRATION' | 'ADD_DROP'
  added_at: string
  section: RegistrationSection
}
export type RuleViolation = {
  /** Kural kodu */
  code:
    | 'HOLD'
    | 'PROGRAM_NOT_ACTIVE'
    | 'NO_CURRICULUM'
    | 'EMPTY_BASKET'
    | 'DUPLICATE_COURSE'
    | 'NOT_IN_CURRICULUM'
    | 'PREREQUISITE'
    | 'SCHEDULE_CONFLICT'
    | 'ECTS_LIMIT'
    | 'FIRST_YEAR_COMPULSORY'
    | 'REPEAT_PRIORITY'
    | 'MAX_DURATION'
    | 'REPEAT_UPGRADE'
    | 'ELECTIVE_SLOT_EXCEEDED'
  /** ERROR işlemi engeller, WARNING bilgi verir */
  severity: 'ERROR' | 'WARNING'
  course_ids: string[]
  /** Ör. limit, total, courses, missing */
  params: {
    [key: string]: any
  }
  /** Türkçe açıklama */
  message: string
}
export type RegistrationWindow = {
  /** Takvim penceresi açık */
  open: boolean
  /** Pencere kapalı ama kayda tanınan ek süre sürüyor */
  extended: boolean
  starts_at: string | null
  /** Bitiş anı hariç: [starts_at, ends_at) */
  ends_at: string | null
  next_starts_at: string | null
}
export type Registration = {
  /** Henüz sepet yoksa null */
  id: string | null
  student_program_id: string
  student: {
    id: string
    student_no: string
    first_name: string
    last_name: string
    program_code: string
    semester: number
    class_level: number
    gpa: number | null
    /** Danışmanın unvanlı adı */
    advisor: string | null
  }
  term: {
    id: string
    code: string
  }
  status: RegistrationStatus
  items: RegistrationItem[]
  total_ects: number
  /** GANO'ya göre bu dönemki AKTS üst sınırı */
  ects_limit: number
  /** Gönderim anındaki sınır */
  max_ects_allowed: number | null
  /** Gönderim anındaki GANO */
  gpa_snapshot: number | null
  submitted_at: string | null
  decided_at: string | null
  /** Karar verenin adı */
  decided_by: string | null
  decision_note: string | null
  extension_until: string | null
  extension_reason: string | null
  evaluation: {
    /** Gönderimi (ve onayı) engelleyen hata var */
    blocking: boolean
    violations: RuleViolation[]
  }
  registration_window: RegistrationWindow
  approval_window: RegistrationWindow
  /** İsteği yapanın bu kayıtta şu an yapabilecekleri */
  actions: {
    can_edit: boolean
    can_submit: boolean
    can_withdraw: boolean
    can_approve: boolean
    can_reject: boolean
    can_revoke: boolean
    can_extend: boolean
  }
  /** Son işlemin (ekleme, şube değiştirme) hata olmayan kural sonuçları */
  warnings: RuleViolation[]
  version: number
}
export type RegistrationOffer = {
  term: {
    id: string
    code: string
  }
  semester: number
  ects_limit: number
  /** Sepetteki AKTS */
  total_ects: number
  items: {
    course: {
      id: string
      code: string
      name_tr: string
      name_en: string
      ects: number
      credit: number
    }
    /** Alttan, dönem dersi, seçmeli, üst yarıyıl, not yükseltme */
    category: 'REPEAT' | 'CURRENT' | 'ELECTIVE' | 'UPPER' | 'UPGRADE'
    kind: 'NEW' | 'REPEAT_FAILED' | 'REPEAT_UPGRADE' | 'ELECTIVE_REPLACEMENT'
    /** Dersin karşıladığı müfredat satırı */
    line: {
      item_id: string
      semester_no: number
      item_type: 'COURSE' | 'ELECTIVE_SLOT'
      code: string
      name_tr: string
      name_en: string
      is_compulsory: boolean
      /** Seçmeli yuvasında kalan ders sayısı (ders satırında 0 ya da 1) */
      remaining: number
    }
    status: ProgressStatus
    last_letter: string | null
    prerequisite: null | RuleViolation
    sections: RegistrationSection[]
    /** Ders sepetteyse sepet satırı */
    basket_item_id: string | null
  }[]
}
export type RegistrationEvent = {
  id: number
  type:
    | 'CREATED'
    | 'ITEM_ADDED'
    | 'ITEM_REMOVED'
    | 'SECTION_CHANGED'
    | 'RETURNED_TO_DRAFT'
    | 'SUBMITTED'
    | 'WITHDRAWN'
    | 'APPROVED'
    | 'REJECTED'
    | 'APPROVAL_REVOKED'
    | 'EXTENSION_GRANTED'
    | 'EXPIRED'
    | 'ADD_DROP_REQUESTED'
    | 'ADD_DROP_APPROVED'
    | 'ADD_DROP_REJECTED'
    | 'ADD_DROP_APPLIED'
    | 'ADD_DROP_WITHDRAWN'
  /** Yapanın adı; sistem işinde null */
  actor: string | null
  payload: {
    [key: string]: any
  }
  occurred_at: string
}
export type RuleProblem = Problem & {
  violations: RuleViolation[]
}
export type RegistrationSummary = {
  student_program_id: string
  student: StudentRef
  program_code: string
  semester: number
  gpa: number | null
  registration_id: string | null
  status: RegistrationStatus
  total_ects: number
  item_count: number
  submitted_at: string | null
  /** Toplu onayda gönderilecek sürüm */
  version: number
  /** Ders seçmeyi kapatan aktif engeli var */
  blocked: boolean
}
export type RegistrationSummaryList = {
  term: {
    id: string
    code: string
  }
  items: RegistrationSummary[]
  next_cursor: string | null
}
export type AddDropDecision = {
  /** Karar verenin adı */
  by: string | null
  at: string | null
  note: string | null
}
export type AddDropRequest = {
  id: string
  registration_id: string
  student_program_id: string
  student: {
    id: string
    student_no: string
    first_name: string
    last_name: string
    program_code: string
  }
  term_id: string
  status: 'PENDING_ADVISOR' | 'PENDING_DEPT_HEAD' | 'APPROVED' | 'REJECTED' | 'WITHDRAWN'
  /** Karar bekleyen aşama */
  stage: ('ADVISOR' | 'DEPT_HEAD' | null) | ('ADVISOR' | 'DEPT_HEAD' | null)
  student_note: string | null
  advisor: null | AddDropDecision
  dept_head: null | AddDropDecision
  items: {
    id: string
    action: 'ADD' | 'DROP'
    ects: number
    section: RegistrationSection
  }[]
  /** Talebin AKTS'ye etkisi (eklenen − bırakılan) */
  ects_delta: number
  actions: {
    can_withdraw: boolean
    can_approve: boolean
    can_reject: boolean
  }
  version: number
  created_at: string
}
export type EligibleAdvisor = {
  staff_id: string
  staff_no: string
  title: string | null
  first_name: string
  last_name: string
  /** Şu anki danışmanlık sayısı */
  active_count: number
}
export type UnitType = 'FACULTY' | 'VOCATIONAL_SCHOOL' | 'SCHOOL' | 'INSTITUTE' | 'CONSERVATORY'
export type Faculty = {
  id: string
  code: string
  name_tr: string
  name_en: string
  unit_type: UnitType
  campus: null | {
    id: string
    code: string
    name: string
  }
  is_active: boolean
}
export type Department = {
  id: string
  code: string
  name_tr: string
  name_en: string
  faculty: Ref
  is_active: boolean
}
export type DegreeLevel = 'ASSOCIATE' | 'BACHELOR' | 'MASTER' | 'PHD'
export type Language = 'TR' | 'EN' | 'MIXED'
export type EducationType = 'DAYTIME' | 'EVENING' | 'DISTANCE'
export type Program = {
  id: string
  code: string
  yoksis_code: string | null
  name_tr: string
  name_en: string
  degree_level: DegreeLevel
  language: Language
  education_type: EducationType
  duration_semesters: number
  max_duration_years: number
  total_ects_required: number
  has_prep_class: boolean
  department: Ref
  faculty: Ref
  is_active: boolean
}
export const {
  useHealthzQuery,
  useLazyHealthzQuery,
  useReadyzQuery,
  useLazyReadyzQuery,
  useLoginMutation,
  useVerifyMfaMutation,
  useRefreshMutation,
  useLogoutMutation,
  useForgotPasswordMutation,
  useVerifyResetTokenMutation,
  useResetPasswordMutation,
  useGetMeQuery,
  useLazyGetMeQuery,
  useGetMyMfaQuery,
  useLazyGetMyMfaQuery,
  useStartMfaSetupMutation,
  useEnableMfaMutation,
  useDisableMfaMutation,
  useRegenerateRecoveryCodesMutation,
  useChangePasswordMutation,
  useListMySessionsQuery,
  useLazyListMySessionsQuery,
  useEndOtherSessionsMutation,
  useEndSessionMutation,
  useListMySecurityEventsQuery,
  useLazyListMySecurityEventsQuery,
  useGetMyPermissionsQuery,
  useLazyGetMyPermissionsQuery,
  useListUsersQuery,
  useLazyListUsersQuery,
  useCreateUserMutation,
  useSetUserStatusMutation,
  useResendActivationEmailMutation,
  useSendPasswordResetEmailMutation,
  useResetUserMfaMutation,
  useListRolesQuery,
  useLazyListRolesQuery,
  useListUserRolesQuery,
  useLazyListUserRolesQuery,
  useAssignRoleMutation,
  useEndRoleAssignmentMutation,
  useGetUserQuery,
  useLazyGetUserQuery,
  useListAuditLogQuery,
  useLazyListAuditLogQuery,
  useListSecurityEventsQuery,
  useLazyListSecurityEventsQuery,
  useListAcademicYearsQuery,
  useLazyListAcademicYearsQuery,
  useCreateAcademicYearMutation,
  useCreateTermMutation,
  useListTermsQuery,
  useLazyListTermsQuery,
  useGetCurrentTermQuery,
  useLazyGetCurrentTermQuery,
  useGetTermQuery,
  useLazyGetTermQuery,
  useUpdateTermMutation,
  useMakeTermCurrentMutation,
  useListCalendarEventTypesQuery,
  useLazyListCalendarEventTypesQuery,
  useGetCalendarWindowsQuery,
  useLazyGetCalendarWindowsQuery,
  useListTermEventsQuery,
  useLazyListTermEventsQuery,
  useCreateCalendarEventMutation,
  useGetCalendarEventQuery,
  useLazyGetCalendarEventQuery,
  useUpdateCalendarEventMutation,
  useDeleteCalendarEventMutation,
  useListCoursesQuery,
  useLazyListCoursesQuery,
  useCreateCourseMutation,
  useGetCourseQuery,
  useLazyGetCourseQuery,
  useUpdateCourseMutation,
  useSetCoursePrerequisitesMutation,
  useAddCourseEquivalenceMutation,
  useDeleteCourseEquivalenceMutation,
  useListElectiveGroupsQuery,
  useLazyListElectiveGroupsQuery,
  useCreateElectiveGroupMutation,
  useGetElectiveGroupQuery,
  useLazyGetElectiveGroupQuery,
  useUpdateElectiveGroupMutation,
  useAddElectiveGroupCourseMutation,
  useRemoveElectiveGroupCourseMutation,
  useListProgramCurriculaQuery,
  useLazyListProgramCurriculaQuery,
  useCreateCurriculumMutation,
  useGetCurriculumQuery,
  useLazyGetCurriculumQuery,
  useUpdateCurriculumMutation,
  useDeleteCurriculumMutation,
  useActivateCurriculumMutation,
  useArchiveCurriculumMutation,
  useAddCurriculumItemMutation,
  useUpdateCurriculumItemMutation,
  useDeleteCurriculumItemMutation,
  useMyCurriculaQuery,
  useLazyMyCurriculaQuery,
  useListGradeScalesQuery,
  useLazyListGradeScalesQuery,
  useCreateGradeScaleMutation,
  useGetGradeScaleQuery,
  useLazyGetGradeScaleQuery,
  useUpdateGradeScaleMutation,
  useListRegulationParametersQuery,
  useLazyListRegulationParametersQuery,
  useGetRegulationParameterHistoryQuery,
  useLazyGetRegulationParameterHistoryQuery,
  useSetRegulationParameterMutation,
  useListOfferingsQuery,
  useLazyListOfferingsQuery,
  useCreateOfferingMutation,
  useGetOfferingQuery,
  useLazyGetOfferingQuery,
  useUpdateOfferingMutation,
  useDeleteOfferingMutation,
  useCreateSectionMutation,
  useGetSectionQuery,
  useLazyGetSectionQuery,
  useUpdateSectionMutation,
  useDeleteSectionMutation,
  useSetSectionInstructorsMutation,
  useSetSectionQuotasMutation,
  useAddScheduleSlotMutation,
  useUpdateScheduleSlotMutation,
  useDeleteScheduleSlotMutation,
  useTermScheduleQuery,
  useLazyTermScheduleQuery,
  useMyTeachingQuery,
  useLazyMyTeachingQuery,
  useSearchInstructorsQuery,
  useLazySearchInstructorsQuery,
  useListAssessmentTypesQuery,
  useLazyListAssessmentTypesQuery,
  useGetAssessmentPlanQuery,
  useLazyGetAssessmentPlanQuery,
  useSetAssessmentPlanMutation,
  useLockAssessmentPlanMutation,
  useUnlockAssessmentPlanMutation,
  useGetGradebookQuery,
  useLazyGetGradebookQuery,
  useExportGradebookQuery,
  useLazyExportGradebookQuery,
  useSetComponentScoresMutation,
  usePreviewScoreImportMutation,
  useImportScoresMutation,
  usePublishComponentScoresMutation,
  useSetAttendanceMutation,
  useGetLetterGradesQuery,
  useLazyGetLetterGradesQuery,
  useSetLetterOverridesMutation,
  usePublishLetterGradesMutation,
  usePublishMakeupMutation,
  useGetSectionGradeStatsQuery,
  useLazyGetSectionGradeStatsQuery,
  usePreviewGradeChangeQuery,
  useLazyPreviewGradeChangeQuery,
  useRequestGradeChangeMutation,
  useListGradeChangesQuery,
  useLazyListGradeChangesQuery,
  useGetGradeChangeQuery,
  useLazyGetGradeChangeQuery,
  useApproveGradeChangeMutation,
  useRejectGradeChangeMutation,
  useWithdrawGradeChangeMutation,
  useListBuildingsQuery,
  useLazyListBuildingsQuery,
  useCreateBuildingMutation,
  useGetBuildingQuery,
  useLazyGetBuildingQuery,
  useUpdateBuildingMutation,
  useListClassroomsQuery,
  useLazyListClassroomsQuery,
  useCreateClassroomMutation,
  useGetClassroomQuery,
  useLazyGetClassroomQuery,
  useUpdateClassroomMutation,
  useListStudentsQuery,
  useLazyListStudentsQuery,
  useGetStudentQuery,
  useLazyGetStudentQuery,
  useCreateStudentProgramMutation,
  useAssignAdvisorMutation,
  useListAdvisorHistoryQuery,
  useLazyListAdvisorHistoryQuery,
  useChangeStudentProgramStatusMutation,
  useListStudentProgramStatusHistoryQuery,
  useLazyListStudentProgramStatusHistoryQuery,
  useListStudentProgramHoldsQuery,
  useLazyListStudentProgramHoldsQuery,
  usePlaceHoldMutation,
  useListHoldsQuery,
  useLazyListHoldsQuery,
  useReleaseHoldMutation,
  useGetTranscriptQuery,
  useLazyGetTranscriptQuery,
  useSimulateTranscriptQuery,
  useLazySimulateTranscriptQuery,
  useGetStudentGradesQuery,
  useLazyGetStudentGradesQuery,
  useListCourseHistoryQuery,
  useLazyListCourseHistoryQuery,
  useGetStudentProgressQuery,
  useLazyGetStudentProgressQuery,
  useGetRegistrationQuery,
  useLazyGetRegistrationQuery,
  useListRegistrationOfferQuery,
  useLazyListRegistrationOfferQuery,
  useListRegistrationEventsQuery,
  useLazyListRegistrationEventsQuery,
  useAddRegistrationItemMutation,
  useChangeRegistrationSectionMutation,
  useRemoveRegistrationItemMutation,
  useSubmitRegistrationMutation,
  useWithdrawRegistrationMutation,
  useGrantRegistrationExtensionMutation,
  useListRegistrationsQuery,
  useLazyListRegistrationsQuery,
  useBulkApproveRegistrationsMutation,
  useGetRegistrationByIdQuery,
  useLazyGetRegistrationByIdQuery,
  useApproveRegistrationMutation,
  useRejectRegistrationMutation,
  useRevokeRegistrationMutation,
  useListAdviseeRegistrationsQuery,
  useLazyListAdviseeRegistrationsQuery,
  useListAddDropRequestsQuery,
  useLazyListAddDropRequestsQuery,
  useRequestAddDropMutation,
  useWithdrawAddDropMutation,
  useApproveAddDropMutation,
  useRejectAddDropMutation,
  useListAddDropQueueQuery,
  useLazyListAddDropQueueQuery,
  useListEligibleAdvisorsQuery,
  useLazyListEligibleAdvisorsQuery,
  useListMyProgramsQuery,
  useLazyListMyProgramsQuery,
  useListMyAdviseesQuery,
  useLazyListMyAdviseesQuery,
  useListFacultiesQuery,
  useLazyListFacultiesQuery,
  useGetFacultyQuery,
  useLazyGetFacultyQuery,
  useListFacultyDepartmentsQuery,
  useLazyListFacultyDepartmentsQuery,
  useGetDepartmentQuery,
  useLazyGetDepartmentQuery,
  useListProgramsQuery,
  useLazyListProgramsQuery,
  useGetProgramQuery,
  useLazyGetProgramQuery,
} = injectedRtkApi
