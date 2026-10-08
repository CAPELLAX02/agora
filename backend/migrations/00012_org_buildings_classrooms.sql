-- +goose Up
-- Binalar bir yerleşkededir ve isteğe bağlı olarak bir akademik birime aittir.
-- Birime ait binaların dersliklerini o birimin yetkilileri yönetebilir.
CREATE TABLE org.buildings (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    campus_id  uuid        NOT NULL REFERENCES org.campuses (id),
    faculty_id uuid        REFERENCES org.faculties (id),
    code       text        NOT NULL CHECK (code ~ '^[A-Z0-9-]{1,20}$'),
    name       text        NOT NULL CHECK (btrim(name) <> ''),
    is_active  boolean     NOT NULL DEFAULT true,
    version    integer     NOT NULL DEFAULT 1, -- iyimser kilit: ETag / If-Match
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buildings_campus_code_key UNIQUE (campus_id, code)
);

CREATE TABLE org.classrooms (
    id            uuid        PRIMARY KEY DEFAULT uuidv7(),
    building_id   uuid        NOT NULL REFERENCES org.buildings (id),
    code          text        NOT NULL CHECK (code ~ '^[A-Z0-9-]{1,20}$'),
    name          text        NOT NULL CHECK (btrim(name) <> ''),
    capacity      integer     NOT NULL CHECK (capacity BETWEEN 0 AND 2000),
    -- Sınavda öğrenciler arasında boşluk bırakıldığı için sınav kapasitesi daha düşüktür.
    exam_capacity integer     NOT NULL CHECK (exam_capacity >= 0),
    room_type     text        NOT NULL CHECK (room_type IN ('LECTURE', 'LAB', 'AMPHI', 'OFFICE', 'ONLINE')),
    features      text[]      NOT NULL DEFAULT '{}'
        CHECK (features <@ ARRAY['PROJECTOR', 'SMART_BOARD', 'COMPUTERS', 'ACCESSIBLE',
                                 'AIR_CONDITIONING', 'SOUND_SYSTEM', 'RECORDING']::text[]),
    is_active     boolean     NOT NULL DEFAULT true,
    version       integer     NOT NULL DEFAULT 1,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT classrooms_building_code_key UNIQUE (building_id, code),
    CONSTRAINT classrooms_exam_within_capacity_check CHECK (exam_capacity <= capacity)
);

CREATE INDEX buildings_faculty_idx ON org.buildings (faculty_id);
-- Ders programında "en az N kişilik boş derslik" araması kapasiteye göre yapılır.
CREATE INDEX classrooms_capacity_idx ON org.classrooms (capacity) WHERE is_active;

-- +goose Down
DROP TABLE org.classrooms;
DROP TABLE org.buildings;
