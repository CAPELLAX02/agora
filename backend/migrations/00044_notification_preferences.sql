-- +goose Up

-- Bildirim tercihleri (tür × kanal). Sadece kullanıcının değiştirdiği değerler tutulur; satırı
-- olmayan kanal türün varsayılanıdır. Zorunlu türlerde tercih yok sayılır (kapatılamaz).
-- Mobil push kanalı mobil fazında eklenecek.
CREATE TABLE communication.notification_preferences (
    user_id    uuid        NOT NULL REFERENCES iam.users (id),
    type_code  text        NOT NULL REFERENCES communication.notification_types (code),
    channel    text        NOT NULL CHECK (channel IN ('IN_APP', 'EMAIL')),
    enabled    boolean     NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, type_code, channel)
);

-- +goose Down
DROP TABLE communication.notification_preferences;
