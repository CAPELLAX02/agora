-- +goose Up
CREATE SCHEMA org;

CREATE TABLE org.campuses (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    code       text        NOT NULL UNIQUE,
    name       text        NOT NULL,
    address    text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE org.faculties (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    code       text        NOT NULL UNIQUE,
    name_tr    text        NOT NULL,
    name_en    text        NOT NULL,
    unit_type  text        NOT NULL
        CHECK (unit_type IN ('FACULTY', 'VOCATIONAL_SCHOOL', 'SCHOOL', 'INSTITUTE', 'CONSERVATORY')),
    campus_id  uuid        REFERENCES org.campuses (id),
    is_active  boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX faculties_campus_id_idx ON org.faculties (campus_id);

CREATE TABLE org.departments (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    faculty_id uuid        NOT NULL REFERENCES org.faculties (id),
    code       text        NOT NULL UNIQUE,
    name_tr    text        NOT NULL,
    name_en    text        NOT NULL,
    is_active  boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX departments_faculty_id_idx ON org.departments (faculty_id);

CREATE TABLE org.programs (
    id                  uuid         PRIMARY KEY DEFAULT uuidv7(),
    department_id       uuid         NOT NULL REFERENCES org.departments (id),
    code                text         NOT NULL UNIQUE,
    yoksis_code         text         UNIQUE,
    name_tr             text         NOT NULL,
    name_en             text         NOT NULL,
    degree_level        text         NOT NULL
        CHECK (degree_level IN ('ASSOCIATE', 'BACHELOR', 'MASTER', 'PHD')),
    language            text         NOT NULL
        CHECK (language IN ('TR', 'EN', 'MIXED')),
    education_type      text         NOT NULL
        CHECK (education_type IN ('DAYTIME', 'EVENING', 'DISTANCE')),
    duration_semesters  smallint     NOT NULL CHECK (duration_semesters BETWEEN 2 AND 12),
    max_duration_years  smallint     NOT NULL,
    total_ects_required numeric(5,1) NOT NULL CHECK (total_ects_required > 0),
    has_prep_class      boolean      NOT NULL DEFAULT false,
    is_active           boolean      NOT NULL DEFAULT true,
    created_at          timestamptz  NOT NULL DEFAULT now(),
    updated_at          timestamptz  NOT NULL DEFAULT now(),

    CONSTRAINT programs_max_duration_check
        CHECK (max_duration_years * 2 >= duration_semesters)
);

CREATE INDEX programs_department_id_idx ON org.programs (department_id);

-- +goose Down
DROP TABLE org.programs;
DROP TABLE org.departments;
DROP TABLE org.faculties;
DROP TABLE org.campuses;
DROP SCHEMA org;
