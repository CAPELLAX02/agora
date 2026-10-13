package grading

import (
	"context"
	"fmt"
	"sort"

	"github.com/CAPELLAX02/agora/backend/internal/grading/gpa"
	"github.com/CAPELLAX02/agora/backend/internal/grading/letter"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Hypothetical, sanal transkriptte bir kesin kayda verilen varsayımsal harftir.
type Hypothetical struct {
	EnrollmentID string
	Letter       string
}

// NotInProgramError: varsayımsal harf verilen kesin kayıt öğrencinin bu programında yok (ya
// da bırakılmış).
type NotInProgramError struct {
	IDs []string
}

func (e *NotInProgramError) Error() string {
	return fmt.Sprintf("grading: programda olmayan %d kesin kayıt", len(e.IDs))
}

// Simulation, sanal transkriptin sonucudur: aynı kesin kayıtlardan gerçek harflerle ve
// varsayımsal harflerle hesaplanan dönem sonuçları.
type Simulation struct {
	Actual    []gpa.Result
	Simulated []gpa.Result
}

// SimulateTermResults, program kaydının dönem sonuçlarını varsayımsal harflerle hesaplar;
// hiçbir şey yazmaz. Hesap dönem sonuçlarınınkiyle aynıdır (gpa.Compute). Notu henüz verilmemiş
// bir derse harf verilirse ders notlanmış sayılır ve ilanda olacağı gibi yerini aldığı önceki
// denemeler (tekrar, eşdeğer ders, kalınan seçmelinin yerine alınan ders) GANO'dan düşer. Geçmiş
// bir dersin harfini değiştirmek "o dönem bu notu alsaydım" sorusunu yanıtlar. Harf, kaydın not
// ölçeğinde derse verilebilen harflerden olmalıdır (ManualLetters).
func SimulateTermResults(ctx context.Context, q db.Querier, studentProgramID string, changes []Hypothetical) (Simulation, error) {
	terms, all, err := loadGPAInputs(ctx, q, []string{studentProgramID})
	if err != nil {
		return Simulation{}, err
	}
	attempts := all[studentProgramID]
	sim := Simulation{Actual: gpa.Compute(terms[studentProgramID], plainAttempts(attempts))}

	index := make(map[string]int, len(attempts))
	for i, a := range attempts {
		index[a.EnrollmentID] = i
	}
	var unknown []string
	for _, c := range changes {
		if _, ok := index[c.EnrollmentID]; !ok {
			unknown = append(unknown, c.EnrollmentID)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Simulation{}, &NotInProgramError{IDs: unknown}
	}

	scales := map[string]letter.Scale{}
	changed := make([]gpaAttempt, len(attempts))
	copy(changed, attempts)
	for _, c := range changes {
		a := &changed[index[c.EnrollmentID]]
		scale, ok := scales[a.ScaleID]
		if !ok {
			if scale, err = loadScale(ctx, q, a.ScaleID); err != nil {
				return Simulation{}, err
			}
			scales[a.ScaleID] = scale
		}
		g, ok := ManualLetters(scale, a.PassFail).Find(c.Letter)
		if !ok {
			return Simulation{}, fmt.Errorf("%w: %s", ErrLetterNotAllowed, c.Letter)
		}
		pending := !a.Completed
		a.Completed, a.ViaMakeup = true, false
		a.CountsInGPA, a.EarnsECTS, a.Passing = g.CountsInGPA, g.EarnsECTS, g.Passing
		a.Coefficient = 0
		if g.Coefficient != nil {
			a.Coefficient = gpa.Hundredths(*g.Coefficient)
		}
		if !pending {
			continue
		}
		previous, err := supersedeTargets(ctx, q, a.EnrollmentID)
		if err != nil {
			return Simulation{}, err
		}
		for _, id := range previous {
			if i, ok := index[id]; ok && changed[i].SupersededIn == "" {
				changed[i].SupersededIn = a.TermID
			}
		}
	}
	sim.Simulated = gpa.Compute(terms[studentProgramID], plainAttempts(changed))
	return sim, nil
}

// LetterOptions, bir kesin kaydın alabileceği harflerdir (sanal transkriptteki seçenekler):
// kaydın not ölçeğinde derse verilebilen harfler (ManualLetters), ölçek sırasıyla.
func LetterOptions(ctx context.Context, q db.Querier, scaleID string, passFail bool) ([]string, error) {
	scale, err := loadScale(ctx, q, scaleID)
	if err != nil {
		return nil, err
	}
	allowed := ManualLetters(scale, passFail)
	out := make([]string, 0, len(allowed))
	for _, g := range allowed {
		out = append(out, g.Letter)
	}
	return out, nil
}
