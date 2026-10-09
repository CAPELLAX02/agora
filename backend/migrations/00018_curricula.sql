-- +goose Up
-- Versiyonlu müfredat (ders planı) ve öğrenci program kaydına bağlantısı.
--
-- Bir programın birden fazla müfredat sürümü olur; öğrenci giriş yılına göre birine
-- bağlanır (2018-2022 girişliler 2022 sürümü, 2023-2025 girişliler 2023 sürümü ...).
-- Sürüm taslak olarak hazırlanır, yürürlüğe girince yarıyıl yerleşimi donar; bütün
-- öğrencileri mezun olunca ya da başka sürüme aktarılınca arşivlenir.
CREATE TABLE curriculum.curricula (
    id                  uuid         PRIMARY KEY DEFAULT uuidv7(),
    program_id          uuid         NOT NULL REFERENCES org.programs (id),
    name_tr             text         NOT NULL CHECK (btrim(name_tr) <> ''),
    name_en             text         NOT NULL CHECK (btrim(name_en) <> ''),
    effective_from_year smallint     NOT NULL CHECK (effective_from_year BETWEEN 1946 AND 2100), -- bu yıl ve sonrası girişliler
    effective_to_year   smallint     CHECK (effective_to_year BETWEEN 1946 AND 2100),           -- boşsa yeni girişlere açık
    total_ects_required numeric(5,1) NOT NULL CHECK (total_ects_required > 0),
    status              text         NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'ACTIVE', 'ARCHIVED')),
    decision_ref        text,        -- senato kararı vb.
    copied_from_id      uuid         REFERENCES curriculum.curricula (id) ON DELETE SET NULL,
    activated_at        timestamptz,
    archived_at         timestamptz,
    version             integer      NOT NULL DEFAULT 1,
    created_at          timestamptz  NOT NULL DEFAULT now(),
    updated_at          timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT curricula_years_check CHECK (effective_to_year IS NULL OR effective_to_year >= effective_from_year),
    CONSTRAINT curricula_activation_check CHECK ((status = 'DRAFT') = (activated_at IS NULL)),
    CONSTRAINT curricula_archive_check CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL)),
    -- Bir programın yürürlükteki sürümleri giriş yılı olarak çakışamaz: her giriş yılının
    -- tek bir müfredatı olur.
    CONSTRAINT curricula_no_overlap EXCLUDE USING gist (
        program_id WITH =,
        int4range(effective_from_year, effective_to_year, '[]') WITH &&
    ) WHERE (status = 'ACTIVE')
);

CREATE INDEX curricula_program_idx ON curriculum.curricula (program_id, effective_from_year DESC);

-- Müfredat satırı: bir yarıyıla yerleşmiş bir ders ya da seçmeli ders yuvası. Yuva,
-- havuzdan (seçmeli grup) seçilecek bir dersin yeridir; kendi saati ve AKTS'si vardır
-- (ör. 5. yarıyılda COMTE03 3-0-3 / 6 AKTS).
CREATE TABLE curriculum.curriculum_items (
    id                   uuid         PRIMARY KEY DEFAULT uuidv7(),
    curriculum_id        uuid         NOT NULL REFERENCES curriculum.curricula (id) ON DELETE CASCADE,
    semester_no          smallint     NOT NULL CHECK (semester_no BETWEEN 1 AND 12),
    item_type            text         NOT NULL CHECK (item_type IN ('COURSE', 'ELECTIVE_SLOT')),
    course_id            uuid         REFERENCES curriculum.courses (id),
    elective_group_id    uuid         REFERENCES curriculum.elective_groups (id),
    slot_theory_hours    smallint     CHECK (slot_theory_hours BETWEEN 0 AND 40),
    slot_practice_hours  smallint     CHECK (slot_practice_hours BETWEEN 0 AND 40),
    slot_national_credit numeric(4,1) CHECK (slot_national_credit >= 0),
    slot_ects            numeric(4,1) CHECK (slot_ects >= 0),
    is_compulsory        boolean      NOT NULL,
    position             smallint     NOT NULL DEFAULT 0,

    CONSTRAINT curriculum_items_course_check CHECK (
        (item_type = 'COURSE' AND course_id IS NOT NULL AND elective_group_id IS NULL
            AND slot_theory_hours IS NULL AND slot_practice_hours IS NULL AND slot_national_credit IS NULL AND slot_ects IS NULL)
        OR
        (item_type = 'ELECTIVE_SLOT' AND course_id IS NULL AND elective_group_id IS NOT NULL
            AND slot_theory_hours IS NOT NULL AND slot_practice_hours IS NOT NULL AND slot_national_credit IS NOT NULL AND slot_ects IS NOT NULL
            AND NOT is_compulsory)
    ),
    -- Bir ders bir müfredatta bir kez geçer (yuvalar tekrarlanabilir: iki ayrı teknik seçmeli).
    CONSTRAINT curriculum_items_course_key UNIQUE (curriculum_id, course_id)
);

CREATE INDEX curriculum_items_curriculum_idx ON curriculum.curriculum_items (curriculum_id, semester_no, position);
CREATE INDEX curriculum_items_course_idx ON curriculum.curriculum_items (course_id) WHERE course_id IS NOT NULL;
CREATE INDEX curriculum_items_group_idx ON curriculum.curriculum_items (elective_group_id) WHERE elective_group_id IS NOT NULL;

-- Öğrencinin program kaydı izlediği müfredat sürümüne bağlanır. Sürüm yürürlüğe girince
-- giriş yılı aralığındaki bağlantısız kayıtlar otomatik bağlanır; intibakla başka sürüme
-- aktarma öğrenci işlerinin kararıdır.
ALTER TABLE enrollment.student_programs ADD COLUMN curriculum_id uuid REFERENCES curriculum.curricula (id);
CREATE INDEX student_programs_curriculum_idx ON enrollment.student_programs (curriculum_id);

-- +goose Down
DROP INDEX enrollment.student_programs_curriculum_idx;
ALTER TABLE enrollment.student_programs DROP COLUMN curriculum_id;
DROP TABLE curriculum.curriculum_items;
DROP TABLE curriculum.curricula;
