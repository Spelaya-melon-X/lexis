package tasks

import (
	"context"
	"fmt"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
	"lexis/internal/tasks/store"
)

type CalendarItem struct {
	Day    string `json:"day"`
	TaskID int64  `json:"taskId"`
	Start  int    `json:"start"`
	Title  string `json:"title"`
	Done   bool   `json:"done"`
}

type Calendar struct {
	Today string         `json:"today"`
	From  string         `json:"from"`
	To    string         `json:"to"`
	Items []CalendarItem `json:"items"`
}

// Calendar читает пункты плана вместе с названиями дел одним запросом.
// Без границ возвращает текущий месяц в таймзоне Lexis.
func (s *Service) Calendar(ctx context.Context, fromS, toS string) (Calendar, error) {
	today := clock.TodayDate(s.clock)
	if fromS == "" && toS == "" {
		first := today.AddDate(0, 0, 1-today.Day())
		fromS = clock.FormatDate(first)
		toS = clock.FormatDate(first.AddDate(0, 1, -1))
	}
	from, to, err := days.ParseRange(fromS, toS, today, 1)
	if err != nil {
		return Calendar{}, err
	}
	rows, err := store.New(s.pool).ListCalendarItems(ctx, store.ListCalendarItemsParams{FromDay: from, ToDay: to})
	if err != nil {
		return Calendar{}, fmt.Errorf("list calendar items: %w", err)
	}
	items := make([]CalendarItem, len(rows))
	for i, row := range rows {
		items[i] = CalendarItem{
			Day: clock.FormatDate(row.Day), TaskID: row.TaskID, Start: int(row.StartMin), Title: row.Title, Done: row.Done,
		}
	}
	return Calendar{Today: clock.FormatDate(today), From: clock.FormatDate(from), To: clock.FormatDate(to), Items: items}, nil
}
