-- +goose Up

-- Not bildirimleri: öğrenciye yayımlanan bileşen notu ve ilan edilen harf notu (bütünleme ve
-- düzeltme dahil); not düzeltmede onaylayana bekleyen talep, talep edene karar.
INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order) VALUES
    ('grades.component_published', 'GRADES', 'Bileşen notu yayımlandı',          'Assessment score published',          '{IN_APP}',       false, 30),
    ('grades.letters_published',   'GRADES', 'Harf notu ilan edildi',            'Letter grade announced',              '{IN_APP,EMAIL}', false, 31),
    ('grades.change_requested',    'GRADES', 'Onayınızı bekleyen not düzeltme',  'Grade change awaiting your approval', '{IN_APP}',       false, 32),
    ('grades.change_decided',      'GRADES', 'Not düzeltme talebinin sonucu',    'Grade change request result',         '{IN_APP}',       false, 33);

-- +goose Down
DELETE FROM communication.notification_preferences WHERE type_code LIKE 'grades.%';
DELETE FROM communication.notifications WHERE type_code LIKE 'grades.%';
DELETE FROM communication.notification_types WHERE code LIKE 'grades.%';
