-- +goose Up

-- İleti ekleri (dosya servisi). Sadece ekten oluşan ileti olabilir: metin boş olabilir, en az
-- metin ya da ek olması uygulamada denetlenir.
ALTER TABLE communication.messages DROP CONSTRAINT messages_body_check;
ALTER TABLE communication.messages ADD CONSTRAINT messages_body_check CHECK (char_length(body) <= 4000);

CREATE TABLE communication.message_attachments (
    message_id uuid     NOT NULL REFERENCES communication.messages (id) ON DELETE CASCADE,
    file_id    uuid     NOT NULL REFERENCES platform.files (id),
    position   smallint NOT NULL DEFAULT 0,
    PRIMARY KEY (message_id, file_id)
);

-- +goose Down
DROP TABLE communication.message_attachments;
UPDATE communication.messages SET body = '(ek)' WHERE body = '';
ALTER TABLE communication.messages DROP CONSTRAINT messages_body_check;
ALTER TABLE communication.messages ADD CONSTRAINT messages_body_check CHECK (char_length(body) BETWEEN 1 AND 4000);
