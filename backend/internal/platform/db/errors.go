package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// 23505 — unique_violation. Код строкой, чтобы не тянуть jackc/pgerrcode ради одной константы.
const pgUniqueViolation = "23505"

// IsUniqueViolation — нарушено UNIQUE-ограничение (дубликат). Работает и с обёрнутой ошибкой.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

// IsIntegrityViolation — нарушено любое ограничение целостности (класс 23: CHECK, UNIQUE,
// NOT NULL, FK). Для импорта: такая ошибка — проблема данных во входном файле, а не сервера.
func IsIntegrityViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && len(pgErr.Code) == 5 && pgErr.Code[:2] == "23"
}
