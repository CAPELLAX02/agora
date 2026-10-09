-- +goose Up
-- Seçmeli yuva birden fazla dersi kapsayabilir: resmî ders planında "Alan Seçmeli 4. Yıl
-- (4 ders) 12-0-12 / 16 AKTS" tek satırdır. Yuvanın saat, kredi ve AKTS'si bu derslerin
-- toplamıdır; öğrenci havuzdan bu kadar ders seçer.
ALTER TABLE curriculum.curriculum_items
    ADD COLUMN slot_course_count smallint CHECK (slot_course_count BETWEEN 1 AND 20);

UPDATE curriculum.curriculum_items SET slot_course_count = 1 WHERE item_type = 'ELECTIVE_SLOT';

ALTER TABLE curriculum.curriculum_items
    ADD CONSTRAINT curriculum_items_slot_count_check
        CHECK ((item_type = 'ELECTIVE_SLOT') = (slot_course_count IS NOT NULL));

-- +goose Down
ALTER TABLE curriculum.curriculum_items DROP COLUMN slot_course_count;
