-- +goose Up
-- Akademik takvim: yıllar, dönemler ve tipli zaman pencereleri (ders seçme, danışman
-- onayı, not girişi ...). Kayıt ve not kuralları "pencere açık mı?" sorusunu bu
-- tablolara sorar.
CREATE SCHEMA academic;

CREATE TABLE academic.academic_years (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    start_year smallint    NOT NULL UNIQUE CHECK (start_year BETWEEN 1946 AND 2100), -- 2026 → "2026-2027"
    starts_on  date        NOT NULL,
    ends_on    date        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT academic_years_period_check CHECK (ends_on > starts_on)
);

CREATE TABLE academic.terms (
    id               uuid        PRIMARY KEY DEFAULT uuidv7(),
    academic_year_id uuid        NOT NULL REFERENCES academic.academic_years (id),
    term_type        text        NOT NULL CHECK (term_type IN ('FALL', 'SPRING', 'SUMMER')),
    code             text        NOT NULL UNIQUE CHECK (code ~ '^[0-9]{4}-(FALL|SPRING|SUMMER)$'),
    starts_on        date        NOT NULL,
    ends_on          date        NOT NULL,
    status           text        NOT NULL DEFAULT 'PLANNED' CHECK (status IN ('PLANNED', 'ACTIVE', 'CLOSED')),
    is_current       boolean     NOT NULL DEFAULT false,
    version          integer     NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT terms_year_type_key UNIQUE (academic_year_id, term_type),
    CONSTRAINT terms_period_check CHECK (ends_on > starts_on),
    -- Kapanmış dönem "aktif dönem" olamaz.
    CONSTRAINT terms_current_check CHECK (NOT is_current OR status <> 'CLOSED')
);

-- Aynı anda tek bir aktif (içinde bulunulan) dönem.
CREATE UNIQUE INDEX terms_single_current_idx ON academic.terms ((true)) WHERE is_current;

-- Olay türleri bir referans listesidir. is_action_window: bir işlemin (ders seçme, not
-- girişi ...) sadece bu pencere açıkken yapılabildiğini söyler.
CREATE TABLE academic.calendar_event_types (
    code             text     PRIMARY KEY CHECK (code ~ '^[A-Z_]+$'),
    name_tr          text     NOT NULL,
    name_en          text     NOT NULL,
    category         text     NOT NULL CHECK (category IN ('REGISTRATION', 'INSTRUCTION', 'EXAM', 'GRADING', 'ADMISSION', 'OTHER')),
    is_action_window boolean  NOT NULL,
    sort_order       smallint NOT NULL
);

INSERT INTO academic.calendar_event_types (code, name_tr, name_en, category, is_action_window, sort_order) VALUES
    ('PRE_REGISTRATION_OSYS',     'ÖSYS ön kayıt işlemleri',                  'Pre-registration (ÖSYS)',              'ADMISSION',    false, 10),
    ('PRE_REGISTRATION_DGS',      'DGS ön kayıt işlemleri',                   'Pre-registration (DGS)',               'ADMISSION',    false, 11),
    ('PREP_REGISTRATION',         'Hazırlık sınıfı kayıtları',                'Preparatory class registration',       'ADMISSION',    true,  12),
    ('ORIENTATION',               'Uyum haftası',                             'Orientation week',                     'OTHER',        false, 13),
    ('DOUBLE_MAJOR_APPLICATION',  'Çift anadal / yandal başvurusu',           'Double major / minor applications',    'ADMISSION',    true,  14),
    ('DOUBLE_MAJOR_PLACEMENT',    'Çift anadal / yandal öğrenci atama',       'Double major / minor placement',       'ADMISSION',    false, 15),
    ('DOUBLE_MAJOR_CONFIRMATION', 'Çift anadal / yandal kayıt onayı',         'Double major / minor confirmation',    'ADMISSION',    true,  16),
    ('COURSE_REGISTRATION',       'Öğrenci ders seçme işlemleri',             'Course registration',                  'REGISTRATION', true,  20),
    ('ADVISOR_APPROVAL',          'Danışman ders onay işlemleri',             'Advisor approval',                     'REGISTRATION', true,  21),
    ('DEPT_HEAD_APPROVAL',        'Bölüm başkanı onay işlemleri',             'Head of department approval',          'REGISTRATION', true,  22),
    ('ADVISOR_MEETING',           'Danışmanlarla yüz yüze görüşme',           'Advisor meetings',                     'REGISTRATION', false, 23),
    ('ADD_DROP_STUDENT',          'Ekle-bırak (öğrenci)',                     'Add-drop (student)',                   'REGISTRATION', true,  24),
    ('ADD_DROP_ADVISOR',          'Ekle-bırak danışman onayı',                'Add-drop advisor approval',            'REGISTRATION', true,  25),
    ('ADD_DROP_DEPT_HEAD',        'Ekle-bırak bölüm başkanı onayı',           'Add-drop head of department approval', 'REGISTRATION', true,  26),
    ('CLASSES',                   'Derslerin başlama ve bitiş tarihleri',     'Classes',                              'INSTRUCTION',  false, 30),
    ('MIDTERM_EXAMS',             'Ara sınavlar',                             'Midterm exams',                        'EXAM',         false, 31),
    ('MIDTERM_GRADE_ENTRY',       'Ara sınav not girişleri',                  'Midterm grade entry',                  'GRADING',      true,  32),
    ('FINAL_EXAMS',               'Final sınavları',                          'Final exams',                          'EXAM',         false, 33),
    ('FINAL_GRADE_ENTRY',         'Final sınavı not girişleri',               'Final grade entry',                    'GRADING',      true,  34),
    ('MANUAL_LETTER_GRADE',       'Elle harf notu atama',                     'Manual letter grade assignment',       'GRADING',      true,  35),
    ('MAKEUP_EXAMS',              'Bütünleme sınavları',                      'Make-up exams',                        'EXAM',         false, 36),
    ('MAKEUP_GRADE_ENTRY',        'Bütünleme not girişleri',                  'Make-up grade entry',                  'GRADING',      true,  37),
    ('THREE_COURSE_EXAMS',        '3 ders sınavları',                         'Three-course exams',                   'EXAM',         false, 38),
    ('SURVEY_PERIOD',             'Ders değerlendirme anketleri',             'Course evaluation surveys',            'OTHER',        true,  40),
    ('HOLIDAY',                   'Resmi tatil',                              'Public holiday',                       'OTHER',        false, 50);

-- Takvim olayı: bir türün belirli bir kapsamda (üniversite, birim ya da program) açık
-- olduğu zaman aralığı. Dar kapsam geniş kapsamı geçersiz kılar: bir birimin kendi ders
-- seçme penceresi varsa o birim için üniversite penceresi uygulanmaz.
CREATE TABLE academic.calendar_events (
    id              uuid        PRIMARY KEY DEFAULT uuidv7(),
    term_id         uuid        NOT NULL REFERENCES academic.terms (id),
    event_type_code text        NOT NULL REFERENCES academic.calendar_event_types (code),
    title_tr        text,       -- boşsa türün adı kullanılır
    title_en        text,
    period          tstzrange   NOT NULL,
    scope_type      text        NOT NULL CHECK (scope_type IN ('UNIVERSITY', 'FACULTY', 'PROGRAM')),
    scope_id        uuid,       -- birim (org.faculties) ya da program (org.programs); üniversite kapsamında boş
    is_published    boolean     NOT NULL DEFAULT true,
    note            text,
    version         integer     NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(), -- kimin oluşturduğu denetim izinde
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT calendar_events_scope_check CHECK ((scope_type = 'UNIVERSITY') = (scope_id IS NULL)),
    CONSTRAINT calendar_events_period_check CHECK (NOT isempty(period) AND NOT lower_inf(period) AND NOT upper_inf(period)),
    -- Aynı tür aynı kapsamda zaman olarak çakışamaz: "pencere açık mı?" sorusunun tek cevabı olur.
    CONSTRAINT calendar_events_no_overlap EXCLUDE USING gist (
        event_type_code WITH =,
        scope_type WITH =,
        (coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid)) WITH =,
        period WITH &&
    )
);

CREATE INDEX calendar_events_term_idx ON academic.calendar_events (term_id, event_type_code);
CREATE INDEX calendar_events_period_idx ON academic.calendar_events USING gist (period);

-- +goose Down
DROP TABLE academic.calendar_events;
DROP TABLE academic.calendar_event_types;
DROP TABLE academic.terms;
DROP TABLE academic.academic_years;
DROP SCHEMA academic;
