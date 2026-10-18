package notify

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Target, yetkinin aranacağı birimdir: program ya da bölüm (biri dolu). Üst birimlerdeki
// (bölüm, fakülte, üniversite) atamalar da kapsar.
type Target struct {
	ProgramID    string
	DepartmentID string
}

// PermissionHolders, yetkiyi hedefi kapsayan etkin bir rol atamasıyla taşıyan etkin
// kullanıcılardır: ör. bir öğrencinin ekle-bırak talebini bölüm başkanı aşamasında görecek
// olanlar. Onay kuyruklarıyla aynı kapsam kuralı.
func PermissionHolders(ctx context.Context, q db.Querier, permission string, t Target) ([]string, error) {
	rows, err := q.Query(ctx, `
		WITH t AS (
			SELECT p.id AS program_id, d.id AS department_id, d.faculty_id
			FROM org.departments d
			LEFT JOIN org.programs p ON p.id = $2::uuid
			WHERE d.id = coalesce(p.department_id, $3::uuid)
		)
		SELECT DISTINCT ra.user_id
		FROM t, iam.role_assignments ra
		JOIN iam.role_permissions rp ON rp.role_id = ra.role_id
		JOIN iam.permissions p ON p.id = rp.permission_id AND p.code = $1
		JOIN iam.users u ON u.id = ra.user_id AND u.status = 'ACTIVE'
		WHERE ra.valid_from <= now() AND (ra.valid_until IS NULL OR ra.valid_until > now())
		  AND (ra.scope_type = 'UNIVERSITY'
		       OR (ra.scope_type = 'FACULTY' AND ra.scope_id = t.faculty_id)
		       OR (ra.scope_type = 'DEPARTMENT' AND ra.scope_id = t.department_id)
		       OR (ra.scope_type = 'PROGRAM' AND ra.scope_id = t.program_id))
		ORDER BY 1`, permission, nullUUID(t.ProgramID), nullUUID(t.DepartmentID))
	if err != nil {
		return nil, fmt.Errorf("notify: yetki sahipleri okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("notify: yetki sahipleri okunamadı: %w", err)
	}
	return ids, nil
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
