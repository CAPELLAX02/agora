import { combineReducers, configureStore, isPlain } from '@reduxjs/toolkit'

import { authReducer } from '@/features/auth/authSlice'
import { baseApi } from '@/shared/api/baseApi'

const rootReducer = combineReducers({
  auth: authReducer,
  [baseApi.reducerPath]: baseApi.reducer,
})

export type RootState = ReturnType<typeof rootReducer>

// Blob (ve File) etiketiyle tanınır: test ortamında yanıtın Blob'u global Blob'un örneği değildir.
const isBlob = (v: unknown) => ['[object Blob]', '[object File]'].includes(Object.prototype.toString.call(v))

/** setupStore, uygulamanın Redux deposunu kurar. Testler kendi depolarını bununla kurar. */
export function setupStore(preloadedState?: Partial<RootState>) {
  return configureStore({
    reducer: rootReducer,
    preloadedState,
    // Dosya yükleme ve indirme uçları Blob taşır (istek gövdesi, Excel yanıtı): bilinçli olarak
    // serileştirilebilir sayılır.
    middleware: (getDefault) =>
      getDefault({
        serializableCheck: { isSerializable: (v: unknown) => isPlain(v) || isBlob(v) },
      }).concat(baseApi.middleware),
  })
}

export type AppStore = ReturnType<typeof setupStore>
export type AppDispatch = AppStore['dispatch']
