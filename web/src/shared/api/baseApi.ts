import { createApi } from '@reduxjs/toolkit/query/react'

import { baseQueryWithReauth } from './baseQuery'

/**
 * baseApi, bütün API uçlarının bağlandığı RTK Query örneğidir. Uçlar sözleşmeden
 * üretilir (generated.ts) ve enhanced.ts'de önbellek davranışıyla zenginleştirilir.
 */
export const baseApi = createApi({
  reducerPath: 'api',
  baseQuery: baseQueryWithReauth,
  endpoints: () => ({}),
})
