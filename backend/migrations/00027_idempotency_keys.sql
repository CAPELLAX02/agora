-- +goose Up

-- İdempotentlik anahtarları: istemcinin aynı isteği (zaman aşımı, çift tıklama, ağ kesintisi
-- yüzünden) tekrar göndermesi işlemi ikinci kez yapmaz, ilk yanıt aynen döner. Anahtar
-- kullanıcıya özeldir; aynı anahtar başka bir istek gövdesiyle kullanılamaz. Kayıtlar bir
-- gün saklanır (worker temizler).
CREATE TABLE platform.idempotency_keys (
    user_id       uuid        NOT NULL REFERENCES iam.users (id) ON DELETE CASCADE,
    key           text        NOT NULL CHECK (char_length(key) BETWEEN 8 AND 255),
    fingerprint   bytea       NOT NULL, -- yöntem, yol ve gövdenin SHA-256 özeti
    status_code   smallint,             -- null: istek hâlâ işleniyor
    response_body bytea,
    content_type  text,
    etag          text,
    location      text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    completed_at  timestamptz,
    PRIMARY KEY (user_id, key)
);

CREATE INDEX idempotency_keys_created_idx ON platform.idempotency_keys (created_at);

-- +goose Down
DROP TABLE platform.idempotency_keys;
