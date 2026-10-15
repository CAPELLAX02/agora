-- +goose Up

-- Ödev ↔ değerlendirme bileşeni köprüsü: ödev, şubenin dönem içi bir bileşenini ağırlıkla
-- besler; bağlı ödevlerin yayımlanmış puanlarından bileşen notu üretilir (kaynak LMS). Bir
-- bileşeni birden çok ödev besleyebilir (ör. üç ödev → ÖDEV %20); plan değişip bileşen
-- silinirse bağlantı da silinir.
CREATE TABLE lms.assignment_links (
    activity_id  uuid         PRIMARY KEY REFERENCES lms.assignments (activity_id),
    component_id uuid         NOT NULL REFERENCES grading.assessment_components (id) ON DELETE CASCADE,
    weight       numeric(5,2) NOT NULL DEFAULT 1 CHECK (weight > 0 AND weight <= 100),
    created_by   uuid         NOT NULL REFERENCES iam.users (id),
    updated_at   timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX assignment_links_component_idx ON lms.assignment_links (component_id);

-- +goose Down
DROP TABLE lms.assignment_links;
