package settings

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/settings/store"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	rows, err := store.New(s.pool).ListSettings(ctx)
	if err != nil {
		return Settings{}, fmt.Errorf("settings get: %w", err)
	}
	return fromRows(rows), nil
}

// Update применяет патч и возвращает итоговые настройки. Пишем все ключи целиком:
// их два, а так в БД не остаётся «половины» настроек с дефолтами из кода.
func (s *Service) Update(ctx context.Context, p Patch) (Settings, error) {
	var out Settings
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListSettings(ctx)
		if err != nil {
			return fmt.Errorf("list: %w", err)
		}
		out, err = p.Apply(fromRows(rows))
		if err != nil {
			return err
		}
		for _, r := range out.rows() {
			if err := q.UpsertSetting(ctx, r); err != nil {
				return fmt.Errorf("upsert %s: %w", r.Key, err)
			}
		}
		return nil
	})
	if err != nil {
		return Settings{}, fmt.Errorf("settings update: %w", err)
	}
	return out, nil
}
