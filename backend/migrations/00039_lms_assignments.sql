-- +goose Up

-- Ödev: teslimin açılışı, son tarih ve kesin kapanış; deneme hakkı; teslim türleri (dosya,
-- çevrimiçi metin) ve dosya sınırları; geç teslim politikası. Puanlar öğrenci başına tutulur
-- ve ödev bazında yayımlanır (yayımlanana kadar öğrenci görmez).
CREATE TABLE lms.assignments (
    activity_id              uuid         PRIMARY KEY REFERENCES lms.activities (id),
    opens_at                 timestamptz,
    due_at                   timestamptz,
    cutoff_at                timestamptz,
    max_attempts             smallint     NOT NULL DEFAULT 1 CHECK (max_attempts BETWEEN 1 AND 10),
    submission_types         text[]       NOT NULL CHECK (cardinality(submission_types) BETWEEN 1 AND 2
                                                         AND submission_types <@ ARRAY['FILE', 'ONLINE_TEXT']),
    max_files                smallint     NOT NULL DEFAULT 5 CHECK (max_files BETWEEN 1 AND 20),
    max_file_size_bytes      bigint       NOT NULL CHECK (max_file_size_bytes > 0),
    -- İzin verilen uzantılar (".pdf"); boşsa platformun izin listesindeki her tür.
    allowed_extensions       text[]       NOT NULL DEFAULT '{}',
    max_score                numeric(6,2) NOT NULL DEFAULT 100 CHECK (max_score > 0 AND max_score <= 1000),
    late_policy              text         NOT NULL DEFAULT 'ALLOW' CHECK (late_policy IN ('ALLOW', 'DENY', 'PENALTY')),
    -- PENALTY: son tarihten sonraki her (başlamış) gün için puandan düşülen yüzde.
    late_penalty_pct_per_day numeric(5,2) NOT NULL DEFAULT 0 CHECK (late_penalty_pct_per_day BETWEEN 0 AND 100),
    grades_released_at       timestamptz,
    CONSTRAINT assignments_dates_check CHECK (
        (opens_at IS NULL OR due_at IS NULL OR opens_at < due_at)
        AND (due_at IS NULL OR cutoff_at IS NULL OR due_at <= cutoff_at)
        AND (opens_at IS NULL OR cutoff_at IS NULL OR opens_at < cutoff_at))
);
-- Zaman çizelgesi (yaklaşan ve geciken teslimler).
CREATE INDEX assignments_due_idx ON lms.assignments (due_at) WHERE due_at IS NOT NULL;

-- Teslimler: öğrencinin her denemesi bir satır. Taslak serbestçe düzenlenir; gönderilen
-- deneme kilitlenir, hak kaldıysa yeni deneme önceki içerikle başlar.
CREATE TABLE lms.assignment_submissions (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    activity_id      uuid        NOT NULL REFERENCES lms.assignments (activity_id),
    user_id          uuid        NOT NULL REFERENCES iam.users (id),
    attempt_no       smallint    NOT NULL CHECK (attempt_no >= 1),
    status           text        NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'SUBMITTED')),
    online_text_html text        NOT NULL DEFAULT '',
    submitted_at     timestamptz,
    is_late          boolean     NOT NULL DEFAULT false,
    version          integer     NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (activity_id, user_id, attempt_no),
    CONSTRAINT assignment_submissions_submitted_check CHECK ((status = 'SUBMITTED') = (submitted_at IS NOT NULL))
);

-- Teslimin dosyaları. Yeni deneme öncekinin dosyalarını paylaşır: dosya, hiçbir teslim onu
-- göstermeyince serbest kalır.
CREATE TABLE lms.submission_files (
    submission_id uuid    NOT NULL REFERENCES lms.assignment_submissions (id),
    file_id       uuid    NOT NULL REFERENCES platform.files (id),
    position      integer NOT NULL,
    PRIMARY KEY (submission_id, file_id)
);
CREATE INDEX submission_files_file_idx ON lms.submission_files (file_id);

-- Puan ve geri bildirim: öğrenci başına bir tane (teslim etmeyene de verilebilir). Puan, verildiği
-- andaki son denemeye aittir; geç teslim kesintisi okunurken o denemenin teslim anından hesaplanır.
CREATE TABLE lms.assignment_grades (
    activity_id   uuid         NOT NULL REFERENCES lms.assignments (activity_id),
    user_id       uuid         NOT NULL REFERENCES iam.users (id),
    score         numeric(6,2) CHECK (score >= 0),
    feedback_html text         NOT NULL DEFAULT '',
    attempt_no    smallint,
    graded_by     uuid         NOT NULL REFERENCES iam.users (id),
    graded_at     timestamptz  NOT NULL DEFAULT now(),
    version       integer      NOT NULL DEFAULT 1,
    PRIMARY KEY (activity_id, user_id)
);

-- Gönderim yorumları: öğrenci ile öğretim elemanları arasında, öğrencinin teslimi üzerine.
CREATE TABLE lms.submission_comments (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    activity_id uuid        NOT NULL REFERENCES lms.assignments (activity_id),
    student_id  uuid        NOT NULL REFERENCES iam.users (id),
    author_id   uuid        NOT NULL REFERENCES iam.users (id),
    body        text        NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submission_comments_thread_idx ON lms.submission_comments (activity_id, student_id, created_at);

-- +goose Down
DROP TABLE lms.submission_comments;
DROP TABLE lms.assignment_grades;
DROP TABLE lms.submission_files;
DROP TABLE lms.assignment_submissions;
DROP TABLE lms.assignments;
