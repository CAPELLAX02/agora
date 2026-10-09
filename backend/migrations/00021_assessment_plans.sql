-- +goose Up
-- Değerlendirme planı: şubenin ara sınav, ödev, proje ... bileşenleri ve ağırlıkları.
-- Notlar (Faz 5) bu bileşenlere girilir; plan kilitlendikten sonra değişmez.
CREATE SCHEMA grading;

CREATE TABLE grading.assessment_types (
    code       text     PRIMARY KEY CHECK (code ~ '^[A-Z_]+$'),
    name_tr    text     NOT NULL,
    name_en    text     NOT NULL,
    -- IN_TERM bileşenlerin ve finalin ağırlıkları toplamı 100'dür. Bütünleme finalin
    -- yerine geçer ve ağırlığı finalinkiyle aynıdır.
    category   text     NOT NULL CHECK (category IN ('IN_TERM', 'FINAL', 'MAKEUP')),
    sort_order smallint NOT NULL
);

INSERT INTO grading.assessment_types (code, name_tr, name_en, category, sort_order) VALUES
    ('MIDTERM',       'Ara sınav',     'Midterm exam',  'IN_TERM', 1),
    ('QUIZ',          'Kısa sınav',    'Quiz',          'IN_TERM', 2),
    ('HOMEWORK',      'Ödev',          'Homework',      'IN_TERM', 3),
    ('PROJECT',       'Proje',         'Project',       'IN_TERM', 4),
    ('LAB',           'Laboratuvar',   'Laboratory',    'IN_TERM', 5),
    ('PRESENTATION',  'Sunum',         'Presentation',  'IN_TERM', 6),
    ('PARTICIPATION', 'Derse katılım', 'Participation', 'IN_TERM', 7),
    ('FINAL',         'Final sınavı',  'Final exam',    'FINAL',   10),
    ('MAKEUP',        'Bütünleme',     'Make-up exam',  'MAKEUP',  11);

-- Planın sürümü ve kilidi şubede tutulur: plan şubenin bir parçasıdır, iyimser kilit
-- (If-Match) ve kilit bilgisi tek satırda olur.
ALTER TABLE offering.sections
    ADD COLUMN assessment_plan_version   integer     NOT NULL DEFAULT 1,
    ADD COLUMN assessment_plan_locked_at timestamptz,
    ADD COLUMN assessment_plan_locked_by uuid; -- kilitleyen kullanıcı (denetim izinde de var)

CREATE TABLE grading.assessment_components (
    id           uuid         PRIMARY KEY DEFAULT uuidv7(),
    section_id   uuid         NOT NULL REFERENCES offering.sections (id) ON DELETE CASCADE,
    type_code    text         NOT NULL REFERENCES grading.assessment_types (code),
    sequence_no  smallint     NOT NULL DEFAULT 1 CHECK (sequence_no BETWEEN 1 AND 20), -- Ara sınav 1, 2 ...
    name_tr      text,        -- boşsa türün adı ve sıra numarası gösterilir
    name_en      text,
    weight       numeric(5,2) NOT NULL CHECK (weight > 0 AND weight <= 100),
    scheduled_on date,
    position     smallint     NOT NULL DEFAULT 0,

    CONSTRAINT assessment_components_type_sequence_key UNIQUE (section_id, type_code, sequence_no)
);

-- Her planda en fazla bir final ve bir bütünleme.
CREATE UNIQUE INDEX assessment_components_single_final_idx ON grading.assessment_components (section_id) WHERE type_code = 'FINAL';
CREATE UNIQUE INDEX assessment_components_single_makeup_idx ON grading.assessment_components (section_id) WHERE type_code = 'MAKEUP';

-- +goose Down
DROP TABLE grading.assessment_components;
ALTER TABLE offering.sections
    DROP COLUMN assessment_plan_locked_by,
    DROP COLUMN assessment_plan_locked_at,
    DROP COLUMN assessment_plan_version;
DROP TABLE grading.assessment_types;
DROP SCHEMA grading;
