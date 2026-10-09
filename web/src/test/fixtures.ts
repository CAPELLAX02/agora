import type { Grant, Me, Tokens } from '@/shared/api/generated'

export const tokens = (over: Partial<Tokens> = {}): Tokens => ({
  access_token: 'access-1',
  token_type: 'Bearer',
  expires_in: 900,
  session_id: '01a11d00-0000-7000-8000-000000000001',
  must_change_password: false,
  ...over,
})

export const me = (over: Partial<Me> = {}): Me => ({
  id: '01a11d00-0000-7000-8000-0000000000aa',
  username: 'P90001',
  email: 'yonetici@agora.test',
  first_name: 'Ayşe',
  last_name: 'Yılmaz',
  status: 'ACTIVE',
  must_change_password: false,
  mfa_enabled: false,
  last_login_at: '2026-10-08T09:00:00Z',
  roles: [
    {
      role: 'SYSTEM_ADMIN',
      role_name: 'Sistem Yöneticisi',
      scope_type: 'UNIVERSITY',
      scope_id: null,
      scope_name: null,
      valid_until: null,
    },
  ],
  session_id: '01a11d00-0000-7000-8000-000000000001',
  mfa_required: false,
  ...over,
})

export const grants = (...permissions: string[]): { items: Grant[] } => ({
  items: permissions.map((permission) => ({ permission, scope_type: 'UNIVERSITY', scope_id: null })),
})

/** problem, RFC 9457 hata gövdesidir. */
export const problem = (status: number, code: string, extra: Record<string, unknown> = {}) => ({
  type: 'about:blank',
  title: 'Hata',
  status,
  code,
  detail: code,
  ...extra,
})
