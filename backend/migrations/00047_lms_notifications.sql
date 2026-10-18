-- +goose Up

-- Forum aboneliği: genel forumda açılan yeni konular abonelere bildirilir. Duyurular forumu
-- bütün üyelere bildirildiği için aboneliği yoktur; bir konuya yazanlar o konunun yanıtlarını
-- kendiliğinden alır.
CREATE TABLE lms.forum_subscriptions (
    activity_id uuid        NOT NULL REFERENCES lms.forums (activity_id),
    user_id     uuid        NOT NULL REFERENCES iam.users (id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (activity_id, user_id)
);
CREATE INDEX forum_subscriptions_user_idx ON lms.forum_subscriptions (user_id);

-- Ders alanı bildirimleri.
INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order) VALUES
    ('lms.assignment_created', 'COURSES', 'Yeni ödev',                        'New assignment',                   '{IN_APP}',       false, 40),
    ('lms.grades_released',    'COURSES', 'Ödev puanı yayımlandı',            'Assignment score released',        '{IN_APP}',       false, 41),
    ('forum.announcement',     'COURSES', 'Ders duyurusu',                    'Course announcement',              '{IN_APP,EMAIL}', false, 42),
    ('forum.reply',            'COURSES', 'Yazdığınız konuya yanıt',          'Reply in a thread you posted in',  '{IN_APP}',       false, 43),
    ('forum.thread',           'COURSES', 'Abone olduğunuz forumda yeni konu', 'New thread in a forum you follow', '{IN_APP}',       false, 44);

-- +goose Down
DELETE FROM communication.notification_preferences WHERE type_code LIKE 'lms.%' OR type_code LIKE 'forum.%';
DELETE FROM communication.notifications WHERE type_code LIKE 'lms.%' OR type_code LIKE 'forum.%';
DELETE FROM communication.notification_types WHERE code LIKE 'lms.%' OR code LIKE 'forum.%';
DROP TABLE lms.forum_subscriptions;
