// Package config читает настройки процесса из переменных окружения.
// Пользовательские настройки (язык, лимит новых слов) живут в БД — модуль settings.
package config

import (
	"errors"
	"fmt"
	"time"
)

const (
	defaultHTTPAddr = ":8080"
	defaultTZ       = "Europe/Moscow"
)

type Config struct {
	HTTPAddr string         // LEXIS_HTTP_ADDR
	DBDSN    string         // LEXIS_DB_DSN, обязательна
	Location *time.Location // LEXIS_TZ
}

// Load собирает Config из окружения.
//
// getenv передаётся параметром (в main — os.Getenv), чтобы тесты не трогали
// глобальное окружение процесса и могли идти параллельно.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr: valueOr(getenv("LEXIS_HTTP_ADDR"), defaultHTTPAddr),
		DBDSN:    getenv("LEXIS_DB_DSN"),
	}
	if cfg.DBDSN == "" {
		return Config{}, errors.New("config: LEXIS_DB_DSN is required")
	}

	tz := valueOr(getenv("LEXIS_TZ"), defaultTZ)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return Config{}, fmt.Errorf("config: LEXIS_TZ=%q: %w", tz, err)
	}
	cfg.Location = loc

	return cfg, nil
}

func valueOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
