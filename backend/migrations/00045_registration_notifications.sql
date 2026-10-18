-- +goose Up

-- Ders kaydı bildirimleri: danışmana onay bekleyen kayıt, öğrenciye danışman kararı; ekle-bırakta
-- onaylayana bekleyen talep, öğrenciye sonuç.
INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order) VALUES
    ('registration.submitted',         'REGISTRATION', 'Onayınızı bekleyen ders kaydı',     'Course registration awaiting your approval', '{IN_APP}',       false, 20),
    ('registration.decision',          'REGISTRATION', 'Ders kaydı kararı',                 'Course registration decision',               '{IN_APP,EMAIL}', false, 21),
    ('registration.add_drop_pending',  'REGISTRATION', 'Onayınızı bekleyen ekle-bırak',     'Add/drop request awaiting your approval',    '{IN_APP}',       false, 22),
    ('registration.add_drop_decision', 'REGISTRATION', 'Ekle-bırak talebinin sonucu',       'Add/drop request result',                    '{IN_APP,EMAIL}', false, 23);

-- +goose Down
DELETE FROM communication.notification_preferences WHERE type_code LIKE 'registration.%';
DELETE FROM communication.notifications WHERE type_code LIKE 'registration.%';
DELETE FROM communication.notification_types WHERE code LIKE 'registration.%';
