-- +goose Up
CREATE SCHEMA communication;

-- Gönderilecek e-postalar (transactional outbox). İş kodu e-postayı kendi
-- transaction'ında buraya yazar, worker okuyup gönderir: e-posta ancak iş verisi
-- commit edilirse gider ve SMTP kesintisinde kaybolmaz.
CREATE TABLE communication.email_outbox (
    id              uuid        PRIMARY KEY DEFAULT uuidv7(),
    to_address      citext      NOT NULL,
    template_code   text        NOT NULL,
    locale          text        NOT NULL DEFAULT 'tr' CHECK (locale IN ('tr', 'en')),
    -- Şablon verisi. Şifre sıfırlama bağlantısı gibi gizli değerler içerebilir, bu
    -- yüzden e-posta gönderilince (ya da kalıcı olarak başarısız olunca) boşaltılır.
    payload         jsonb       NOT NULL DEFAULT '{}',
    status          text        NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'SENDING', 'SENT', 'FAILED')),
    attempts        integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_until    timestamptz, -- SENDING'de: bu ana kadar gönderilmezse başka worker alır
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz,

    CONSTRAINT email_outbox_sending_lock_check CHECK ((status = 'SENDING') = (locked_until IS NOT NULL))
);

-- Worker sadece bekleyen kayıtlara bakar: gönderilmiş milyonlarca satır index'e girmez.
CREATE INDEX email_outbox_due_idx ON communication.email_outbox (next_attempt_at)
    WHERE status IN ('PENDING', 'SENDING');

-- +goose Down
DROP SCHEMA communication CASCADE;
