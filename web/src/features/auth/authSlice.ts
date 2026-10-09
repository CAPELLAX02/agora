import { createSlice, type PayloadAction } from '@reduxjs/toolkit'

export type AuthStatus = 'restoring' | 'authenticated' | 'anonymous'

/** SignedOutReason, kullanıcının neden oturumsuz kaldığıdır: giriş ekranında gösterilir. */
export type SignedOutReason = 'expired' | 'loggedOut' | 'disabled' | null

export type AuthState = {
  status: AuthStatus
  /** accessToken sadece bellekte tutulur: XSS ile kalıcı olarak çalınamaz. Refresh token HttpOnly çerezdedir. */
  accessToken: string | null
  sessionId: string | null
  mustChangePassword: boolean
  reason: SignedOutReason
}

export type SessionTokens = {
  access_token: string
  session_id: string
  must_change_password: boolean
}

const initialState: AuthState = {
  status: 'restoring',
  accessToken: null,
  sessionId: null,
  mustChangePassword: false,
  reason: null,
}

const signedOutState = (reason: SignedOutReason): AuthState => ({
  ...initialState,
  status: 'anonymous',
  reason,
})

export const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    /** signedIn, giriş ya da token yenilemesi sonrası yeni token'ları kaydeder. */
    signedIn(state, action: PayloadAction<SessionTokens>) {
      state.status = 'authenticated'
      state.accessToken = action.payload.access_token
      state.sessionId = action.payload.session_id
      state.mustChangePassword = action.payload.must_change_password
      state.reason = null
    },
    /** restoreFailed, açılışta geçerli bir oturum bulunamadığını bildirir. */
    restoreFailed: () => signedOutState(null),
    sessionExpired: () => signedOutState('expired'),
    accountDisabled: () => signedOutState('disabled'),
    signedOut: () => signedOutState('loggedOut'),
    passwordChanged(state) {
      state.mustChangePassword = false
    },
    passwordChangeRequired(state) {
      state.mustChangePassword = true
    },
  },
})

export const {
  signedIn,
  restoreFailed,
  sessionExpired,
  accountDisabled,
  signedOut,
  passwordChanged,
  passwordChangeRequired,
} = authSlice.actions

export const authReducer = authSlice.reducer
