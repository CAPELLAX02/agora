-- +goose Up
-- Liste uç noktaları Türkçe ada göre sıralayıp keyset (cursor) sayfalama yapar.
-- Sıralama ifadesiyle birebir aynı collation'a sahip index, ORDER BY ... LIMIT
-- sorgusunun tüm tabloyu sıralamadan index üzerinden yürümesini sağlar.
CREATE INDEX faculties_name_tr_idx ON org.faculties (name_tr COLLATE "tr-x-icu", id);
CREATE INDEX departments_name_tr_idx ON org.departments (name_tr COLLATE "tr-x-icu", id);
CREATE INDEX programs_name_tr_idx ON org.programs (name_tr COLLATE "tr-x-icu", id);

-- +goose Down
DROP INDEX org.programs_name_tr_idx;
DROP INDEX org.departments_name_tr_idx;
DROP INDEX org.faculties_name_tr_idx;