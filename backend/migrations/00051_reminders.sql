-- +goose Up

-- Gönderilmiş hatırlatmalar: aynı hatırlatma (tür, kayıt, kişi) bir kez gider. Worker hatırlatmayı
-- bu tabloya ekleyebildiği kişilere bildirir; tekrar çalıştırmak ya da iki worker zararsızdır.
CREATE TABLE communication.reminders_sent (
    kind    text        NOT NULL CHECK (kind IN ('window_opening', 'window_closing', 'assignment_due')),
    ref_id  uuid        NOT NULL, -- takvim olayı ya da ödev etkinliği
    user_id uuid        NOT NULL REFERENCES iam.users (id),
    sent_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, ref_id, user_id)
);

INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order) VALUES
    ('reminder.window_opening', 'REMINDERS', 'İşlem penceresi açılıyor',  'Window opening soon',     '{IN_APP,EMAIL}', false, 70),
    ('reminder.window_closing', 'REMINDERS', 'İşlem penceresi kapanıyor', 'Window closing soon',     '{IN_APP,EMAIL}', false, 71),
    ('reminder.assignment_due', 'REMINDERS', 'Ödevin son tarihi yaklaşıyor', 'Assignment due soon', '{IN_APP,EMAIL}', false, 72);

-- +goose Down
DELETE FROM communication.notification_preferences WHERE type_code LIKE 'reminder.%';
DELETE FROM communication.notifications WHERE type_code LIKE 'reminder.%';
DELETE FROM communication.notification_types WHERE code LIKE 'reminder.%';
DROP TABLE communication.reminders_sent;
