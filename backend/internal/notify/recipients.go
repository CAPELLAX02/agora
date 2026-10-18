package notify

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// PermissionHolders, yetkiyi programı kapsayan (üniversite, programın fakültesi, bölümü ya da
// kendisi) etkin bir rol atamasıyla taşıyan etkin kullanıcılardır: ör. bir öğrencinin ekle-bırak
// talebini bölüm başkanı aşamasında görecek olanlar. Onay kuyruklarıyla aynı kapsam kuralı.
func PermissionHolders(ctx context.Context, q db.Querier, permission, programID string) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT ra.user_id
		FROM iam.role_assignments ra
		JOIN iam.role_permissions rp ON rp.role_id = ra.role_id
		JOIN iam.permissions p ON p.id = rp.permission_id AND p.code = $1
		JOIN iam.users u ON u.id = ra.user_id AND u.status = 'ACTIVE'
		JOIN org.programs pr ON pr.id = $2
		JOIN org.departments d ON d.id = pr.department_id
		WHERE ra.valid_from <= now() AND (ra.valid_until IS NULL OR ra.valid_until > now())
		  AND (ra.scope_type = 'UNIVERSITY'
		       OR (ra.scope_type = 'FACULTY' AND ra.scope_id = d.faculty_id)
		       OR (ra.scope_type = 'DEPARTMENT' AND ra.scope_id = d.id)
		       OR (ra.scope_type = 'PROGRAM' AND ra.scope_id = pr.id))
		ORDER BY 1`, permission, programID)
	if err != nil {
		return nil, fmt.Errorf("notify: yetki sahipleri okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("notify: yetki sahipleri okunamadı: %w", err)
	}
	return ids, nil
}
