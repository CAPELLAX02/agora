-- +goose Up

-- Bildirim türleri. Her tür bir kategoriye girer, varsayılan kanalları vardır; zorunlu türler
-- (güvenlik) tercihle kapatılamaz. Başlık ve metin şablonları kodda (internal/notify), tür
-- başına Türkçe ve İngilizce; kayıtta sadece şablon verisi tutulur.
CREATE TABLE communication.notification_types (
    code             text    PRIMARY KEY CHECK (code ~ '^[a-z_]+\.[a-z_]+$'),
    category         text    NOT NULL
        CHECK (category IN ('SECURITY', 'REGISTRATION', 'GRADES', 'COURSES', 'ANNOUNCEMENTS', 'MESSAGES', 'REMINDERS')),
    name_tr          text    NOT NULL,
    name_en          text    NOT NULL,
    default_channels text[]  NOT NULL DEFAULT '{IN_APP}' CHECK (default_channels <@ ARRAY['IN_APP', 'EMAIL']),
    is_mandatory     boolean NOT NULL DEFAULT false,
    sort_order       integer NOT NULL
);

INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order) VALUES
    ('security.password_changed', 'SECURITY', 'Parolanız değiştirildi',            'Your password was changed',            '{IN_APP,EMAIL}', true, 10),
    ('security.mfa_enabled',      'SECURITY', 'İki adımlı doğrulama açıldı',       'Two-step verification turned on',      '{IN_APP,EMAIL}', true, 11),
    ('security.mfa_disabled',     'SECURITY', 'İki adımlı doğrulama kapatıldı',    'Two-step verification turned off',     '{IN_APP,EMAIL}', true, 12);

-- Kullanıcının bildirimleri. Yüksek hacimli ve kısa ömürlü olduğu için aylık partition'lara
-- bölünür; 180 günden eski partition'lar worker'da düşürülür. Kimlik bütün partition'larda
-- artandır: liste cursor'ı ve canlı akışın Last-Event-ID'si odur.
CREATE TABLE communication.notifications (
    id          bigint      GENERATED ALWAYS AS IDENTITY,
    created_at  timestamptz NOT NULL DEFAULT now(),
    user_id     uuid        NOT NULL REFERENCES iam.users (id),
    type_code   text        NOT NULL REFERENCES communication.notification_types (code),
    payload     jsonb       NOT NULL DEFAULT '{}',
    link_path   text        CHECK (link_path ~ '^/'), -- uygulama içi adres
    read_at     timestamptz,
    archived_at timestamptz,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE communication.notifications_default PARTITION OF communication.notifications DEFAULT;

CREATE INDEX notifications_user_idx ON communication.notifications (user_id, id DESC);
-- Okunmamış sayısı: sadece okunmamış ve arşivlenmemiş satırlar index'e girer.
CREATE INDEX notifications_unread_idx ON communication.notifications (user_id)
    WHERE read_at IS NULL AND archived_at IS NULL;

-- ensure_notification_partitions, içinde bulunulan ay ve sonraki months_ahead ay için
-- partition açar; drop_notification_partitions, bütünüyle keep'ten eski partition'ları
-- düşürür (varsayılan partition'daki eski satırlar silinir). İkisi de tekrar çalıştırılabilir.
-- +goose StatementBegin
CREATE FUNCTION communication.ensure_notification_partitions(months_ahead integer DEFAULT 3) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    month date;
    i     integer;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('communication.ensure_notification_partitions'));
    FOR i IN 0..months_ahead LOOP
        month := (date_trunc('month', now() AT TIME ZONE 'UTC') + make_interval(months => i))::date;
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS communication.%I PARTITION OF communication.notifications FOR VALUES FROM (%L) TO (%L)',
            'notifications_' || to_char(month, 'YYYY_MM'),
            month::timestamp AT TIME ZONE 'UTC',
            (month + interval '1 month')::timestamp AT TIME ZONE 'UTC'
        );
    END LOOP;
END
$$;

CREATE FUNCTION communication.drop_notification_partitions(keep interval DEFAULT '180 days') RETURNS integer
LANGUAGE plpgsql AS $$
DECLARE
    -- Bu aydan önceki aylar bütünüyle saklama süresinin dışında kalır.
    boundary date := date_trunc('month', (now() - keep) AT TIME ZONE 'UTC')::date;
    name     text;
    dropped  integer := 0;
    i        integer;
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('communication.ensure_notification_partitions'));
    FOR i IN 1..36 LOOP
        name := 'notifications_' || to_char(boundary - make_interval(months => i), 'YYYY_MM');
        IF to_regclass('communication.' || name) IS NOT NULL THEN
            EXECUTE format('DROP TABLE communication.%I', name);
            dropped := dropped + 1;
        END IF;
    END LOOP;
    DELETE FROM communication.notifications_default WHERE created_at < now() - keep;
    RETURN dropped;
END
$$;
-- +goose StatementEnd

SELECT communication.ensure_notification_partitions(3);

-- +goose Down
DROP FUNCTION communication.drop_notification_partitions(interval);
DROP FUNCTION communication.ensure_notification_partitions(integer);
DROP TABLE communication.notifications;
DROP TABLE communication.notification_types;
