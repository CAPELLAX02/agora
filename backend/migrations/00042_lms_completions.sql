-- +goose Up

-- Etkinlik tamamlama: öğrenci başına, kendiliğinden. Sayfa, klasör, forum ve bağlantı
-- görüntülenince; dosya indirilince; ödev gönderilince tamamlanır. Etiket ilerlemeye girmez.
-- Ders alanının ilerlemesi: görünür etkinliklerden tamamlananların oranı.
CREATE TABLE lms.activity_completions (
    activity_id  uuid        NOT NULL REFERENCES lms.activities (id),
    user_id      uuid        NOT NULL REFERENCES iam.users (id),
    completed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (activity_id, user_id)
);
CREATE INDEX activity_completions_user_idx ON lms.activity_completions (user_id);

-- +goose Down
DROP TABLE lms.activity_completions;
