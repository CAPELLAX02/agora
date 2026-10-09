import { combineReducers, configureStore } from '@reduxjs/toolkit'

import { authReducer } from '@/features/auth/authSlice'
import { baseApi } from '@/shared/api/baseApi'

const rootReducer = combineReducers({
  auth: authReducer,
  [baseApi.reducerPath]: baseApi.reducer,
})

export type RootState = ReturnType<typeof rootReducer>

/** setupStore, uygulamanın Redux deposunu kurar. Testler kendi depolarını bununla kurar. */
export function setupStore(preloadedState?: Partial<RootState>) {
  return configureStore({
    reducer: rootReducer,
    preloadedState,
    middleware: (getDefault) => getDefault().concat(baseApi.middleware),
  })
}

export type AppStore = ReturnType<typeof setupStore>
export type AppDispatch = AppStore['dispatch']
