-- +goose Up
CREATE SCHEMA iam;

-- Oturum açabilen hesap. Kişi bilgileri people.persons'ta durur.
CREATE TABLE iam.users (
    id                   uuid        PRIMARY KEY DEFAULT uuidv7(),
    person_id            uuid        NOT NULL UNIQUE REFERENCES people.persons (id),
    username             citext      NOT NULL UNIQUE, -- öğrenci/personel numarası
    email                citext      NOT NULL UNIQUE,
    status               text        NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED', 'DISABLED')),
    password_hash        text        NOT NULL,
    password_changed_at  timestamptz NOT NULL DEFAULT now(),
    must_change_password boolean     NOT NULL DEFAULT false,
    failed_login_count   integer     NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
    locked_until         timestamptz,
    last_login_at        timestamptz,
    perm_version         integer     NOT NULL DEFAULT 1, -- rol değişince artar, yetki önbelleğini geçersiz kılar
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- Her giriş bir oturumdur. Refresh token'lar oturuma bağlıdır (token ailesi).
CREATE TABLE iam.sessions (
    id                  uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id             uuid        NOT NULL REFERENCES iam.users (id),
    client_type         text        NOT NULL CHECK (client_type IN ('WEB', 'MOBILE')),
    ip                  inet,
    user_agent          text,
    amr                 text[]      NOT NULL DEFAULT '{pwd}',
    created_at          timestamptz NOT NULL DEFAULT now(),
    last_seen_at        timestamptz NOT NULL DEFAULT now(),
    absolute_expires_at timestamptz NOT NULL,
    revoked_at          timestamptz,
    revoke_reason       text
        CHECK (revoke_reason IN ('LOGOUT', 'LOGOUT_ALL', 'REUSE_DETECTED', 'PASSWORD_RESET', 'ADMIN')),

    CONSTRAINT sessions_revocation_check CHECK ((revoked_at IS NULL) = (revoke_reason IS NULL))
);

CREATE INDEX sessions_active_user_id_idx ON iam.sessions (user_id) WHERE revoked_at IS NULL;

-- Refresh token'ın kendisi asla saklanmaz, sadece SHA-256 hash'i.
CREATE TABLE iam.refresh_tokens (
    id             uuid        PRIMARY KEY DEFAULT uuidv7(),
    session_id     uuid        NOT NULL REFERENCES iam.sessions (id) ON DELETE CASCADE,
    token_hash     bytea       NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    parent_id      uuid        REFERENCES iam.refresh_tokens (id),
    issued_at      timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL,
    used_at        timestamptz,
    replaced_by_id uuid        REFERENCES iam.refresh_tokens (id)
);

CREATE INDEX refresh_tokens_session_id_idx ON iam.refresh_tokens (session_id);

CREATE TABLE iam.roles (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    code        text NOT NULL UNIQUE,
    name_tr     text NOT NULL,
    name_en     text NOT NULL,
    scope_type  text NOT NULL
        CHECK (scope_type IN ('UNIVERSITY', 'FACULTY', 'DEPARTMENT', 'PROGRAM', 'NONE')),
    description text
);

CREATE TABLE iam.permissions (
    id           uuid    PRIMARY KEY DEFAULT uuidv7(),
    code         text    NOT NULL UNIQUE CHECK (code ~ '^[a-z_]+:[a-z_]+$'), -- kaynak:eylem
    description  text    NOT NULL,
    requires_mfa boolean NOT NULL DEFAULT false
);

CREATE TABLE iam.role_permissions (
    role_id       uuid NOT NULL REFERENCES iam.roles (id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES iam.permissions (id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- Kullanıcıya belirli bir kapsamda (ör. bir bölümde) ve belirli bir süre için verilen rol.
CREATE TABLE iam.role_assignments (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id     uuid        NOT NULL REFERENCES iam.users (id),
    role_id     uuid        NOT NULL REFERENCES iam.roles (id),
    scope_type  text        NOT NULL
        CHECK (scope_type IN ('UNIVERSITY', 'FACULTY', 'DEPARTMENT', 'PROGRAM', 'NONE')),
    scope_id    uuid,
    valid_from  timestamptz NOT NULL DEFAULT now(),
    valid_until timestamptz,
    assigned_by uuid        REFERENCES iam.users (id),
    reason      text,
    created_at  timestamptz NOT NULL DEFAULT now(),

    -- Üniversite geneli ve kapsamsız rollerde scope_id boş, diğerlerinde dolu olmalı.
    CONSTRAINT role_assignments_scope_check
        CHECK ((scope_type IN ('UNIVERSITY', 'NONE')) = (scope_id IS NULL)),
    CONSTRAINT role_assignments_period_check
        CHECK (valid_until IS NULL OR valid_until > valid_from),
    -- Aynı kullanıcıya aynı rol aynı kapsamda zaman olarak çakışan iki kez verilemez.
    CONSTRAINT role_assignments_no_overlap EXCLUDE USING gist (
        user_id WITH =,
        role_id WITH =,
        (coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid)) WITH =,
        tstzrange(valid_from, valid_until) WITH &&
    )
);

CREATE INDEX role_assignments_user_id_idx ON iam.role_assignments (user_id);

-- +goose Down
DROP TABLE iam.role_assignments;
DROP TABLE iam.role_permissions;
DROP TABLE iam.permissions;
DROP TABLE iam.roles;
DROP TABLE iam.refresh_tokens;
DROP TABLE iam.sessions;
DROP TABLE iam.users;
DROP SCHEMA iam;