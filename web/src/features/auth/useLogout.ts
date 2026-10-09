import { useCallback } from 'react'
import { useNavigate } from 'react-router'

import { useAppDispatch } from '@/app/hooks'
import { baseApi } from '@/shared/api/baseApi'
import { useLogoutMutation } from '@/shared/api/generated'

import { signedOut } from './authSlice'

/**
 * useLogout, oturumu sunucuda sonlandırır ve yerel durumu temizler. Sunucuya
 * ulaşılamasa da kullanıcı çıkış yapmış sayılır: access token bellekten silinir.
 * Giriş ekranı "geri dönülecek sayfa" hatırlamaz: sonraki kullanıcı başkası olabilir.
 */
export function useLogout() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const [logout] = useLogoutMutation()
  return useCallback(() => {
    void logout({ 'X-Agora-Client': 'web', refreshTokenBody: {} }).finally(() => {
      dispatch(signedOut())
      dispatch(baseApi.util.resetApiState())
      void navigate('/giris', { replace: true })
    })
  }, [dispatch, logout, navigate])
}
