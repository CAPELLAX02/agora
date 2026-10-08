-- +goose Up
-- İki adımlı doğrulama (TOTP, RFC 6238).
--
-- mfa_secret_enc, AES-256-GCM ile şifrelenmiş TOTP sırrıdır. Kurulum başladığında
-- yazılır, kullanıcı ilk kodu doğrulayınca mfa_enabled true olur. mfa_last_used_step,
-- kabul edilmiş son TOTP adımıdır: aynı kod ikinci kez kullanılamaz.
ALTER TABLE iam.users
    ADD COLUMN mfa_enabled        boolean     NOT NULL DEFAULT false,
    ADD COLUMN mfa_secret_enc     bytea,
    ADD COLUMN mfa_last_used_step bigint      NOT NULL DEFAULT 0 CHECK (mfa_last_used_step >= 0),
    ADD COLUMN mfa_enabled_at     timestamptz,
    ADD CONSTRAINT users_mfa_check
        CHECK (NOT mfa_enabled OR (mfa_secret_enc IS NOT NULL AND mfa_enabled_at IS NOT NULL));

-- Telefon kaybolursa kullanılan tek kullanımlık kurtarma kodları. Kodun kendisi
-- saklanmaz, anahtarlı hash'i (HMAC-SHA256) saklanır.
CREATE TABLE iam.mfa_recovery_codes (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id    uuid        NOT NULL REFERENCES iam.users (id),
    code_hash  bytea       NOT NULL CHECK (length(code_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at    timestamptz,

    CONSTRAINT mfa_recovery_codes_user_code_key UNIQUE (user_id, code_hash)
);

-- Parolası doğrulanmış ama ikinci adımı tamamlanmamış giriş. Token'ın kendisi
-- saklanmaz, SHA-256 hash'i saklanır. Ömrü kısadır ve deneme sayısı sınırlıdır.
CREATE TABLE iam.mfa_challenges (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id     uuid        NOT NULL REFERENCES iam.users (id),
    token_hash  bytea       NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    client_type text        NOT NULL CHECK (client_type IN ('WEB', 'MOBILE')),
    attempts    integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz
);

CREATE INDEX mfa_challenges_user_id_idx ON iam.mfa_challenges (user_id);

-- MFA açılınca ya da kapatılınca kullanıcının diğer oturumları kapatılır.
ALTER TABLE iam.sessions DROP CONSTRAINT sessions_revoke_reason_check;
ALTER TABLE iam.sessions ADD CONSTRAINT sessions_revoke_reason_check
    CHECK (revoke_reason IN ('LOGOUT', 'LOGOUT_ALL', 'REUSE_DETECTED', 'PASSWORD_CHANGE', 'PASSWORD_RESET',
                             'MFA_CHANGE', 'ADMIN'));

-- +goose Down
UPDATE iam.sessions SET revoke_reason = 'LOGOUT_ALL' WHERE revoke_reason = 'MFA_CHANGE';
ALTER TABLE iam.sessions DROP CONSTRAINT sessions_revoke_reason_check;
ALTER TABLE iam.sessions ADD CONSTRAINT sessions_revoke_reason_check
    CHECK (revoke_reason IN ('LOGOUT', 'LOGOUT_ALL', 'REUSE_DETECTED', 'PASSWORD_CHANGE', 'PASSWORD_RESET', 'ADMIN'));

DROP TABLE iam.mfa_challenges;
DROP TABLE iam.mfa_recovery_codes;

ALTER TABLE iam.users
    DROP CONSTRAINT users_mfa_check,
    DROP COLUMN mfa_enabled_at,
    DROP COLUMN mfa_last_used_step,
    DROP COLUMN mfa_secret_enc,
    DROP COLUMN mfa_enabled;
