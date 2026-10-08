-- +goose Up
CREATE SCHEMA audit;

-- Kim, neyi, ne zaman değiştirdi? İş verisi üzerindeki yönetimsel değişikliklerin
-- izi (rol atama, hesap askıya alma, not düzeltme...). Satırlar güncellenmez ve
-- silinmez. Yüksek hacimli olduğu için aylık partition'lara bölünür.
CREATE TABLE audit.audit_log (
    id                   bigint      GENERATED ALWAYS AS IDENTITY,
    occurred_at          timestamptz NOT NULL DEFAULT now(),
    actor_user_id        uuid,       -- sistem işlemlerinde (worker, seed) boş
    impersonator_user_id uuid,       -- "X, Y adına" görüntüleme (P2)
    action               text        NOT NULL CHECK (action ~ '^[a-z_]+\.[a-z_]+$'), -- ör. role.assign
    entity_type          text        NOT NULL,
    entity_id            text,
    before               jsonb,
    after                jsonb,
    ip                   inet,
    user_agent           text,
    request_id           text,
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

-- Kimlik doğrulamayla ilgili olaylar: başarılı ve başarısız girişler, kilitlenme,
-- token yeniden kullanımı, parola değişiklikleri. Kullanıcının "son girişlerim"
-- ekranı ve güvenlik incelemesi buradan beslenir.
CREATE TABLE audit.security_events (
    id                 bigint      GENERATED ALWAYS AS IDENTITY,
    occurred_at        timestamptz NOT NULL DEFAULT now(),
    event_type         text        NOT NULL CHECK (event_type ~ '^[A-Z_]+$'),
    user_id            uuid,       -- kullanıcı bulunamadıysa boş
    username_attempted text,
    ip                 inet,
    user_agent         text,
    request_id         text,
    details            jsonb       NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

-- Varsayılan partition'lar, aylık partition'ı henüz açılmamış bir zamana yazılan
-- satırın kaybolmasını önler. Normal işleyişte boş kalırlar.
CREATE TABLE audit.audit_log_default PARTITION OF audit.audit_log DEFAULT;
CREATE TABLE audit.security_events_default PARTITION OF audit.security_events DEFAULT;

-- Partition'lı tablodaki index'ler bütün partition'lara otomatik uygulanır.
CREATE INDEX audit_log_entity_idx ON audit.audit_log (entity_type, entity_id, occurred_at DESC);
CREATE INDEX audit_log_actor_idx ON audit.audit_log (actor_user_id, occurred_at DESC);
CREATE INDEX audit_log_occurred_idx ON audit.audit_log (occurred_at DESC, id DESC);
CREATE INDEX security_events_user_idx ON audit.security_events (user_id, occurred_at DESC);
CREATE INDEX security_events_type_idx ON audit.security_events (event_type, occurred_at DESC);
CREATE INDEX security_events_occurred_idx ON audit.security_events (occurred_at DESC, id DESC);

-- ensure_partitions, içinde bulunulan ay ve sonraki months_ahead ay için partition
-- açar. Tekrar çalıştırılabilir. API açılışta, worker günlük olarak çağırır. Aynı
-- anda çalışan iki süreç birbirini advisory lock ile bekler.
-- +goose StatementBegin
CREATE FUNCTION audit.ensure_partitions(months_ahead integer DEFAULT 3) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    parent text;
    month  date;
    i      integer;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('audit.ensure_partitions'));

    FOREACH parent IN ARRAY ARRAY['audit_log', 'security_events'] LOOP
        FOR i IN 0..months_ahead LOOP
            month := (date_trunc('month', now() AT TIME ZONE 'UTC') + make_interval(months => i))::date;
            EXECUTE format(
                'CREATE TABLE IF NOT EXISTS audit.%I PARTITION OF audit.%I FOR VALUES FROM (%L) TO (%L)',
                parent || '_' || to_char(month, 'YYYY_MM'),
                parent,
                month::timestamp AT TIME ZONE 'UTC',
                (month + interval '1 month')::timestamp AT TIME ZONE 'UTC'
            );
        END LOOP;
    END LOOP;
END
$$;
-- +goose StatementEnd

SELECT audit.ensure_partitions(3);

-- +goose Down
DROP SCHEMA audit CASCADE;
