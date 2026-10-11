-- +goose Up

-- Alan olayları için transactional outbox. Bir iş (ör. ders kaydının onayı) olayı kendi
-- transaction'ında buraya yazar; worker okuyup tüketicilere dağıtır. Böylece olay ancak iş
-- verisi commit edilirse yayımlanır ve tüketici (e-posta, ileride LMS üyeliği, analitik)
-- geçici olarak çalışmasa da kaybolmaz: ikili yazma (dual write) sorunu yoktur.
CREATE SCHEMA platform;

CREATE TABLE platform.outbox_events (
    id              bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_type      text        NOT NULL CHECK (event_type ~ '^[a-z_]+\.[a-z_]+$'), -- ör. registration.approved
    aggregate_type  text        NOT NULL,
    aggregate_id    uuid        NOT NULL,
    payload         jsonb       NOT NULL DEFAULT '{}',
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    status          text        NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PROCESSING', 'DONE', 'FAILED')),
    attempts        integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_until    timestamptz, -- PROCESSING'de: bu ana kadar bitmezse başka worker alır
    last_error      text,
    processed_at    timestamptz,

    CONSTRAINT outbox_events_lock_check CHECK ((status = 'PROCESSING') = (locked_until IS NOT NULL))
);

-- Worker sadece bekleyen olaylara bakar.
CREATE INDEX outbox_events_due_idx ON platform.outbox_events (next_attempt_at, id)
    WHERE status IN ('PENDING', 'PROCESSING');
CREATE INDEX outbox_events_aggregate_idx ON platform.outbox_events (aggregate_type, aggregate_id);

-- Tüketicilerin işlediği olaylar. Tüketici kendi yazmalarını bu satırla aynı transaction'da
-- yapar: olay yeniden dağıtılsa da (worker çöktü, bir başka tüketici hata verdi) aynı
-- tüketici aynı olayı ikinci kez işlemez.
CREATE TABLE platform.outbox_consumptions (
    event_id    bigint      NOT NULL REFERENCES platform.outbox_events (id) ON DELETE CASCADE,
    consumer    text        NOT NULL,
    consumed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, consumer)
);

-- Dönem geçişinde yarıyıl ilerletmesi: hangi dönem için yapıldığı. Bir dönem aktif yapılınca
-- (güz ya da bahar) aktif öğrencilerin yarıyılı bir kez ilerler; aynı dönem tekrar aktif
-- yapılsa ya da daha eski bir döneme dönülse ilerlemez.
CREATE TABLE enrollment.semester_advances (
    term_id     uuid        PRIMARY KEY REFERENCES academic.terms (id),
    advanced_at timestamptz NOT NULL DEFAULT now(),
    advanced    integer     NOT NULL CHECK (advanced >= 0) -- ilerletilen kayıt sayısı
);

-- +goose Down
DROP TABLE enrollment.semester_advances;
DROP SCHEMA platform CASCADE;
