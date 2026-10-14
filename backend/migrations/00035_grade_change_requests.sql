-- +goose Up

-- Not düzeltme talepleri: harf notları ilan edildikten sonra not sadece bu yolla değişir. Şubenin
-- öğretim elemanı gerekçesiyle talep eder; bölüm başkanı ya da fakülte öğrenci işleri onaylar ya
-- da reddeder (görevler ayrılığı: talep eden onaylayamaz). Onay notu uygular: bileşen notu ve
-- harf, not geçmişine CORRECTION satırı, sınıf istatistiği ve dönem sonuçları.
--
-- İki tür: SCORE bir bileşen notunu düzeltir, harf notu motorla yeniden hesaplanır; LETTER harfi
-- doğrudan değiştirir (ders başarı puanı aynı kalır). old_value ve new_value talep anındaki
-- değerlerdir: {"letter", "weighted", "reason"} ve SCORE'da {"score", "absent"}.
CREATE TABLE grading.grade_change_requests (
    id            uuid         PRIMARY KEY DEFAULT uuidv7(),
    enrollment_id uuid         NOT NULL REFERENCES enrollment.enrollments (id),
    section_id    uuid         NOT NULL REFERENCES offering.sections (id), -- kesin kaydın şubesi (kopya)
    component_id  uuid         REFERENCES grading.assessment_components (id),
    kind          text         NOT NULL CHECK (kind IN ('SCORE', 'LETTER')),
    old_value     jsonb        NOT NULL,
    new_value     jsonb        NOT NULL,
    reason        text         NOT NULL CHECK (char_length(btrim(reason)) BETWEEN 10 AND 500),
    requested_by  uuid         NOT NULL REFERENCES iam.users (id),
    requested_at  timestamptz  NOT NULL DEFAULT now(),
    status        text         NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'APPLIED', 'REJECTED', 'WITHDRAWN')),
    reviewed_by   uuid         REFERENCES iam.users (id),
    review_note   text         CHECK (review_note IS NULL OR char_length(btrim(review_note)) BETWEEN 5 AND 500),
    decided_at    timestamptz,

    CONSTRAINT grade_change_requests_kind_component_check CHECK ((kind = 'SCORE') = (component_id IS NOT NULL)),
    CONSTRAINT grade_change_requests_decided_check CHECK ((status = 'PENDING') = (decided_at IS NULL)),
    CONSTRAINT grade_change_requests_reviewer_check
        CHECK ((status IN ('APPLIED', 'REJECTED')) = (reviewed_by IS NOT NULL)),
    CONSTRAINT grade_change_requests_separation_check CHECK (reviewed_by IS NULL OR reviewed_by <> requested_by),
    CONSTRAINT grade_change_requests_reject_note_check CHECK (status <> 'REJECTED' OR review_note IS NOT NULL)
);

-- Bir kesin kayıtta aynı anda tek bekleyen talep.
CREATE UNIQUE INDEX grade_change_requests_pending_idx ON grading.grade_change_requests (enrollment_id) WHERE status = 'PENDING';
-- Onay kuyruğu ve öğretim elemanının talepleri.
CREATE INDEX grade_change_requests_status_idx ON grading.grade_change_requests (status, requested_at);
CREATE INDEX grade_change_requests_requester_idx ON grading.grade_change_requests (requested_by, requested_at DESC);

-- +goose Down
DROP TABLE grading.grade_change_requests;
