-- +goose Up

-- Mesajlaşma: birebir (DIRECT), grup (GROUP, personel kurar) ve şube grubu (SECTION: üyeleri şubenin
-- öğretim elemanları ve kesin kaydı olan öğrencileri, üyelikten türetilir). Birebir konuşma iki kişi
-- arasında tektir (direct_key); şube grubu şube başına tektir.
CREATE TABLE communication.conversations (
    id                uuid        PRIMARY KEY DEFAULT uuidv7(),
    conversation_type text        NOT NULL CHECK (conversation_type IN ('DIRECT', 'GROUP', 'SECTION')),
    title             text        CHECK (char_length(title) BETWEEN 1 AND 120),
    section_id        uuid        UNIQUE REFERENCES offering.sections (id),
    direct_key        text        UNIQUE, -- birebirde iki kullanıcının sıralı kimlikleri
    created_by        uuid        REFERENCES iam.users (id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    last_message_at   timestamptz,
    CONSTRAINT conversations_kind_check CHECK (
        (conversation_type = 'DIRECT' AND direct_key IS NOT NULL AND section_id IS NULL AND title IS NULL)
        OR (conversation_type = 'GROUP' AND title IS NOT NULL AND section_id IS NULL AND direct_key IS NULL)
        OR (conversation_type = 'SECTION' AND section_id IS NOT NULL AND direct_key IS NULL))
);

-- Üyelik ve kişiye özel durum. Şube grubunda satır, üye konuşmayı ilk açtığında ya da ilk ileti
-- gönderildiğinde oluşur (üyelik yine şubeden denetlenir).
CREATE TABLE communication.conversation_members (
    conversation_id      uuid        NOT NULL REFERENCES communication.conversations (id) ON DELETE CASCADE,
    user_id              uuid        NOT NULL REFERENCES iam.users (id),
    member_role          text        NOT NULL DEFAULT 'MEMBER' CHECK (member_role IN ('OWNER', 'MEMBER')),
    joined_at            timestamptz NOT NULL DEFAULT now(),
    left_at              timestamptz,
    last_read_message_id uuid,       -- bu iletiye kadar (dahil) okundu; iletiler zaman sıralı kimlikli
    is_starred           boolean     NOT NULL DEFAULT false,
    muted_until          timestamptz,
    PRIMARY KEY (conversation_id, user_id)
);
CREATE INDEX conversation_members_user_idx ON communication.conversation_members (user_id) WHERE left_at IS NULL;

-- İleti metni düz yazıdır (en çok 4000 karakter). Kimlik uuidv7: zamana göre sıralı, sayfalama
-- cursor'ı ve okundu imleci.
CREATE TABLE communication.messages (
    id              uuid        PRIMARY KEY DEFAULT uuidv7(),
    conversation_id uuid        NOT NULL REFERENCES communication.conversations (id) ON DELETE CASCADE,
    sender_id       uuid        NOT NULL REFERENCES iam.users (id),
    body            text        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4000),
    reply_to_id     uuid        REFERENCES communication.messages (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    edited_at       timestamptz,
    deleted_at      timestamptz
);
CREATE INDEX messages_conversation_idx ON communication.messages (conversation_id, id DESC);

-- Mesaj bildirimleri varsayılan kapalı: konuşmaların kendi okunmamış sayıları var, her ileti
-- bildirim merkezini doldurmasın. İsteyen tercihlerden uygulama içi ya da e-posta açar.
INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order) VALUES
    ('message.received', 'MESSAGES', 'Yeni ileti', 'New message', '{}', false, 60);

-- +goose Down
DELETE FROM communication.notification_preferences WHERE type_code = 'message.received';
DELETE FROM communication.notifications WHERE type_code = 'message.received';
DELETE FROM communication.notification_types WHERE code = 'message.received';
DROP TABLE communication.messages;
DROP TABLE communication.conversation_members;
DROP TABLE communication.conversations;
