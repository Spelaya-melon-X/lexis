// Package app — контракт модуля модульного монолита.
package app

import (
	"context"

	"github.com/go-chi/chi/v5"
)

// Module — бизнес-модуль (words, groups, training, ...).
// Новый модуль = новый пакет + одна строка регистрации в cmd/server/main.go.
type Module interface {
	// Name — сегмент URL: роуты модуля монтируются под /api/<Name>.
	Name() string
	Routes(r chi.Router)
}

// Starter — опциональный интерфейс для модулей с фоновой работой
// (сейчас таких нет; задел на будущие модули вроде планера). main проверяет его через type assertion:
//
//	if s, ok := m.(app.Starter); ok { ... }
//
// Start должен вернуть управление сразу, а фоновые горутины — завершиться,
// когда отменён ctx. Так модулям без фона не нужно писать пустой Start.
type Starter interface {
	Start(ctx context.Context) error
}
