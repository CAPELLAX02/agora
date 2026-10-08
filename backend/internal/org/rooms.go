package org

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Bina ve derslik yönetimi hataları.
var (
	ErrConflict         = errors.New("org: aynı kodla kayıt zaten var")
	ErrVersionMismatch  = errors.New("org: kayıt bu arada başka biri tarafından değiştirilmiş")
	ErrUnknownReference = errors.New("org: başvurulan kayıt bulunamadı")
)

// RoomType, dersliğin türüdür.
type RoomType string

// Derslik türleri. Değerler veritabanındaki CHECK kısıtıyla birebir aynıdır.
const (
	RoomLecture RoomType = "LECTURE"
	RoomLab     RoomType = "LAB"
	RoomAmphi   RoomType = "AMPHI"
	RoomOffice  RoomType = "OFFICE"
	RoomOnline  RoomType = "ONLINE"
)

// Valid, değerin tanımlı derslik türlerinden biri olup olmadığını söyler.
func (t RoomType) Valid() bool {
	switch t {
	case RoomLecture, RoomLab, RoomAmphi, RoomOffice, RoomOnline:
		return true
	}
	return false
}

// ClassroomFeatures, dersliklerde tanımlanabilen donanımlardır. Liste veritabanındaki
// CHECK kısıtıyla aynıdır.
var ClassroomFeatures = []string{
	"PROJECTOR", "SMART_BOARD", "COMPUTERS", "ACCESSIBLE", "AIR_CONDITIONING", "SOUND_SYSTEM", "RECORDING",
}

// BuildingRef, başka bir kaydın içinde gösterilen bina özetidir.
type BuildingRef struct {
	ID   string
	Code string
	Name string
}

// Building, bir yerleşkedeki binadır.
type Building struct {
	ID             string
	Code           string
	Name           string
	Campus         CampusRef
	Faculty        *FacultyRef // bir birime ait değilse nil
	IsActive       bool
	Version        int
	ClassroomCount int // aktif derslik sayısı
}

// Classroom, bir binadaki derslik, laboratuvar ya da amfidir.
type Classroom struct {
	ID           string
	Code         string
	Name         string
	Building     BuildingRef
	Campus       CampusRef
	FacultyID    string // binanın ait olduğu birim; yetki kapsamı için
	Capacity     int
	ExamCapacity int
	RoomType     RoomType
	Features     []string
	IsActive     bool
	Version      int
}

// ClassroomCursor, derslik listesinde bir sonraki sayfanın başlangıcıdır.
type ClassroomCursor struct {
	BuildingCode string `json:"b"`
	Code         string `json:"c"`
	ID           string `json:"i"`
}

// ClassroomFilter, derslik listesinin filtreleridir.
type ClassroomFilter struct {
	BuildingID      string
	CampusID        string
	RoomType        RoomType
	MinCapacity     int
	Query           string
	IncludeInactive bool
	After           *ClassroomCursor
	Limit           int
}

const buildingSelect = `
	SELECT b.id, b.code, b.name, b.is_active, b.version,
	       c.id, c.code, c.name,
	       f.id, f.code, f.name_tr,
	       (SELECT count(*) FROM org.classrooms cl WHERE cl.building_id = b.id AND cl.is_active)
	FROM org.buildings b
	JOIN org.campuses c ON c.id = b.campus_id
	LEFT JOIN org.faculties f ON f.id = b.faculty_id`

func scanBuilding(s scanner) (Building, error) {
	var (
		b                 Building
		fID, fCode, fName *string
	)
	err := s.Scan(&b.ID, &b.Code, &b.Name, &b.IsActive, &b.Version,
		&b.Campus.ID, &b.Campus.Code, &b.Campus.Name,
		&fID, &fCode, &fName, &b.ClassroomCount)
	if err != nil {
		return Building{}, err
	}
	if fID != nil {
		b.Faculty = &FacultyRef{ID: *fID, Code: *fCode, NameTR: *fName}
	}
	return b, nil
}

// ListBuildings, binaları yerleşke ve koda göre sıralı döndürür.
func (r *Repository) ListBuildings(ctx context.Context, campusID string, includeInactive bool) ([]Building, error) {
	var w db.Where
	if campusID != "" {
		w.Add("b.campus_id = $1", campusID)
	}
	if !includeInactive {
		w.Add("b.is_active")
	}
	rows, err := r.db.Query(ctx, buildingSelect+"\n\t"+w.SQL()+"\n\tORDER BY c.code, b.code", w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("org: binalar listelenemedi: %w", err)
	}
	return collect(rows, scanBuilding)
}

// GetBuilding, binayı döndürür. Yoksa ErrNotFound döner.
func (r *Repository) GetBuilding(ctx context.Context, id string) (Building, error) {
	b, err := scanBuilding(r.db.QueryRow(ctx, buildingSelect+"\n\tWHERE b.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Building{}, ErrNotFound
	}
	if err != nil {
		return Building{}, fmt.Errorf("org: bina okunamadı: %w", err)
	}
	return b, nil
}

const classroomSelect = `
	SELECT cl.id, cl.code, cl.name, cl.capacity, cl.exam_capacity, cl.room_type, cl.features,
	       cl.is_active, cl.version,
	       b.id, b.code, b.name, coalesce(b.faculty_id::text, ''),
	       c.id, c.code, c.name
	FROM org.classrooms cl
	JOIN org.buildings b ON b.id = cl.building_id
	JOIN org.campuses c ON c.id = b.campus_id`

func scanClassroom(s scanner) (Classroom, error) {
	var (
		cl       Classroom
		roomType string
	)
	err := s.Scan(&cl.ID, &cl.Code, &cl.Name, &cl.Capacity, &cl.ExamCapacity, &roomType, &cl.Features,
		&cl.IsActive, &cl.Version,
		&cl.Building.ID, &cl.Building.Code, &cl.Building.Name, &cl.FacultyID,
		&cl.Campus.ID, &cl.Campus.Code, &cl.Campus.Name)
	cl.RoomType = RoomType(roomType)
	return cl, err
}

// ListClassrooms, filtreye uyan derslikleri bina ve derslik koduna göre sıralı döndürür.
func (r *Repository) ListClassrooms(ctx context.Context, f ClassroomFilter) (rooms []Classroom, hasMore bool, err error) {
	var w db.Where
	if !f.IncludeInactive {
		w.Add("cl.is_active AND b.is_active")
	}
	if f.BuildingID != "" {
		w.Add("cl.building_id = $1", f.BuildingID)
	}
	if f.CampusID != "" {
		w.Add("b.campus_id = $1", f.CampusID)
	}
	if f.RoomType != "" {
		w.Add("cl.room_type = $1", string(f.RoomType))
	}
	if f.MinCapacity > 0 {
		w.Add("cl.capacity >= $1", f.MinCapacity)
	}
	if f.Query != "" {
		w.Add("(cl.code ILIKE $1 OR cl.name ILIKE $1 OR b.name ILIKE $1)", "%"+f.Query+"%")
	}
	if f.After != nil {
		w.Add("(b.code, cl.code, cl.id) > ($1, $2, $3::uuid)", f.After.BuildingCode, f.After.Code, f.After.ID)
	}
	limit := w.Arg(f.Limit + 1)

	rows, err := r.db.Query(ctx, classroomSelect+"\n\t"+w.SQL()+"\n\tORDER BY b.code, cl.code, cl.id\n\tLIMIT "+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("org: derslikler listelenemedi: %w", err)
	}
	rooms, err = collect(rows, scanClassroom)
	if err != nil {
		return nil, false, fmt.Errorf("org: derslikler okunamadı: %w", err)
	}
	if len(rooms) > f.Limit {
		return rooms[:f.Limit], true, nil
	}
	return rooms, false, nil
}

// GetClassroom, dersliği döndürür. Yoksa ErrNotFound döner.
func (r *Repository) GetClassroom(ctx context.Context, id string) (Classroom, error) {
	cl, err := scanClassroom(r.db.QueryRow(ctx, classroomSelect+"\n\tWHERE cl.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Classroom{}, ErrNotFound
	}
	if err != nil {
		return Classroom{}, fmt.Errorf("org: derslik okunamadı: %w", err)
	}
	return cl, nil
}

// BuildingInput, bina oluşturma ve güncelleme verisidir.
type BuildingInput struct {
	CampusID  string // sadece oluşturmada
	FacultyID string
	Code      string // sadece oluşturmada
	Name      string
	IsActive  bool // sadece güncellemede
}

// CreateBuilding, bir bina oluşturur ve denetim kaydı yazar.
func (r *Repository) CreateBuilding(ctx context.Context, actorID string, in BuildingInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO org.buildings (campus_id, faculty_id, code, name)
			VALUES ($1, $2, $3, $4) RETURNING id`,
			in.CampusID, nullable(in.FacultyID), in.Code, in.Name).Scan(&id)
		if err := writeError(err, "bina oluşturulamadı"); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "building.create", EntityType: "building", EntityID: id,
			After: map[string]any{"campus_id": in.CampusID, "faculty_id": in.FacultyID, "code": in.Code, "name": in.Name},
		})
	})
	return id, err
}

// UpdateBuilding, binanın adını, birimini ve etkinliğini günceller. version, istemcinin
// gördüğü sürümdür: kayıt bu arada değiştiyse ErrVersionMismatch döner.
func (r *Repository) UpdateBuilding(ctx context.Context, actorID, id string, version int, in BuildingInput) error {
	return db.InTx(ctx, r.db, func(tx pgx.Tx) error {
		before, err := scanBuilding(tx.QueryRow(ctx, buildingSelect+"\n\tWHERE b.id = $1\n\tFOR UPDATE OF b", id))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("org: bina okunamadı: %w", err)
		}
		if before.Version != version {
			return ErrVersionMismatch
		}

		_, err = tx.Exec(ctx, `
			UPDATE org.buildings
			SET faculty_id = $2, name = $3, is_active = $4, version = version + 1, updated_at = now()
			WHERE id = $1`, id, nullable(in.FacultyID), in.Name, in.IsActive)
		if err := writeError(err, "bina güncellenemedi"); err != nil {
			return err
		}

		facultyID := ""
		if before.Faculty != nil {
			facultyID = before.Faculty.ID
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "building.update", EntityType: "building", EntityID: id,
			Before: map[string]any{"faculty_id": facultyID, "name": before.Name, "is_active": before.IsActive},
			After:  map[string]any{"faculty_id": in.FacultyID, "name": in.Name, "is_active": in.IsActive},
		})
	})
}

// ClassroomInput, derslik oluşturma ve güncelleme verisidir.
type ClassroomInput struct {
	BuildingID   string // sadece oluşturmada
	Code         string // sadece oluşturmada
	Name         string
	Capacity     int
	ExamCapacity int
	RoomType     RoomType
	Features     []string
	IsActive     bool // sadece güncellemede
}

// CreateClassroom, bir derslik oluşturur ve denetim kaydı yazar.
func (r *Repository) CreateClassroom(ctx context.Context, actorID string, in ClassroomInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO org.classrooms (building_id, code, name, capacity, exam_capacity, room_type, features)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			in.BuildingID, in.Code, in.Name, in.Capacity, in.ExamCapacity, string(in.RoomType), normalizeFeatures(in.Features),
		).Scan(&id)
		if err := writeError(err, "derslik oluşturulamadı"); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "classroom.create", EntityType: "classroom", EntityID: id,
			After: classroomAudit(in),
		})
	})
	return id, err
}

// UpdateClassroom, dersliğin değiştirilebilir alanlarını günceller. version, istemcinin
// gördüğü sürümdür: kayıt bu arada değiştiyse ErrVersionMismatch döner ve hiçbir
// şey yazılmaz. Böylece aynı dersliği aynı anda düzenleyen iki kişiden birinin
// değişikliği sessizce kaybolmaz (kayıp güncelleme).
func (r *Repository) UpdateClassroom(ctx context.Context, actorID, id string, version int, in ClassroomInput) error {
	return db.InTx(ctx, r.db, func(tx pgx.Tx) error {
		before, err := scanClassroom(tx.QueryRow(ctx, classroomSelect+"\n\tWHERE cl.id = $1\n\tFOR UPDATE OF cl", id))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("org: derslik okunamadı: %w", err)
		}
		if before.Version != version {
			return ErrVersionMismatch
		}

		_, err = tx.Exec(ctx, `
			UPDATE org.classrooms
			SET name = $2, capacity = $3, exam_capacity = $4, room_type = $5, features = $6,
			    is_active = $7, version = version + 1, updated_at = now()
			WHERE id = $1`,
			id, in.Name, in.Capacity, in.ExamCapacity, string(in.RoomType), normalizeFeatures(in.Features), in.IsActive)
		if err := writeError(err, "derslik güncellenemedi"); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "classroom.update", EntityType: "classroom", EntityID: id,
			Before: classroomAudit(ClassroomInput{
				Name: before.Name, Capacity: before.Capacity, ExamCapacity: before.ExamCapacity,
				RoomType: before.RoomType, Features: before.Features, IsActive: before.IsActive,
			}),
			After: classroomAudit(in),
		})
	})
}

func classroomAudit(in ClassroomInput) map[string]any {
	m := map[string]any{
		"name": in.Name, "capacity": in.Capacity, "exam_capacity": in.ExamCapacity,
		"room_type": in.RoomType, "features": normalizeFeatures(in.Features), "is_active": in.IsActive,
	}
	if in.BuildingID != "" {
		m["building_id"], m["code"] = in.BuildingID, in.Code
	}
	return m
}

// normalizeFeatures, donanım listesini sıralar ve tekrarları atar: aynı derslik iki
// farklı sırayla kaydedildiğinde denetim kaydında sahte bir fark görünmesin. Liste
// hiç verilmemişse boş dizi döner: nil, veritabanına NULL olarak giderdi.
func normalizeFeatures(features []string) []string {
	out := append([]string{}, features...)
	slices.Sort(out)
	return slices.Compact(out)
}

// writeError, yazma hatalarını alan hatalarına çevirir.
func writeError(err error, msg string) error {
	if err == nil {
		return nil
	}
	if db.IsConflict(err) {
		return ErrConflict
	}
	if db.IsForeignKeyViolation(err) {
		return ErrUnknownReference
	}
	return fmt.Errorf("org: %s: %w", msg, err)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
