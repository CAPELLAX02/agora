-- +goose Up
-- Sistem yöneticisi denetim izini okuyabilir (03 dokümanı §4.4, yetki matrisi).
-- 00006'da eksik kalmıştı.
INSERT INTO iam.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM iam.roles r, iam.permissions p
WHERE r.code = 'SYSTEM_ADMIN' AND p.code = 'audit:read';

-- Rol-yetki matrisi değişince o role sahip kullanıcıların yetki sürümü artırılmalı:
-- aksi halde önbellekteki eski yetkiler, kaydın süresi dolana kadar kullanılır.
UPDATE iam.users SET perm_version = perm_version + 1, updated_at = now()
WHERE id IN (
    SELECT ra.user_id FROM iam.role_assignments ra
    JOIN iam.roles r ON r.id = ra.role_id
    WHERE r.code = 'SYSTEM_ADMIN'
);

-- +goose Down
DELETE FROM iam.role_permissions
WHERE role_id = (SELECT id FROM iam.roles WHERE code = 'SYSTEM_ADMIN')
  AND permission_id = (SELECT id FROM iam.permissions WHERE code = 'audit:read');

UPDATE iam.users SET perm_version = perm_version + 1, updated_at = now()
WHERE id IN (
    SELECT ra.user_id FROM iam.role_assignments ra
    JOIN iam.roles r ON r.id = ra.role_id
    WHERE r.code = 'SYSTEM_ADMIN'
);
