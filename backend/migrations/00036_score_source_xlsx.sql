-- +goose Up

-- Bileşen notları Excel şablonundan da içe aktarılır: notun kaynağı CSV ve Excel olarak ayrılır.
ALTER TABLE grading.assessment_scores
    DROP CONSTRAINT assessment_scores_source_check,
    ADD CONSTRAINT assessment_scores_source_check CHECK (source IN ('MANUAL', 'CSV', 'XLSX', 'LMS'));

-- +goose Down
UPDATE grading.assessment_scores SET source = 'CSV' WHERE source = 'XLSX';
ALTER TABLE grading.assessment_scores
    DROP CONSTRAINT assessment_scores_source_check,
    ADD CONSTRAINT assessment_scores_source_check CHECK (source IN ('MANUAL', 'CSV', 'LMS'));
