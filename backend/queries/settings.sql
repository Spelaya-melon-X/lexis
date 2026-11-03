-- Запросы модуля settings. Значения — JSONB (у каждого ключа свой тип, см. settings.Settings).

-- name: ListSettings :many
SELECT key, value FROM settings;

-- name: UpsertSetting :exec
INSERT INTO settings (key, value) VALUES (@key, @value)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;
