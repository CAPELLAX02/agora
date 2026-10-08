-- +goose Up
-- Parola değişikliğinde kullanıcının diğer oturumları kapatılır. Sebep ayrı tutulur:
-- "parolasını değiştirdi" ile "her yerden çıkış yaptı" güvenlik incelemesinde farklı anlamlar taşır.
ALTER TABLE iam.sessions DROP CONSTRAINT sessions_revoke_reason_check;
ALTER TABLE iam.sessions ADD CONSTRAINT sessions_revoke_reason_check
    CHECK (revoke_reason IN ('LOGOUT', 'LOGOUT_ALL', 'REUSE_DETECTED', 'PASSWORD_CHANGE', 'PASSWORD_RESET', 'ADMIN'));

-- +goose Down
UPDATE iam.sessions SET revoke_reason = 'LOGOUT_ALL' WHERE revoke_reason = 'PASSWORD_CHANGE';
ALTER TABLE iam.sessions DROP CONSTRAINT sessions_revoke_reason_check;
ALTER TABLE iam.sessions ADD CONSTRAINT sessions_revoke_reason_check
    CHECK (revoke_reason IN ('LOGOUT', 'LOGOUT_ALL', 'REUSE_DETECTED', 'PASSWORD_RESET', 'ADMIN'));
