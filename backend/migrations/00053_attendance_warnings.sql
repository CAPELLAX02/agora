-- +goose Up

-- Devamsızlık uyarıları: öğrenci, oturum grubunda (teori ya da uygulama) sınırın %70'ine
-- ulaşınca (NEAR) ve sınırı aşınca (OVER) birer kez bildirim alır. Yoklama düzeltmesiyle
-- devamsızlık azalsa da gönderilmiş uyarı geri alınmaz.
CREATE TABLE attendance.warnings_sent (
    enrollment_id uuid        NOT NULL REFERENCES enrollment.enrollments (id) ON DELETE CASCADE,
    session_group text        NOT NULL CHECK (session_group IN ('THEORY', 'PRACTICE')),
    level         text        NOT NULL CHECK (level IN ('NEAR', 'OVER')),
    sent_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (enrollment_id, session_group, level)
);

INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order) VALUES
    ('attendance.warning', 'COURSES', 'Devamsızlık uyarısı', 'Attendance warning', '{IN_APP,EMAIL}', false, 45);

-- +goose Down
DELETE FROM communication.notification_preferences WHERE type_code = 'attendance.warning';
DELETE FROM communication.notifications WHERE type_code = 'attendance.warning';
DELETE FROM communication.notification_types WHERE code = 'attendance.warning';
DROP TABLE attendance.warnings_sent;
