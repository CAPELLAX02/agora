// registration-rush: ders seçme açılışı stres testi.
//
// Öğrenciler artan bir hızla (ramping-arrival-rate) gelir: giriş yapar, alınabilir derslerini
// ister, alttan ve dönem derslerini, sonra seçmelilerini sepete ekler ve sepeti danışmana
// gönderir. Aynı yarıyıldaki öğrenciler aynı seçmeli şubelerine (kontenjanı 40) saldırır:
// kontenjan aşımı olmamalı. Kanıt testten sonra SQL ile alınır (make loadtest-verify).
//
// Beklenen 4xx'ler hata sayılmaz: 409 (kontenjan dolu, pencere...) ve 422 (kural ihlali)
// ders seçmenin olağan yanıtlarıdır. Hedefler: 5xx < %0,5, ders seçme uçlarında p95 < 800 ms.
//
// PRELOGIN=1: öğrenciler pencere açılmadan giriş yapmış olur (tokenlar kurulumda, paralel
// alınır); sadece ders seçme yükü ölçülür. Varsayılan en kötü durumdur: giriş ve ders seçme
// aynı anda. Girişte parola argon2id ile doğrulanır ve bilerek pahalıdır (CPU ve bellek).
import http from 'k6/http'
import { check } from 'k6'
import exec from 'k6/execution'
import { SharedArray } from 'k6/data'
import { Counter } from 'k6/metrics'

const BASE = __ENV.BASE_URL || 'http://host.docker.internal:8080'
const PASSWORD = __ENV.PASSWORD || 'agora-dev-parola'
const PEAK = Number(__ENV.PEAK_RATE || 25) // tepe noktada saniyede gelen öğrenci
const RAMP = __ENV.RAMP || '1m'
const HOLD = __ENV.HOLD || '1m'
const RUN = __ENV.RUN_ID || String(Math.floor(Date.now() / 1000))
const PRELOGIN = __ENV.PRELOGIN === '1'

const students = new SharedArray('students', () =>
  open('../data/students.csv')
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean),
)

export const options = {
  scenarios: {
    rush: {
      executor: 'ramping-arrival-rate',
      startRate: 0,
      timeUnit: '1s',
      preAllocatedVUs: 100,
      maxVUs: 800,
      stages: [
        { duration: RAMP, target: PEAK },
        { duration: HOLD, target: PEAK },
        { duration: '15s', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.005'],
    'http_req_duration{kind:registration}': ['p(95)<800'],
    checks: ['rate>0.99'],
  },
  summaryTrendStats: ['avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  setupTimeout: '5m',
}

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 299 }, 409, 422))

const added = new Counter('items_added')
const sectionFull = new Counter('section_full')
const ruleRejected = new Counter('rule_rejected')
const submitted = new Counter('registrations_submitted')

function loginRequest(username) {
  return {
    method: 'POST',
    url: `${BASE}/api/v1/auth/login`,
    body: JSON.stringify({ username, password: PASSWORD }),
    params: { headers: { 'Content-Type': 'application/json', 'X-Agora-Client': 'mobile' }, tags: { kind: 'login' } },
  }
}

// setup, PRELOGIN kipinde bütün öğrencilerin tokenlarını 16'şarlı paralel gruplarla alır.
export function setup() {
  if (!PRELOGIN) return { tokens: [] }
  const tokens = []
  for (let i = 0; i < students.length; i += 16) {
    const batch = students.slice(i, i + 16)
    http.batch(batch.map(loginRequest)).forEach((r) => tokens.push(r.status === 200 ? r.json('access_token') : null))
  }
  return { tokens }
}

export default function (data) {
  const i = exec.scenario.iterationInTest
  if (i >= students.length) return // bütün öğrenciler geldi
  const username = students[i]

  let token = data.tokens[i]
  if (!PRELOGIN) {
    const { method, url, body, params } = loginRequest(username)
    const login = http.request(method, url, body, params)
    if (!check(login, { 'giriş başarılı': (r) => r.status === 200 })) return
    token = login.json('access_token')
  }
  if (!check(token, { 'token var': (t) => !!t })) return
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }
  const get = (path) => http.get(`${BASE}${path}`, { headers, tags: { kind: 'registration' } })

  const sp = get('/api/v1/me/programs').json('items.0.id')
  const offer = get(`/api/v1/student-programs/${sp}/registration/offer`)
  if (!check(offer, { 'alınabilir dersler': (r) => r.status === 200 })) return
  const semester = offer.json('semester')

  const taken = {}
  for (const o of offer.json('items')) {
    if (!['REPEAT', 'CURRENT', 'ELECTIVE'].includes(o.category) || o.prerequisite) continue
    if (o.category === 'ELECTIVE') {
      if (o.line.semester_no > semester || (taken[o.line.item_id] || 0) >= o.line.remaining) continue
    }
    for (const sec of o.sections) {
      const r = http.post(
        `${BASE}/api/v1/student-programs/${sp}/registration/items`,
        JSON.stringify({ section_id: sec.id }),
        { headers: { ...headers, 'Idempotency-Key': `rush-${RUN}-${username}-${sec.id}` }, tags: { kind: 'registration' } },
      )
      check(r, { 'ekleme yanıtı beklenen': (res) => [201, 409, 422].includes(res.status) })
      if (r.status === 201) {
        added.add(1)
        taken[o.line.item_id] = (taken[o.line.item_id] || 0) + 1
        break
      }
      if (r.status === 409 && r.json('code') === 'SECTION_FULL') sectionFull.add(1)
      else if (r.status === 422) ruleRejected.add(1)
    }
  }

  const reg = get(`/api/v1/student-programs/${sp}/registration`)
  if (reg.status === 200 && reg.json('actions.can_submit')) {
    const r = http.post(`${BASE}/api/v1/student-programs/${sp}/registration/submit`, null, {
      headers: { ...headers, 'If-Match': reg.headers['Etag'], 'Idempotency-Key': `rush-${RUN}-${username}-submit` },
      tags: { kind: 'registration' },
    })
    check(r, { 'gönderim yanıtı beklenen': (res) => [200, 422].includes(res.status) })
    if (r.status === 200) submitted.add(1)
  }
}
