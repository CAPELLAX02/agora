-- +goose Up

-- Dönem sonuçları: öğrencinin her dönem için yarıyıl (YANO) ve genel (GANO) ağırlıklı not
-- ortalaması, alınan ve kazanılan AKTS, onur durumu. Türetilmiş veridir: kesin kayıtlardan
-- yeniden üretilir; not ilanı olayıyla (grades.published) güncellenir ve ders seçmenin
-- kullandığı student_programs.gpa_cache ile earned_ects_cache'i besler.
--
-- Her satır o dönemin sonundaki durumdur: GANO, o döneme kadar alınan derslerin o dönemde
-- geçerli olan notlarıyla hesaplanır. Bir dersin sonraki dönemde tekrar alınması geçmiş
-- dönemlerin YANO ve GANO'sunu değiştirmez.
CREATE TABLE enrollment.term_results (
    student_program_id uuid         NOT NULL REFERENCES enrollment.student_programs (id),
    term_id            uuid         NOT NULL REFERENCES academic.terms (id),
    attempted_ects     numeric(5,1) NOT NULL CHECK (attempted_ects >= 0),   -- dönemde notu verilen derslerin AKTS'si
    earned_ects        numeric(5,1) NOT NULL CHECK (earned_ects >= 0),      -- bunlardan kazanılanlar
    term_gpa           numeric(3,2) CHECK (term_gpa BETWEEN 0 AND 4),       -- YANO; ortalamaya giren ders yoksa boş
    cumulative_ects    numeric(5,1) NOT NULL CHECK (cumulative_ects >= 0),  -- dönem sonunda geçerli notlarla kazanılan AKTS
    cumulative_gpa     numeric(3,2) CHECK (cumulative_gpa BETWEEN 0 AND 4), -- GANO
    standing           text         NOT NULL CHECK (standing IN ('NORMAL', 'HONOR', 'HIGH_HONOR')),
    computed_at        timestamptz  NOT NULL DEFAULT now(),
    PRIMARY KEY (student_program_id, term_id),
    CONSTRAINT term_results_earned_check CHECK (earned_ects <= attempted_ects),
    CONSTRAINT term_results_standing_gpa_check CHECK (standing = 'NORMAL' OR term_gpa IS NOT NULL)
);

-- Dönemin onur listesi ve analitik.
CREATE INDEX term_results_term_idx ON enrollment.term_results (term_id, standing);

-- +goose Down
DROP TABLE enrollment.term_results;
