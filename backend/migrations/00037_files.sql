-- +goose Up

-- Yüklenen dosyaların üst verisi (yol haritası §5 #20). İçerik S3 uyumlu nesne deposundadır
-- (storage_key); bu tablo dosyanın kimliği, sahibi ve doğrulanmış özellikleridir. Dosyaya
-- erişim, onu kullanan varlık (ödev teslimi, ders içeriği ...) üzerinden denetlenir.
CREATE TABLE platform.files (
    id            uuid        PRIMARY KEY DEFAULT uuidv7(),
    storage_key   text        NOT NULL UNIQUE,
    original_name text        NOT NULL CHECK (length(original_name) BETWEEN 1 AND 255),
    -- İçerikten doğrulanmış tür: istemcinin bildirdiği Content-Type'a güvenilmez.
    mime_type     text        NOT NULL,
    size_bytes    bigint      NOT NULL CHECK (size_bytes >= 0),
    sha256        bytea       NOT NULL CHECK (length(sha256) = 32),
    uploaded_by   uuid        NOT NULL REFERENCES iam.users (id),
    uploaded_at   timestamptz NOT NULL DEFAULT now(),
    -- Bir varlığa bağlandığı an. Bağlanmadan bekleyen dosyalar worker'da temizlenir.
    attached_at   timestamptz,
    deleted_at    timestamptz
);

-- Temizlik işi: bağlanmamış, silinmemiş eski dosyalar.
CREATE INDEX files_unattached_idx ON platform.files (uploaded_at) WHERE attached_at IS NULL AND deleted_at IS NULL;
CREATE INDEX files_uploader_idx ON platform.files (uploaded_by, uploaded_at);

-- +goose Down
DROP TABLE platform.files;
