//go:build integration

// Интеграционные тесты journal на живом Postgres (запуск — SPEC §15).
package journal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/habits"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
)

func patch(t *testing.T, body string) Patch {
	t.Helper()
	var p Patch
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("patch %s: %v", body, err)
	}
	return p
}

func TestService(t *testing.T) {
	pool := dbtest.New(t)
	msk, _ := time.LoadLocation("Europe/Moscow")
	// 00:30 МСК 3-го: в UTC ещё 2-е — «сегодня» обязано быть 3-е.
	clk := clock.NewFixed(time.Date(2026, 11, 3, 0, 30, 0, 0, msk))
	svc := NewService(pool, clk, nil)
	ctx := context.Background()

	// Пустой день — не 404, а пустая запись с 7 пустыми квадратиками.
	d, err := svc.Get(ctx, "2026-11-03")
	if err != nil {
		t.Fatal(err)
	}
	if d.Rating != nil || d.Story != "" || len(d.Wins) != 0 || len(d.Dopamine) != DopamineSlots || d.DopamineLevel != 0 {
		t.Fatalf("empty day = %+v", d)
	}

	d, err = svc.Update(ctx, "2026-11-03", patch(t, `{"rating": 8, "story": "хороший день", "wins": [" пробежал 5 км ", "", "дочитал книгу"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if *d.Rating != 8 || d.Story != "хороший день" || !reflect.DeepEqual(d.Wins, []string{"пробежал 5 км", "дочитал книгу"}) {
		t.Fatalf("after patch = %+v", d)
	}

	// Частичный патч не трогает остальное; null снимает оценку; порядок пунктов сохраняется.
	d, err = svc.Update(ctx, "2026-11-03", patch(t, `{"rating": null, "wins": ["дочитал книгу", "пробежал 5 км"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Rating != nil || d.Story != "хороший день" || !reflect.DeepEqual(d.Wins, []string{"дочитал книгу", "пробежал 5 км"}) {
		t.Fatalf("partial patch = %+v", d)
	}

	// Дофамин: зажечь два, переписать один, погасить другой.
	for slot, text := range map[int]string{0: "кофе", 4: "звонок другу"} {
		if _, err := svc.SetDopamine(ctx, "2026-11-03", slot, text); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.SetDopamine(ctx, "2026-11-03", 0, "  кофе с другом "); err != nil {
		t.Fatal(err)
	}
	d, err = svc.SetDopamine(ctx, "2026-11-03", 4, "")
	if err != nil {
		t.Fatal(err)
	}
	if d.DopamineLevel != 1 || d.Dopamine[0] == nil || *d.Dopamine[0] != "кофе с другом" || d.Dopamine[4] != nil {
		t.Fatalf("dopamine = %v level %d", d.Dopamine, d.DopamineLevel)
	}

	// Дофамин в новый день создаёт строку дня (FK).
	if _, err := svc.SetDopamine(ctx, "2026-10-30", 6, "музыка"); err != nil {
		t.Fatal(err)
	}

	// Сводка: все дни окна, включая пустые.
	from, to, days, err := svc.List(ctx, "2026-10-30", "2026-11-03")
	if err != nil {
		t.Fatal(err)
	}
	if clock.FormatDate(from) != "2026-10-30" || clock.FormatDate(to) != "2026-11-03" || len(days) != 5 {
		t.Fatalf("list window %v..%v, %d days", from, to, len(days))
	}
	if days[0].DopamineLevel != 1 || days[0].Rating != nil {
		t.Errorf("2026-10-30 = %+v", days[0])
	}
	if days[1] != (DaySummary{Day: "2026-10-31"}) {
		t.Errorf("empty day = %+v", days[1])
	}
	if last := days[4]; last.Day != "2026-11-03" || last.DopamineLevel != 1 || last.Wins != 2 || !last.HasStory {
		t.Errorf("2026-11-03 = %+v", last)
	}

	// 23:59 МСК — ещё 3-е; через минуту 4-е уже «сегодня» и в него можно писать.
	clk.Set(time.Date(2026, 11, 3, 23, 59, 0, 0, msk))
	if _, err := svc.Update(ctx, "2026-11-04", patch(t, `{"rating": 5}`)); errCode(err) != "future_day" {
		t.Fatalf("4th at 23:59 must be future, got %v", err)
	}
	clk.Advance(2 * time.Minute)
	if _, err := svc.Update(ctx, "2026-11-04", patch(t, `{"rating": 5}`)); err != nil {
		t.Fatalf("4th after midnight: %v", err)
	}
}

func TestModule_HTTP(t *testing.T) {
	pool := dbtest.New(t)
	msk, _ := time.LoadLocation("Europe/Moscow")
	r := chi.NewRouter()
	r.Route("/api/journal", New(pool, clock.NewFixed(time.Date(2026, 11, 3, 12, 0, 0, 0, msk)), nil).Routes)

	do := func(method, path, body string) map[string]any {
		t.Helper()
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s %s: status %d, body %s", method, path, rr.Code, rr.Body)
		}
		var out map[string]any
		if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	got := do(http.MethodPut, "/api/journal/days/2026-11-03/dopamine/2", `{"text": "солнце"}`)
	if got["dopamineLevel"] != 1.0 || got["dopamine"].([]any)[2] != "солнце" || got["rating"] != nil {
		t.Errorf("put dopamine = %v", got)
	}
	got = do(http.MethodPatch, "/api/journal/days/2026-11-03", `{"rating": 9}`)
	if got["rating"] != 9.0 || got["day"] != "2026-11-03" {
		t.Errorf("patch = %v", got)
	}
	got = do(http.MethodGet, "/api/journal/days?from=2026-11-01&to=2026-11-03", "")
	if days := got["days"].([]any); len(days) != 3 || days[2].(map[string]any)["rating"] != 9.0 {
		t.Errorf("list = %v", got)
	}
}

func TestService_Week(t *testing.T) {
	pool := dbtest.New(t)
	msk, _ := time.LoadLocation("Europe/Moscow")
	// Четверг 5 ноября, неделя 2..8 ноября. Привычки создаём «в понедельник».
	clk := clock.NewFixed(time.Date(2026, 11, 2, 9, 0, 0, 0, msk))
	hs := habits.NewService(pool, clk)
	svc := NewService(pool, clk, hs)
	ctx := context.Background()

	run, err := hs.Create(ctx, habits.Fields{Name: "Бег", Kind: habits.KindPositive, TrackAmount: true, Unit: "км"})
	if err != nil {
		t.Fatal(err)
	}
	smoke, err := hs.Create(ctx, habits.Fields{Name: "Курение", Kind: habits.KindNegative})
	if err != nil {
		t.Fatal(err)
	}
	clk.Set(time.Date(2026, 11, 5, 21, 0, 0, 0, msk))
	f := func(v float64) *float64 { return &v }
	for _, d := range []string{"2026-11-02", "2026-11-04"} {
		if _, err := hs.SaveDay(ctx, d, []habits.CheckinInput{{HabitID: run.ID, Done: true, Amount: f(5)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := hs.SaveDay(ctx, "2026-11-03", []habits.CheckinInput{{HabitID: smoke.ID, Done: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := hs.CreateGoal(ctx, habits.GoalInput{HabitID: &run.ID, From: "2026-11-01", To: "2026-11-09"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hs.CreateGoal(ctx, habits.GoalInput{Title: "в декабре", From: "2026-12-01", To: "2026-12-02"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "2026-11-02", patch(t, `{"rating": 8}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "2026-11-04", patch(t, `{"rating": 6}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetDopamine(ctx, "2026-11-04", 0, "кофе"); err != nil {
		t.Fatal(err)
	}

	// Пустая неделя — не 404, а пустой анализ со сводкой.
	w, err := svc.Week(ctx, "2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	if w.WeekEnd != "2026-11-08" || len(w.Blocks) != 0 || w.UpdatedAt != nil || len(w.Summary.Days) != 7 {
		t.Fatalf("empty week = %+v", w)
	}
	if w.Summary.Filled != 2 || *w.Summary.AvgRating != 7 || *w.Summary.AvgDopamine != 0.5 {
		t.Errorf("averages = %+v", w.Summary.PeriodAverages)
	}
	if len(w.Summary.Goals) != 1 || w.Summary.Goals[0].Progress == nil {
		t.Errorf("goals = %+v", w.Summary.Goals)
	}
	stats := map[string]habits.PeriodStat{}
	for _, s := range w.Summary.Habits {
		stats[s.Name] = s
	}
	// Пн..Чт учитываются (сегодня четверг), бег — 2 дня и 10 км, курение — срыв во вторник.
	if r := stats["Бег"]; r.Days != 4 || r.Checked != 2 || *r.Amount != 10 {
		t.Errorf("run = %+v", r.Counted)
	}
	if s := stats["Курение"]; s.Days != 4 || s.Checked != 1 || s.BestClean != 2 {
		t.Errorf("smoke = %+v", s.Counted)
	}

	// Сохранить, перезаписать, очистить.
	w, err = svc.SaveWeek(ctx, "2026-11-02", []Block{{Topic: " Спорт ", Conclusion: "бегать по утрам"}, {Topic: "", Conclusion: ""}, {Topic: "Сон"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Blocks) != 2 || w.Blocks[0].Topic != "Спорт" || w.UpdatedAt == nil || len(w.Summary.Habits) != 2 {
		t.Fatalf("saved = %+v", w)
	}
	w, err = svc.SaveWeek(ctx, "2026-11-02", []Block{{Topic: "Сон"}, {Topic: "Спорт", Conclusion: "бегать по утрам"}})
	if err != nil || len(w.Blocks) != 2 || w.Blocks[0].Topic != "Сон" {
		t.Fatalf("reordered = %+v %v", w.Blocks, err)
	}
	if w, err = svc.SaveWeek(ctx, "2026-11-02", nil); err != nil || len(w.Blocks) != 0 || w.UpdatedAt != nil {
		t.Fatalf("cleared = %+v %v", w, err)
	}

	// Прошлую неделю — можно, следующую — нет.
	if _, err := svc.SaveWeek(ctx, "2026-10-26", []Block{{Topic: "x"}}); err != nil {
		t.Errorf("past week: %v", err)
	}
	if _, err := svc.SaveWeek(ctx, "2026-11-09", []Block{{Topic: "x"}}); errCode(err) != "future_day" {
		t.Errorf("future week: %v", err)
	}
}
