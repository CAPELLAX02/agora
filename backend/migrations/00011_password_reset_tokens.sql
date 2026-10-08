-- +goose Up
-- Parola sıfırlama ve hesap aktivasyonu bağlantıları. Token'ın kendisi değil SHA-256
-- hash'i saklanır: veritabanı sızsa bile bağlantılar kullanılamaz. Tek kullanımlıktır,
-- kısa ömürlüdür ve kullanıcının yeni bir isteği öncekileri geçersiz kılar.
CREATE TABLE iam.password_reset_tokens (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    user_id      uuid        NOT NULL REFERENCES iam.users (id),
    purpose      text        NOT NULL CHECK (purpose IN ('RESET', 'ACTIVATION')),
    token_hash   bytea       NOT NULL UNIQUE,
    expires_at   timestamptz NOT NULL,
    used_at      timestamptz, -- kullanıldığında ya da yeni bir istekle geçersiz kılındığında
    requested_ip inet,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_tokens_user_idx ON iam.password_reset_tokens (user_id, created_at DESC);

-- +goose Down
DROP TABLE iam.password_reset_tokens;
