-- +goose Up
-- citext:     büyük/küçük harf duyarsız metin (kullanıcı adı, e-posta)
-- btree_gist: EXCLUDE kısıtlarında eşitlik + aralık çakışmasını birlikte kullanmak için
-- pg_trgm:    benzerlik tabanlı metin araması (ders adı, kişi adı)
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- +goose Down
DROP EXTENSION IF EXISTS pg_trgm;
DROP EXTENSION IF EXISTS btree_gist;
DROP EXTENSION IF EXISTS citext;
