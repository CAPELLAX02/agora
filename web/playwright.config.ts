import { defineConfig, devices } from '@playwright/test'

// Uçtan uca testler gerçek yığına karşı çalışır: API, worker (e-postalar), web, ayrı
// bir veritabanı (agora_e2e) ve Redis'in 1 numaralı veritabanı. Önce e2e/setup-db.sh.
const api = 'http://localhost:18081'
const web = 'http://localhost:4174'

const backendEnv = {
  AGORA_ENV: 'development',
  AGORA_DATABASE_URL: 'postgres://agora:agora_dev_password@localhost:5432/agora_e2e?sslmode=disable',
  AGORA_REDIS_URL: 'redis://localhost:6379/1',
  AGORA_HTTP_ADDR: ':18081',
  AGORA_METRICS_ADDR: ':19191',
  AGORA_WORKER_METRICS_ADDR: ':19193',
  AGORA_WEB_BASE_URL: web,
  AGORA_CORS_ALLOWED_ORIGINS: web,
  // Testler aynı IP'den çok sayıda giriş yapar.
  AGORA_LOGIN_RATE_LIMIT: '1000',
  AGORA_LOG_LEVEL: 'warn',
}

export default defineConfig({
  testDir: './e2e',
  // Senaryolar ortak bir veritabanını sırayla değiştirir (ör. yöneticinin MFA'sı).
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: web,
    locale: 'tr-TR',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: 'go -C ../backend run ./cmd/api',
      url: `${api}/readyz`,
      env: backendEnv,
      timeout: 120_000,
      reuseExistingServer: false,
    },
    {
      command: 'go -C ../backend run ./cmd/worker',
      url: 'http://localhost:19193/metrics',
      env: backendEnv,
      timeout: 120_000,
      reuseExistingServer: false,
    },
    {
      command: './node_modules/.bin/vite --port 4174 --strictPort',
      url: web,
      env: { AGORA_API_URL: api },
      timeout: 60_000,
      reuseExistingServer: false,
    },
  ],
})
