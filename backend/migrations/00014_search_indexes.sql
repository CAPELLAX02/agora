-- +goose Up
-- Arama kutuları "içinde geçen" (ILIKE '%...%') sorgular kullanır: B-tree index bunlara
-- yardım edemez. Trigram (pg_trgm) GIN index'leri, 92 bin öğrencilik veride aramayı
-- tam tablo taramasından index taramasına çevirir. İfadeler sorgulardakiyle birebir
-- aynı olmalıdır, aksi halde index kullanılmaz.
CREATE INDEX persons_full_name_trgm_idx ON people.persons
    USING gin ((first_name || ' ' || last_name) gin_trgm_ops);
CREATE INDEX students_student_no_trgm_idx ON people.students USING gin (student_no gin_trgm_ops);
-- citext sütunlarda index text'e çevrilen ifade üzerindedir; sorgular da ::text ile arar.
CREATE INDEX users_username_trgm_idx ON iam.users USING gin ((username::text) gin_trgm_ops);
CREATE INDEX users_email_trgm_idx ON iam.users USING gin ((email::text) gin_trgm_ops);

-- +goose Down
DROP INDEX iam.users_email_trgm_idx;
DROP INDEX iam.users_username_trgm_idx;
DROP INDEX people.students_student_no_trgm_idx;
DROP INDEX people.persons_full_name_trgm_idx;
