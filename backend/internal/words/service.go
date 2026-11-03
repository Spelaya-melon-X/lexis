package words

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
	"lexis/internal/words/store"
)

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	bus   eventbus.Publisher
}

func NewService(pool *pgxpool.Pool, clk clock.Clock, bus eventbus.Publisher) *Service {
	return &Service{pool: pool, clock: clk, bus: bus}
}

// Word — слово в ответах API.
type Word struct {
	ID             int64      `json:"id"`
	Term           string     `json:"term"`
	Forms          []string   `json:"forms,omitempty"`
	Translation    string     `json:"translation"`
	Level          int16      `json:"level"`
	Streak         int16      `json:"streak"`
	NextReviewAt   time.Time  `json:"nextReviewAt"`
	LastReviewedAt *time.Time `json:"lastReviewedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	Examples       []Example  `json:"examples,omitempty"`
	Groups         []GroupRef `json:"groups,omitempty"`
}

type GroupRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type BulkResult struct {
	Created int         `json:"created"`
	Merged  int         `json:"merged"`
	Errors  []LineError `json:"errors"`
}

// Bulk парсит текст и сохраняет все валидные карточки одной транзакцией:
// либо записан весь пакет, либо ничего (битые строки транзакцию не роняют —
// их отсеял парсер, они просто возвращаются в Errors).
func (s *Service) Bulk(ctx context.Context, text string) (BulkResult, error) {
	parsed := Parse(text)
	created, merged, err := s.SaveCards(ctx, parsed.Cards)
	if err != nil {
		return BulkResult{}, fmt.Errorf("words bulk: %w", err)
	}
	return BulkResult{Created: created, Merged: merged, Errors: parsed.Errors}, nil
}

// SaveCards сохраняет уже разобранные карточки (Parse / ParseRows) одной транзакцией,
// сливая с существующими словами. Используется и вводом, и импортом CSV/JSON.
func (s *Service) SaveCards(ctx context.Context, cards []Card) (created, merged int, err error) {
	if len(cards) == 0 {
		return 0, 0, nil
	}
	now := s.clock.Now()
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		groups := newGroupResolver(q, now)
		for _, c := range cards {
			id, isNew, err := upsertWord(ctx, q, c, now)
			if err != nil {
				return err
			}
			if isNew {
				created++
			} else {
				merged++
			}
			if err := appendExamples(ctx, q, id, c.Examples); err != nil {
				return err
			}
			if err := groups.attach(ctx, id, c.Groups); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("save cards: %w", err)
	}

	// Публикуем после коммита: подписчики не должны увидеть слова, которых в итоге нет.
	s.bus.Publish(eventbus.WordsAdded{Created: created, Merged: merged, At: now})
	return created, merged, nil
}

// upsertWord создаёт слово или находит существующий дубликат (§4).
// Два запроса вместо `ON CONFLICT DO UPDATE ... RETURNING`: DO NOTHING не трогает
// существующую строку (term сохраняет исходный регистр), а «создано/слито» видно по ErrNoRows.
func upsertWord(ctx context.Context, q *store.Queries, c Card, now time.Time) (id int64, created bool, err error) {
	id, err = q.InsertWord(ctx, store.InsertWordParams{
		Term:        c.Term,
		Forms:       nonNil(c.Forms),
		Translation: c.Translation,
		Now:         now,
	})
	switch {
	case err == nil:
		return id, true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return 0, false, fmt.Errorf("insert word %q: %w", c.Term, err)
	}

	id, err = q.GetWordIDByKey(ctx, store.GetWordIDByKeyParams{Term: c.Term, Translation: c.Translation})
	if err != nil {
		return 0, false, fmt.Errorf("get word %q: %w", c.Term, err)
	}
	return id, false, nil
}

// appendExamples дописывает примеры в конец списка слова. Повторы (UNIQUE (word_id, sentence))
// молча пропускаются и позицию не занимают — дыр в position не будет.
func appendExamples(ctx context.Context, q *store.Queries, wordID int64, examples []Example) error {
	if len(examples) == 0 {
		return nil
	}
	pos, err := q.NextExamplePosition(ctx, wordID)
	if err != nil {
		return fmt.Errorf("next example position: %w", err)
	}
	for _, ex := range examples {
		n, err := q.InsertExample(ctx, store.InsertExampleParams{WordID: wordID, Position: pos, Sentence: ex.Text})
		if err != nil {
			return fmt.Errorf("insert example: %w", err)
		}
		pos += int16(n)
	}
	return nil
}

// groupResolver — get-or-create групп с кэшем на время одной транзакции:
// в bulk одна и та же группа обычно встречается у десятков слов.
type groupResolver struct {
	q   *store.Queries
	now time.Time
	ids map[string]int64
}

func newGroupResolver(q *store.Queries, now time.Time) *groupResolver {
	return &groupResolver{q: q, now: now, ids: make(map[string]int64)}
}

func (g *groupResolver) id(ctx context.Context, name string) (int64, error) {
	if id, ok := g.ids[name]; ok {
		return id, nil
	}
	id, err := g.q.InsertGroup(ctx, store.InsertGroupParams{Name: name, Now: g.now})
	if errors.Is(err, pgx.ErrNoRows) { // уже есть
		id, err = g.q.GetGroupIDByName(ctx, name)
	}
	if err != nil {
		return 0, fmt.Errorf("group %q: %w", name, err)
	}
	g.ids[name] = id
	return id, nil
}

func (g *groupResolver) attach(ctx context.Context, wordID int64, names []string) error {
	for _, name := range names {
		gid, err := g.id(ctx, name)
		if err != nil {
			return err
		}
		if err := g.q.AddWordGroup(ctx, store.AddWordGroupParams{WordID: wordID, GroupID: gid}); err != nil {
			return fmt.Errorf("add word group: %w", err)
		}
	}
	return nil
}

// ListFilter — необязательные фильтры GET /api/words; nil — без фильтра.
type ListFilter struct {
	Level   *int16
	GroupID *int64
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]Word, error) {
	q := store.New(s.pool)
	rows, err := q.ListWords(ctx, store.ListWordsParams{Level: f.Level, GroupID: f.GroupID})
	if err != nil {
		return nil, fmt.Errorf("words list: %w", err)
	}
	words, err := withDetails(ctx, q, rows)
	if err != nil {
		return nil, fmt.Errorf("words list: %w", err)
	}
	return words, nil
}

func (s *Service) Get(ctx context.Context, id int64) (Word, error) {
	q := store.New(s.pool)
	row, err := q.GetWord(ctx, id)
	if err != nil {
		return Word{}, fmt.Errorf("words get: %w", notFound(err))
	}
	words, err := withDetails(ctx, q, []store.Word{row})
	if err != nil {
		return Word{}, fmt.Errorf("words get: %w", err)
	}
	return words[0], nil
}

// withDetails подтягивает примеры и группы для всех слов двумя запросами (без N+1).
func withDetails(ctx context.Context, q *store.Queries, rows []store.Word) ([]Word, error) {
	words := make([]Word, len(rows))
	ids := make([]int64, len(rows))
	byID := make(map[int64]*Word, len(rows))
	for i, r := range rows {
		words[i] = Word{
			ID:             r.ID,
			Term:           r.Term,
			Forms:          r.Forms,
			Translation:    r.Translation,
			Level:          r.Level,
			Streak:         r.Streak,
			NextReviewAt:   r.NextReviewAt,
			LastReviewedAt: r.LastReviewedAt,
			CreatedAt:      r.CreatedAt,
		}
		ids[i] = r.ID
		byID[r.ID] = &words[i]
	}
	if len(rows) == 0 {
		return words, nil
	}

	examples, err := q.ListExamplesByWordIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list examples: %w", err)
	}
	for _, e := range examples {
		w := byID[e.WordID]
		w.Examples = append(w.Examples, exampleFromDB(e.Sentence))
	}

	groups, err := q.ListGroupsByWordIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list word groups: %w", err)
	}
	for _, g := range groups {
		w := byID[g.WordID]
		w.Groups = append(w.Groups, GroupRef{ID: g.ID, Name: g.Name})
	}
	return words, nil
}

// exampleFromDB восстанавливает предложение и спаны из сохранённой разметки.
// В БД попадает только то, что прошло ParseExample, так что ошибки быть не должно;
// если всё-таки есть (руками поправили БД) — отдаём текст без выделения, а не 500.
func exampleFromDB(markup string) Example {
	ex, code := ParseExample(markup)
	if code != "" {
		return Example{Text: markup, Sentence: markup}
	}
	return ex
}

// UpdateParams — тело PATCH /api/words/:id. nil — поле не меняется.
// Examples и Groups заменяют текущие целиком; пустой список — очистить.
type UpdateParams struct {
	Term        *string   `json:"term"`
	Translation *string   `json:"translation"`
	Examples    *[]string `json:"examples"` // разметка, как в §4: "I [go] home"
	Groups      *[]string `json:"groups"`
}

// Update меняет текст слова, примеры и группы. SRS-поля (level, streak, даты) не трогает.
func (s *Service) Update(ctx context.Context, id int64, p UpdateParams) (Word, error) {
	upd, err := validateUpdate(p)
	if err != nil {
		return Word{}, fmt.Errorf("words update: %w", err)
	}

	now := s.clock.Now()
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		w, err := q.GetWord(ctx, id)
		if err != nil {
			return notFound(err)
		}

		if upd.term != nil || upd.translation != nil {
			params := store.UpdateWordTextParams{ID: id, Term: w.Term, Forms: w.Forms, Translation: w.Translation}
			if upd.term != nil {
				params.Term, params.Forms = *upd.term, nonNil(upd.forms)
			}
			if upd.translation != nil {
				params.Translation = *upd.translation
			}
			if err := q.UpdateWordText(ctx, params); err != nil {
				if db.IsUniqueViolation(err) {
					return httpx.Conflict("duplicate_word", "word with this term and translation already exists")
				}
				return fmt.Errorf("update word: %w", err)
			}
		}

		if upd.examples != nil {
			if err := q.DeleteExamples(ctx, id); err != nil {
				return fmt.Errorf("delete examples: %w", err)
			}
			if err := appendExamples(ctx, q, id, upd.examples); err != nil {
				return err
			}
		}

		if upd.groups != nil {
			if err := q.DeleteWordGroups(ctx, id); err != nil {
				return fmt.Errorf("delete word groups: %w", err)
			}
			if err := newGroupResolver(q, now).attach(ctx, id, upd.groups); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Word{}, fmt.Errorf("words update: %w", err)
	}
	return s.Get(ctx, id)
}

// validatedUpdate — UpdateParams после нормализации по правилам §4.
// examples/groups: nil — не менять, пустой не-nil слайс — очистить.
type validatedUpdate struct {
	term        *string
	forms       []string
	translation *string
	examples    []Example
	groups      []string
}

// validateUpdate проверяет поля до транзакции теми же функциями, что и парсер,
// поэтому через PATCH нельзя сохранить то, что не прошло бы через ввод.
func validateUpdate(p UpdateParams) (validatedUpdate, error) {
	var v validatedUpdate
	if p.Term != nil {
		term, forms, code := normalizeTerm(*p.Term)
		if code != "" {
			return v, badField(code)
		}
		v.term, v.forms = &term, forms
	}
	if p.Translation != nil {
		t, code := normalizeTranslation(*p.Translation)
		if code != "" {
			return v, badField(code)
		}
		v.translation = &t
	}
	if p.Examples != nil {
		v.examples = []Example{}
		for _, raw := range *p.Examples {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			ex, code := ParseExample(raw)
			if code != "" {
				return v, badField(code)
			}
			v.examples = appendExample(v.examples, ex)
		}
	}
	if p.Groups != nil {
		// Через parseTags, чтобы "#Food" и "food, verbs" нормализовались ровно как во вводе.
		v.groups = nonNil(parseTags(strings.Join(*p.Groups, " ")))
	}
	return v, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteWord(ctx, id)
	if err != nil {
		return fmt.Errorf("words delete: %w", err)
	}
	if n == 0 {
		return httpx.NotFound("word not found")
	}
	return nil
}

// Unarchive возвращает слово из «Архива» (5) на «Мастер» (4) и делает его due сразу (§6).
func (s *Service) Unarchive(ctx context.Context, id int64) (Word, error) {
	q := store.New(s.pool)
	n, err := q.UnarchiveWord(ctx, store.UnarchiveWordParams{ID: id, Now: s.clock.Now()})
	if err != nil {
		return Word{}, fmt.Errorf("words unarchive: %w", err)
	}
	if n == 0 {
		// 0 строк: либо слова нет, либо оно не в архиве — различаем для понятной ошибки.
		if _, err := q.GetWord(ctx, id); err != nil {
			return Word{}, fmt.Errorf("words unarchive: %w", notFound(err))
		}
		return Word{}, httpx.Conflict("not_archived", "word is not archived")
	}
	return s.Get(ctx, id)
}

func badField(code ErrorCode) error {
	return httpx.BadRequest(string(code), "invalid field: "+string(code))
}

// notFound превращает ErrNoRows в 404, остальные ошибки пропускает как есть.
func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.NotFound("word not found")
	}
	return err
}

// nonNil нужен для pgx: nil-слайс уходит в БД как NULL, а forms — NOT NULL.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
