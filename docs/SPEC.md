# Lexis — спецификация

Единый источник правды по продукту и архитектуре. Всё ниже уже согласовано с владельцем проекта (Danik).
Если реализация расходится со спекой, сначала спроси, потом меняй спеку и код вместе.

---

## 1. Что это

Локальное **офлайн** веб-приложение для изучения английских слов. Один пользователь, работает на MacBook Air M4 (macOS).
Запуск: `python3 run.py` → Docker поднимает 2 контейнера → `http://localhost:8090` (`LEXIS_PORT`; 8080 на машине занят другим проектом).

Никаких внешних API: переводы и примеры пользователь готовит заранее (обычно через Claude) и вставляет текстом.

Будущее (НЕ реализуем сейчас, но архитектура должна позволять добавить модулем): планер дня, To-Do, трекер привычек.

---

## 2. Стек (утверждён)

| Слой | Технологии |
|---|---|
| Backend | Go 1.27, `chi`, `pgx/v5`, `sqlc`, `goose` (миграции, embed), `slog` |
| DB | PostgreSQL 17 |
| Frontend | Svelte 5 (runes) + Vite 8 + TypeScript 6, без роутера (табы + `#hash`), графики — свой SVG; шрифт `@fontsource-variable/inter` (в бандле, офлайн); тесты — vitest (чистая логика) |
| Infra | Docker + docker-compose: `app` (Go-бинарник со встроенным фронтом через `embed.FS`, distroless) + `db` |
| Launcher | `run.py` (stdlib only, кроссплатформенный, но целевая ОС — macOS) |

**Явно отказались:** Kafka, Redis, nginx, ORM, web-push, системные уведомления (агент, cron) — см. §9. Очереди/фон — горутины, каналы, таблицы в Postgres.

---

## 3. Архитектура backend

Модульный монолит. Каждый модуль — пакет `internal/<module>` со своими handler/service/repo, реализует интерфейс:

```go
// internal/app/module.go
type Module interface {
    Name() string
    Routes(r chi.Router)          // монтируется под /api/<name>
}
// опционально: Start(ctx) error — для модулей с фоновыми задачами (сейчас таких нет)
```

Новый модуль = новый пакет + одна строка регистрации в `cmd/server/main.go`.

Общая платформа `internal/platform/`:
- `db` — pgx pool, прогон миграций goose из embed при старте
- `clock` — **единственный** источник «сейчас» и «сегодня» в TZ пользователя (`LEXIS_TZ`, по умолчанию Europe/Moscow). Интерфейс, чтобы подменять в тестах. `time.Now()` в бизнес-коде запрещён.
- `eventbus` — in-process pub/sub на каналах (`SessionFinished`, `WordLevelChanged`, `WordsAdded`); на него подписан `achievements` (`stats` считает всё на лету из БД и подписка ему не нужна)
- `httpx` — JSON-ответы, единый формат ошибки `{"error": {"code": "...", "message": "..."}}`, middleware (recover, request log)

Бинарник:
- `lexis` — сервер
- `lexis healthcheck` — GET `127.0.0.1:8080/healthz`, exit 0/1 (нужно для distroless healthcheck в compose)

ENV: `LEXIS_HTTP_ADDR`, `LEXIS_DB_DSN`, `LEXIS_TZ`.

### Структура

```
lexis/
├── run.py  Dockerfile  docker-compose.yml  .env.example  CLAUDE.md
├── docs/SPEC.md
├── backend/
│   ├── go.mod  sqlc.yaml
│   ├── cmd/server/main.go
│   ├── migrations/0001_init.sql
│   ├── queries/*.sql
│   ├── web/embed.go              // //go:embed all:dist
│   └── internal/
│       ├── app/  config/
│       ├── platform/{db,clock,eventbus,httpx}/
│       ├── words/                // parser.go + parser_test.go, CRUD
│       ├── groups/               // список, статус дня, аналитика
│       ├── training/             // сессии, srs.go, queue.go, стратегии
│       ├── stats/  achievements/
│       ├── importexport/  settings/
└── frontend/
    └── src/
        ├── App.svelte  app.css  main.ts
        ├── lib/{api,i18n,state,anim,components}/
        └── tabs/{AddWords,Groups,Training,Stats,Settings}/
```

---

## 4. Формат ввода слов

Одна строка — одна карточка:

```
term || translation || example1 <> example2 || #group1 #group2
```

```
apple || яблоко || I eat an apple every day || #существительные #еда
run || бежать || I [run] in the morning <> They [run] fast || #глаголы #спорт
go_went_gone || идти || I [go] home <> I [went] home <> I have [gone] home || #глаголы
```

### Правила парсера (явная разметка, без эвристик — решено 2026-11-02)
Реализация: `words.Parse(text) ParseResult` в `internal/words/parser.go` (чистая функция).
- Пробелы вокруг `||` любые. Поля 3 и 4 необязательны (`term || translation` — валидно, пустое поле примеров/тегов — тоже). Больше 4 полей — ошибка.
- term и translation: trim, повторные пробелы внутри схлопываются в один.
- Теги с `#` или без, разделитель — пробел или запятая. Нормализация: lower-case, trim, `#` убирается при хранении; одиночный `#` пропускается, повторы убираются (порядок первого появления).
- Пустые строки и строки, начинающиеся с `//`, пропускаются. Номер строки — номер в исходном тексте (1-based, с учётом пропущенных).
- **Несколько примеров:** разделитель `<>` (просто split). Пустые куски пропускаются. `;` и `,` — обычный текст.
- **Выделение:** выделено ровно то, что в `[ ]`; фрагментов в предложении может быть сколько угодно. Пробелы внутри скобок обрезаются: `[ go ]` → `[go]`. Без `[ ]` — без выделения, автопоиска term нет.
  Ошибки: незакрытая/лишняя/вложенная скобка, пустые `[ ]`.
- **Хранение примера:** в `word_examples.sentence` лежит нормализованная разметка (`I [go] home`). Предложение без скобок и спаны (руны, `[start, end)`) восстанавливает `words.ParseExample` при чтении и отдаёт API.
- **Три формы глагола:** term с `_` — это формы: `go_went_gone` → `forms = [go, went, gone]`, `term = "go_went_gone"` (части trim'аются). Частей не 3 или есть пустая — ошибка строки. Дефис — всегда часть слова (`well-known`).
  `form_index` не храним: в стратегии `context` фронт сравнивает выделенные слова с формами (case-insensitive) и показывает совпавшую форму, если ни одна не совпала — все три.
- **Дубликаты:** то же `term` + `translation` (case-insensitive) → карточку не создаём заново, добавляем новые группы и новые примеры (пример = уникальная разметка).
  Внутри одного текста сливает **парсер** (превью честное; карточка остаётся на номере первой строки), с уже существующими в БД — **сервис** (`ON CONFLICT`).
- Ошибочные строки не роняют весь импорт: возвращаются в превью с номером строки, исходным текстом и `code` (snake_case для i18n): `too_many_fields`, `missing_translation`, `empty_term`, `empty_translation`, `invalid_verb_forms`, `unbalanced_brackets`, `empty_highlight`. Остальное сохраняется.

`POST /api/words/parse` — только превью (ничего не пишет). `POST /api/words/bulk` — сохраняет.
UI: textarea → живое превью карточек справа, ошибки красным → кнопка «Создать карточки» → индикатор «Создание карточек...».

### Импорт/экспорт
- CSV: `term,translation,example,tags` (tags через пробел или `;`); гибко к порядку колонок по заголовку.
- JSON: массив объектов той же формы. Экспорт — полный бэкап (слова, группы, уровни, история).

**Уточнения (решено 2026-11-02, `internal/importexport`):**
- Формат импорта определяется **по содержимому**: `[` → JSON-список слов, `{` → бэкап, иначе CSV. UTF-8 BOM (Excel) допускается.
- **CSV**: заголовок обязателен, колонки без учёта регистра в любом порядке: `term`, `translation` (обязательные), `example`|`examples`, `tags`|`groups`;
  лишние колонки игнорируются. Примеры — через `<>`, теги — через пробел, `,` или `;`. Ошибка строки → `errors[]` с номером строки файла.
- **JSON-список**: `[{"term","translation","example"|"examples","tags"}]` — строки, как в CSV; лишние поля игнорируются; `line` = номер элемента с 1.
- Список слов идёт через те же правила §4 и `words.SaveCards`, что и ввод: дубликаты сливаются (группы и примеры добавляются).
- **Экспорт CSV** — тот же формат (импортируется обратно), без уровней и истории.
- **Бэкап** (`version: 1`): `{version, exportedAt, settings, groups, words (с examples и groupIds), sessions (с reviews), achievements}`.
  id внутри файла — только для ссылок. Импорт бэкапа **заменяет все данные, включая settings**, одной транзакцией;
  битый файл (неизвестное поле, висячая ссылка, нарушение CHECK) → 400, база не меняется.
- **Очистка базы** удаляет всё, кроме settings; id начинаются заново.

---

## 5. Группы

- Слово может быть в нескольких группах (many-to-many).
- Статус на сегодня: **«Повторена» / «Не повторена»**. Вычисляется на лету, никакого cron-сброса в 00:00:
  повторена ⇔ (есть завершённая `training_session` по этой группе — обычная **или** free — с `finished_at` в сегодняшней дате, TZ пользователя)
  **ИЛИ** (в группе нет карточек на сегодня: due-счётчик = 0).
- ✅ Группа без карточек на сегодня считается «повторённой» автоматически. Пустая группа (0 слов) — тоже.
- ✅ Свободная тренировка тоже засчитывает группу как «повторённую» (при этом её due-карточки остаются due и дальше видны в due-счётчике).

Аналитика группы:
- график продуктивности по дням (доля «Знаю» в сессиях);
- распределение слов по 5 уровням;
- сколько раз группа пройдена за неделю/месяц;
- % слов на уровнях «Мастер» + «Архив».

**Уточнения (решено 2026-11-02, `internal/groups`):**
- due-счётчик = слова группы с `level < 5` и `next_review_at < начало завтрашнего дня` (новые тоже входят — у них `next_review_at = created_at`).
- «Пройдена за неделю/месяц» = число **завершённых** сессий группы (обычных и free) в **календарной** неделе (с понедельника) / месяце (с 1-го), в TZ пользователя, включая сегодня.
- График продуктивности: `?range=week|month` (7 / 30 последних дней, включая сегодня; по умолчанию month). Точка на каждый день:
  `goodShare = Σgood / Σtotal` по завершённым сессиям дня (обычные + free; total/good — по первым ответам), `null` — ответов не было.
  Взвешено по ответам, а не среднее по сессиям. По дням раскладывается в Go (`groups.Productivity`), не в SQL.
- % «Мастер» + «Архив» — целое, округление половины вверх; пустая группа — 0.
- Группы создаются только из тегов при вводе; управление — переименование (имя нормализуется как тег, `words.NormalizeGroupName`), цвет (`#rrggbb` или `null`), удаление (слова и история сессий остаются).

---

## 6. SRS — 5 уровней

Кнопки ответа: **Знаю** (`good`), **Затрудняюсь** (`hard`), **Не знаю** (`again`). Плюс «Показать ответ».

| Уровень | Переход выше | Интервал повтора |
|---|---|---|
| 1 Новое | любой первый ответ → 2 | — |
| 2 Изучаю | 3 «Знаю» подряд, минимум в 2 разных дня → 3 | 1 день |
| 3 Знаю | ещё 3 «Знаю» подряд с интервалом ≥ 3 дней → 4 | 3 дня |
| 4 Мастер | 2 «Знаю» подряд с интервалом ≥ 7 дней → 5 | 7 дней |
| 5 Архив | — | не повторяется; можно вернуть вручную (`unarchive` → уровень 4, серия 0, due сразу) |

- «Не знаю» → уровень −1 (не ниже 2 «Изучаю»), серия = 0, показать завтра.
- «Затрудняюсь» → серия = 0, уровень не меняется, показать завтра.
- **Уточнения (решено 2026-11-02, `training.Apply`):**
  - Уровень 1: любой первый ответ → 2, серия = 0 (знакомство в серию не идёт); показать завтра.
  - Серия «Знаю подряд» считается с шагом: «Знаю» засчитывается, только если с прошлого **засчитанного** «Знаю»
    (`last_good_at`) прошло ≥ N календарных дней в TZ пользователя — ур.2: ≥ 1, ур.3: ≥ 3, ур.4: ≥ 7.
    Первое «Знаю» серии (streak = 0) засчитывается всегда. Незасчитанное «Знаю» серию не растит, но и не сбрасывает.
    Для ур.2 это строже буквального «в 2 разных дня» (нужно 3 разных дня) — сознательно, одно правило на все уровни.
  - При повышении уровня серия = 0 («ещё N Знаю» на новом уровне).
  - `next_review_at`: «Знаю» → now + интервал нового уровня (1/3/7 дней, календарных — `AddDate`); «Затрудняюсь»/«Не знаю» → now + 1 день.
- Уровень меняется только по **первому** ответу на слово в сессии; повторы ошибок внутри сессии — только закрепление.
- «Карточки на сегодня» = слова с `next_review_at <= конец сегодняшнего дня`, уровень < 5.
- Вся логика — чистые функции в `training/srs.go`, полное table-driven покрытие тестами.

---

## 7. Тренировка

Перед стартом: выбрать **группу** и **стратегию**.

| Стратегия | Показываем | Пользователь отвечает |
|---|---|---|
| `en_ru` | слово на английском | перевод |
| `ru_en` | перевод | английское слово |
| `context` | предложение с выделенным `[словом]` | перевод предложения + слова |

Для 3 форм глагола:
- `en_ru`: `go – went – gone` → «идти»
- `ru_en`: «идти» → все три формы, засчитывается целиком
- `context`: каждое предложение — отдельная подкарточка; в ответе — форма(ы), совпавшие с выделенным в `[ ]`, если ни одна не совпала — все три (см. §4)

Самооценка: пользователь может ввести ответ в поле (не проверяется строго), жмёт «Показать ответ», затем Знаю / Затрудняюсь / Не знаю.
UI: 3D-flip карточки, прогресс-бар, экран результатов (good/hard/again, время, изменения уровней).

### Порядок карточек в сессии (`training/queue.go`)
1. Корзины по срочности: просроченные → сегодняшние → новые. Внутри корзины — shuffle.
2. Лимит новых слов за сессию: 10 (настраивается в Settings: ключ `new_words_limit` в `settings`, JSON-число; нет/мусор → 10). Лимит берётся после shuffle.
3. «Не знаю» → карточка возвращается через 3–5 позиций, пока не получит Знаю/Затрудняюсь. Очередь держит клиент: `answer` отдаёт `requeueAt` (смещение 3..5) на каждый `again`, в т.ч. повторный.
4. Разнесение похожих: формы одного глагола и слова с одинаковым переводом не идут подряд.
   Похожесть = одинаковый перевод без учёта регистра (подкарточки глагола туда попадают сами). Разносим внутри корзины, корзины не перемешиваем; если невозможно (в корзине один глагол) — best effort.
5. Стратегия `context`: глагол с 3 формами → подкарточка на каждый пример; обычное слово → одна карточка со случайным примером; слово без примеров → карточка без примера (фронт показывает как `en_ru`).
6. Если карточек нет — сессия не создаётся (`sessionId: null, cards: []`).
7. Состояние сессии — только в БД; «первый ответ на слово в сессии» = нет строки в `reviews` по `(session_id, word_id)`.

### Свободная тренировка (решено: да)
- Флаг `free=true` в `POST /api/training/start` и в `training_sessions.free`.
- Карточки: **все** слова группы (включая уровень 5 «Архив»), один общий shuffle без корзин и без лимита новых; разнесение похожих и requeue «Не знаю» работают как обычно.
- Ответы пишутся в `reviews` (`level_before = level_after`), SRS **не применяется**: `level`, `streak`, `next_review_at`, `last_reviewed_at`, `last_good_at` не меняются, событие `WordLevelChanged` не публикуется.
- Засчитывает группу как «повторённую» сегодня (§5); идёт в статистику, heatmap и серию дней (`SessionFinished{Free: true}` публикуется).

---

## 8. Статистика (вкладка)

- Календарь активности (heatmap), серия дней.
- За день: сколько групп пройдено, сколько слов разобрано по группам-типам (#существительные, #глаголы…), время тренировок.
- Тренды неделя/месяц (линейные графики).
- Достижения/бейджи: серии дней (3/7/30), выучено слов (10/50/100/500 на уровне ≥ 4), первая группа в «Архиве» и т.п. Разблокировка — через eventbus.

**Уточнения (решено 2026-11-02, `internal/stats`, `internal/achievements`):**
- **Активный день** = день (TZ пользователя) с хотя бы одной **завершённой** сессией, обычной или free. Серия: `current` не рвётся, пока идёт сегодняшний день
  (если сегодня ещё не тренировался — считается до вчера), `best` — лучшая за всю историю (`stats.Streaks`).
- Heatmap/тренды/«за день» считаются из `training_sessions` (только завершённые): `words` = Σ первых ответов по сессиям дня, `seconds` = Σ `duration_sec`,
  `goodShare` = good / words (null — ответов не было). Все дни окна присутствуют в ответе, включая пустые.
- «По группам-типам»: уникальные разобранные слова дня по **всем** группам слова (слово с #глаголы #спорт идёт в обе); сумма по группам ≠ итогу.
- «Групп пройдено за день» = число разных групп у завершённых сессий дня (удалённые группы не считаются).
- **Достижения** (порядок показа, тексты — i18n по коду): `first_session`, `streak_3`, `streak_7`, `streak_30` (по **лучшей** серии),
  `learned_10`, `learned_50`, `learned_100`, `learned_500` (слов на уровне ≥ 4 сейчас), `first_archived_group` (непустая группа, все слова в «Архиве»).
  Полученное не отзывается, даже если метрика упала.
- Проверка — идемпотентная функция «снимок БД → `achievements.Evaluate` → `INSERT ON CONFLICT DO NOTHING`». Зовётся подписчиком на `SessionFinished` /
  `WordLevelChanged` **и** при `GET /api/achievements` (подписчик асинхронный — так фронт сразу после finish видит свежие ачивки).
- Ping активности (`activity_days`) убран вместе с уведомлениями (§9): «активный день» считается по сессиям.

---

## 9. Уведомления — убраны (решено 2026-11-02)

Системные уведомления (cron 9:00/19:00 + хост-агент `notifier.py` с `terminal-notifier`) **полностью удалены**:
лишняя сложность для локального приложения. Вместе с ними ушли таблицы `notifications`, `activity_days`,
`POST /api/activity/ping`, команды `run.py install | uninstall | notify-test` и зависимость `robfig/cron`.
Напоминание «что повторить сегодня» — только внутри приложения (статус групп, due-счётчики).

---

## 10. БД

Полная схема: `backend/migrations/0001_init.sql` (до первого релиза правим её на месте). Кратко:

```sql
words            (id, term, forms TEXT[], translation, level SMALLINT 1..5, streak SMALLINT,
                  next_review_at TIMESTAMPTZ NOT NULL, last_reviewed_at, last_good_at, created_at)
                  UNIQUE (lower(term), lower(translation)); у нового слова next_review_at = created_at
                  CHECK (cardinality(forms) IN (0, 3))
word_examples    (id, word_id, position, sentence,  -- sentence с разметкой "I [go] home", спаны считаются при чтении
                  UNIQUE (word_id, sentence))
groups           (id, name UNIQUE, color, created_at)
word_groups      (word_id, group_id, PRIMARY KEY (word_id, group_id))
training_sessions(id, group_id → ON DELETE SET NULL, strategy, free BOOL, started_at, finished_at,
                  total, good, hard, again, duration_sec)
reviews          (id, session_id, word_id, answer, first_in_session BOOL,
                  level_before, level_after, answered_at)
achievements     (code PK, unlocked_at)
settings         (key PK, value JSONB)
```

Время в БД всегда приходит параметром из `platform/clock` (без `DEFAULT now()`); «сегодня» в SQL — полуинтервал `[startOfDay, startOfNextDay)`, посчитанный в Go.

---

## 11. API

```
GET  /healthz
POST /api/words/parse            {text} → {cards[], errors[]}
POST /api/words/bulk             {text} → {created, merged, errors[]}
GET  /api/words?group=<id>&level=<1..5>  → {words: Word[]}   (оба фильтра необязательны; новые сверху)
PATCH /api/words/:id             {term?, translation?, examples?: string[], groups?: string[]} → Word
                                 (examples/groups заменяют целиком, [] — очистить; SRS не трогает; 409 duplicate_word)
DELETE /api/words/:id            → 204 (примеры и связи с группами — каскадом, группы остаются)
POST /api/words/:id/unarchive    → Word (только из level 5, иначе 409 not_archived)
```
`Word = {id, term, forms?, translation, level, streak, nextReviewAt, lastReviewedAt, createdAt, examples?: [{text, sentence, highlights?: [{start,end}]}], groups?: [{id, name}]}`.
Ошибки валидации — 400 с кодами парсера (§4) либо `invalid_id`, `invalid_level`, `invalid_group`, `invalid_json`.
```
GET  /api/groups                 → {groups: Group[]}   (по имени)
                                 Group = {id, name, color, createdAt, wordCount, due, repeatedToday, levels: [ур.1..ур.5]}
GET  /api/groups/:id?range=week|month → {group: Group, analytics: {masteredPercent, sessionsThisWeek, sessionsThisMonth,
                                 range, productivity: [{day: "YYYY-MM-DD", sessions, goodShare | null}]}, words: Word[]}
                                 400 invalid_range, 404
PATCH /api/groups/:id            {name?, color?: "#rrggbb" | null} → Group   (нет поля — не менять, null — очистить)
                                 400 invalid_group_name | invalid_color, 404, 409 duplicate_group
DELETE /api/groups/:id           → 204 (связи со словами — каскадом, training_sessions.group_id → NULL)
POST /api/training/start         {groupId, strategy, free?} → {sessionId | null, cards[]}
                                 Card = {wordId, term, forms?, translation, level, example?: {text, sentence, highlights?}}
                                 400 invalid_group | invalid_strategy, 404 группы нет
POST /api/training/:id/answer    {wordId, answer} → {levelBefore, levelAfter, firstInSession, requeueAt?}
                                 400 invalid_answer | invalid_word | word_not_in_session, 404, 409 session_finished
POST /api/training/:id/finish    → {sessionId, free, total, good, hard, again, durationSec, levelChanges: [{wordId, term, levelBefore, levelAfter}]}
                                 (итоги по первым ответам; 409 session_finished при повторе)
GET  /api/stats/overview?day=YYYY-MM-DD   (по умолчанию сегодня)
                                 → {day: DayStats, groupsTrained, byGroup: [{id, name, words}], streak: {current, best},
                                    totals: {words, learned, levels: [ур.1..ур.5]}}
GET  /api/stats/calendar?from=&to=        (YYYY-MM-DD включительно; по умолчанию последние 365 дней; ≤ 366 дней)
                                 → {from, to, days: DayStats[]}
GET  /api/stats/trends?range=week|month   (7 / 30 дней, включая сегодня; по умолчанию week) → {range, days: DayStats[]}
                                 DayStats = {day, sessions, words, good, hard, again, seconds, goodShare | null}
                                 400 invalid_date | invalid_range
GET  /api/achievements           → {achievements: [{code, progress, target, unlockedAt | null}]}
GET  /api/settings               → {language: "ru"|"en", newWordsLimit: 0..100}
PUT  /api/settings               {language?, newWordsLimit?} → Settings   (400 invalid_language | invalid_new_words_limit)
POST /api/data/import            multipart, поле "file" (≤ 20 МБ), формат — по содержимому (§4)
                                 → {kind: "words", created, merged, errors[]} | {kind: "backup", restored: {groups, words, sessions, reviews, achievements}}
                                 400 missing_file | empty_file | invalid_multipart | invalid_csv | csv_missing_columns | invalid_json
                                     | invalid_backup | unsupported_backup_version; 413 file_too_large
GET  /api/data/export?format=json|csv   (по умолчанию json — бэкап) → файл (Content-Disposition: lexis-backup-YYYY-MM-DD.json | lexis-words-…csv)
POST /api/data/reset             {"confirm": true} → 204   (400 confirm_required; подтверждение пользователя — на фронте)
```
Префикс `/api/data` вместо `/api/import`, `/api/export`, `/api/admin`: модуль монтируется под `/api/<Name>` (§3), а import/export/reset — один модуль.
Неизвестный `/api/...` → 404 `not_found` JSON, не тот метод → 405 `method_not_allowed`; `GET /healthz` пингует БД (503 `db_unavailable`).
```
```

---

## 12. UI / дизайн

- **Стиль:** минимализм + упрощённый neo-brutalism. Без macOS-окон с кружками (отказались).
- **Палитра:** белый фон; зелёный — успех/прогресс; красный — ошибки/опасные действия; текст чёрный/тёмно-серый; обводки тонкие чёрные 2–3px.
- Карточки: белые с чёрной обводкой или лёгкой жёсткой тенью. Кнопки плоские, чёткие границы, hover — смена цвета/лёгкий сдвиг.
- Шрифт: Inter (локально в бандле, офлайн). Иконки линейные минималистичные (inline SVG).
- ЦА 16–26 лет.
- **Навигация:** табы — Карточки (ввод) · Группы · Тренировка · Статистика · Настройки. Основные разделы без скролла страницы; внутри разделов скролл для длинных списков ок.
- **Анимации:** простые — fade, slide, scale; 200–300ms; ease-out или лёгкий spring. Переключение табов — плавное появление; карточка — 3D flip; добавление слова — fade-in; смена уровня — анимация прогресс-бара.
- **i18n:** RU / EN, переключатель в шапке. Все тексты UI — через словари `lib/i18n/{ru,en}.json`.

### Settings
Язык; лимит новых слов за сессию; импорт/экспорт CSV/JSON; очистка базы.
Ключи в `settings` (JSONB): `language` (`"ru"` | `"en"`, по умолчанию `"ru"`), `new_words_limit` (число 0..100, по умолчанию 10; читает training).

---

## 13. План работ

- [x] Шаг 1 — консультация, стек
- [x] Шаг 2 — структура
- [x] Шаг 3 — docker-compose, Dockerfile, run.py (notifier позже удалён, §9)
- [ ] Шаг 4 — Backend:
  - [x] 4.1 `go.mod`, миграция `0001_init.sql`, `platform/{clock,db,eventbus,httpx}`, `config`, `app` (2026-11-02)
  - [x] 4.2 words (2026-11-02):
    - [x] парсер `words.Parse` / `ParseExample` + table-driven тесты; формат §4 переделан на явную разметку (`<>`, `_`, `[ ]`),
      из `word_examples` убраны `highlight_start/end`, `form_index`; в `words` добавлен `CHECK (cardinality(forms) IN (0, 3))`
    - [x] `queries/words.sql` + блок в `sqlc.yaml` → `internal/words/store` (+ глобальные overrides `timestamptz → time.Time`)
    - [x] `Service`: Bulk (одна транзакция; insert `ON CONFLICT DO NOTHING` → id дубликата; примеры с продолжением `position`; группы get-or-create с кэшем; `WordsAdded` после коммита), List/Get, Update, Delete, Unarchive
    - [x] хендлеры + `words.New(pool, clock, bus)` (`app.Module`); unit-тесты хендлеров/валидации, интеграционные тесты сервиса и HTTP-роутов
    - Регистрация в `cmd/server/main.go` — в 4.8 (там же собирается роутер)
  - [x] 4.3 training (2026-11-02):
    - [x] `training.Apply` (SRS §6) и `training.Build` / `RequeueOffset` (очередь §7) — чистые функции + table-driven/property тесты
    - [x] `queries/training.sql` → `internal/training/store`; `Service`: Start / Answer / Finish (транзакции, `FOR UPDATE` на сессию и слово), события `WordLevelChanged`, `SessionFinished`
    - [x] хендлеры + `training.New(pool, clock, bus)`; unit-тесты хендлеров, интеграционные тесты сервиса и HTTP
    - `eventbus.Publisher` вынесен в platform (words и training принимают его); регистрация в `main.go` — в 4.8
  - [x] 4.4 groups (2026-11-02):
    - [x] `groups/analytics.go` — чистые функции (WeekStart/MonthStart, Productivity, Levels.MasteredPercent, RepeatedToday) + table-driven тесты
    - [x] `queries/groups.sql` → `internal/groups/store` (одна агрегирующая выборка на список/одну группу, без N+1)
    - [x] `Service`: List / Get (слова — через `groups.WordLister`, его реализует `words.Service`) / Update / Delete; хендлеры, `groups.New(pool, clock, wordLister)`
    - [x] unit-тесты хендлеров и `OptionalString`, интеграционные тесты сервиса и HTTP
    - `db.IsUniqueViolation` вынесен в platform; `words.NormalizeGroupName` экспортирован; регистрация в `main.go` — в 4.8
  - [x] 4.5 stats/achievements (2026-11-02):
    - [x] `stats/calc.go` (Streaks, BucketByDay, GroupsTrained) и `achievements/rules.go` (Evaluate) — чистые функции + table-driven тесты
    - [x] `queries/{stats,achievements}.sql` → `internal/{stats,achievements}/store`
    - [x] stats: Overview / Calendar / Trends + хендлеры; achievements: Check / List + подписчик (`achievements.New` подписывается на шину) + хендлер
    - [x] unit-тесты хендлеров, интеграционные тесты сервисов, HTTP и сквозной «Publish → подписчик → БД»
    - `eventbus.Subscriber` и `dbtest.Fixtures` вынесены в platform; регистрация в `main.go` — в 4.8
  - [x] 4.6 ~~notifications~~ — решено убрать (2026-11-02): удалены `notifier/`, команды агента в `run.py`, таблицы `notifications` и `activity_days`, `robfig/cron` (§9)
  - [x] 4.7 importexport/settings (2026-11-02):
    - [x] words: `ParseRows` + `SaveCards` (импорт идёт через те же правила и слияние, что и ввод), `;` как разделитель тегов
    - [x] settings: `Patch.Apply` (валидация) + Get/Update + хендлеры; unit + интеграционные тесты
    - [x] importexport: `DetectFormat`, `ParseCSV`, `ParseWordList`, `WriteCSV`, `Backup` + `validateBackup` (чистые, table-driven);
      Import / ExportCSV / ExportBackup (REPEATABLE READ) / Restore (одна транзакция, `COPY` для массовых таблиц) / Reset; хендлеры `/api/data/*`;
      интеграционные: импорт CSV/JSON, круговой бэкап «экспорт → восстановление → экспорт», откат битого бэкапа, reset, HTTP с multipart
  - [x] 4.8 (2026-11-02): `cmd/server` (main с graceful shutdown, `newRouter` с регистрацией модулей, `lexis healthcheck`), `web/embed.go`
    (статика из бинарника, SPA-fallback, кэш для `assets/`, 503 «фронт не собран»); unit-тесты + смоук-тест всего API через роутер.
    Проверено вживую: старт с миграциями, healthcheck 0/1, SIGTERM → exit 0. `docker compose build` — после появления `frontend/` (шаг 5).
- [x] Шаг 5 — Frontend:
  - [x] 5.1 каркас (2026-11-02): Vite + Svelte 5 + TS (strict, `noUncheckedIndexedAccess`), токены в `app.css`, API-клиент + типы §11
    (`lib/api`), i18n (`translate` + Intl.PluralRules, ключи типизированы из `ru.json`, тест «ru и en совпадают по ключам»),
    `lib/text.segments` (выделения по code points), хеш-роутинг (`lib/state/route`), тосты, компоненты Button/Card/Icon/ProgressBar/
    LevelBadge/Highlighted/ConfirmDialog/Toaster/Spinner, оболочка App (шапка, табы, RU/EN ↔ `/api/settings`), заглушки табов.
    Проверено: `npm run check` 0/0, vitest, `docker compose build` + `up` → фронт из бинарника, healthcheck `healthy`.
  - [x] 5.2 Карточки (2026-11-02): textarea (черновик в `state/draft` + localStorage — не теряется при смене таба/перезагрузке),
    живое превью `/parse` (debounce 300ms, устаревшие ответы отбрасывает `lib/async.latest`), ошибки красным с номером строки —
    клик выделяет строку в поле (`lib/lines.lineRange`), шпаргалка формата + «Вставить пример», «Создать N карточек» (⌘/Ctrl+Enter)
    → bulk → тост; **после сохранения в поле остаются только ошибочные строки** (`keepLines`), без ошибок — поле очищается.
    Проверено: check/vitest/build, Vite-прокси → Go (:8090) на `/parse` и `/bulk`.
  - [x] 5.3 Группы (2026-11-02): список (цвет, статус дня, due, слова, мини-полоска уровней) + детали в `#groups/<id>`
    (на узком окне — либо список, либо детали): переименование, палитра цвета (8 цветов + «без цвета»), удаление с подтверждением,
    «Тренировать» → `#training/<id>`; плитки (due, % Мастер+Архив, сессии за неделю/месяц), SVG-график продуктивности
    (`lib/chart.plot`, неделя/месяц; дни без ответов — без точки, линия соединяет дни с данными), `LevelBars`;
    слова: правка в диалоге (все поля, examples/groups целиком), удаление, «вернуть из архива». Общие `Modal` (на `<dialog>`) и
    `ConfirmDialog` поверх него; `lib/form` (splitTags/splitLines), `lib/dates` (YYYY-MM-DD → локальная полночь, не UTC).
  - [x] 5.4 Тренировка (2026-11-02): выбор группы (с due; `#training/<id>` из «Групп» — уже выбрана), стратегии
    (последняя запоминается в localStorage), free; пусто на сегодня → «Всё повторено» + кнопка свободной тренировки.
    Сессия: прогресс-бар, счётчик и таймер, 3D-flip (`FlipCard`, невидимая сторона `inert`), поле «твой ответ» (не проверяется,
    показывается на обороте), Enter/пробел — показать ответ, 1/2/3 — Знаю/Затрудняюсь/Не знаю; подсказка о смене уровня
    и о возврате карточки; «Завершить» досрочно с подтверждением; экран итогов (слов, время, good/hard/again, изменения уровней).
    Очередь — чистые функции `lib/training` (`answer` вставляет «Не знаю» через `requeueAt`; `done + queue = total` —
    прогресс не откатывается), `faces` — что на сторонах по стратегии (context: совпавшие с `[ ]` формы, иначе все три;
    без примера — как en_ru). Активная сессия — в `state/session.svelte.ts` (переживает смену таба, не перезагрузку).
  - [x] 5.5 Статистика (2026-11-02): плитки (серия текущая/лучшая, слов и время сегодня, групп сегодня, выучено ≥4),
    heatmap за год (`lib/heatmap`: недели-колонки Пн..Вс, интенсивность 0..4 относительно самого активного дня — любой день
    с занятием виден; подписи месяцев без наложений; клик/Enter по дню → сводка дня), сводка дня (ответы, время, «по группам»),
    тренды неделя/месяц (столбики слов + линия доли «Знаю», `chart.bars`/`plot`), уровни всех слов, достижения
    (прогресс/дата; неизвестный код с бэка — показываем код, `hasKey`). Общий `Segmented` (неделя/месяц) — и в «Группах».
  - [x] 5.6 Настройки (2026-11-02): язык (`Segmented`), лимит новых слов (0–100), импорт файла (CSV/JSON-список — сразу;
    JSON-бэкап — только после подтверждения «заменит все данные»; после восстановления перечитываются настройки/язык),
    экспорт ссылками (JSON-бэкап, CSV), очистка базы с подтверждением. Заглушка `Placeholder` удалена.
- [x] Шаг 6 — интеграция (2026-11-02): `python3 run.py up --build` → образ собран, `.env` создан из примера, app `healthy`
  на http://localhost:8090, фронт из бинарника, API отвечает; go/vitest/svelte-check зелёные. README — [README.md](../README.md): запуск, данные, формат ввода, горячие клавиши, команды разработки.

Все ❓ открытые вопросы закрыты (2026-11-02).

---

## 14. Конвенции бэкенда (как пользоваться platform)

Уже реализовано в 4.1. Новый код обязан этим пользоваться, а не изобретать своё.

- **Время.** `clock.Clock` (`Now()`, `Location()`); в тестах `clock.NewFixed(t)` + `Set`/`Advance`.
  Хелперы: `clock.StartOfDay(t)`, `clock.DayBounds(t) (start, end)` — полуинтервал для SQL `x >= start AND x < end`,
  `clock.Today(c)`, `clock.DaysBetween(a, b)` — разница **календарных** дат в TZ `a`.
  Время в БД всегда передаётся параметром (в схеме нет `DEFAULT now()`), `now()`/`CURRENT_DATE` в запросах не используем.
  ⚠️ `timestamptz` из pgx приходит не в TZ пользователя (UTC / `time.Local`). Перед календарными сравнениями приводить:
  `clock.DaysBetween(t.In(now.Location()), now)` — иначе 01:00 МСК окажется «вчера» (см. `training.Apply` и его тест).
- **Случайность.** В чистых функциях — параметром `*rand.Rand` (`math/rand/v2`), в сервисе — фабрика `newRand` на вызов
  (`*rand.Rand` не потокобезопасен), в тестах — фиксированный seed.
- **HTTP.** Хендлер — `func(w, r) error`, оборачивается `httpx.Handle(...)`. Тело — `httpx.Decode(w, r, &req)` (строгий: неизвестные поля → 400, лимит 1 МБ).
  Ответ — `httpx.JSON(w, status, v)` / `httpx.NoContent(w)`.
  Ошибки для клиента — `*httpx.Error`: `httpx.BadRequest(code, msg)`, `httpx.NotFound(msg)`, `httpx.Conflict(code, msg)`, `httpx.NewError(status, code, msg)`.
  `code` — snake_case, фронт по нему берёт текст из i18n. Любая другая ошибка → 500 `internal` без деталей (детали в лог). Обёртка `fmt.Errorf("op: %w", err)` не мешает — ищется через `errors.As`.
  Middleware: `httpx.Recover`, `httpx.RequestLog`.
- **События.** `eventbus.Bus`: `Subscribe(name, func(ctx, Event))`, `Publish(Event)`, `Close()`. События — типы из `eventbus/events.go`
  (`SessionFinished`, `WordLevelChanged`, `WordsAdded`), новые добавляются туда же. Подписчик разбирает их через type switch.
  Обработчик асинхронный; в тестах после `Close()` все события гарантированно обработаны.
- **Модуль.** `app.Module{Name(); Routes(chi.Router)}`, опционально `app.Starter{Start(ctx) error}`.
- **sqlc.** Код генерируется **по модулям**: в `sqlc.yaml` по блоку на модуль — `queries/<module>.sql` → `internal/<module>/store`
  (`engine: postgresql`, `schema: migrations`, `sql_package: pgx/v5`). Модуль может читать чужие таблицы своими запросами (groups считает due по words).
  Проверено: sqlc v1.31 парсит схему, таблица `groups` работает без кавычек.
  Маппинг типов общий (верхний `overrides` в `sqlc.yaml`): `timestamptz` → `time.Time` / `*time.Time`; в блоках модулей `emit_pointers_for_null_types: true`.
  ⚠️ pgx пишет nil-слайс как `NULL` — для `NOT NULL` массивов (`words.forms`) передавать `[]string{}`.
  «Upsert с id» делаем двумя запросами: `INSERT ... ON CONFLICT DO NOTHING RETURNING id` → при `pgx.ErrNoRows` `SELECT id` по ключу (см. `words.upsertWord`).
  Транзакции — `pgx.BeginFunc(ctx, pool, func(tx) error)` + `store.New(tx)`; unique violation → `db.IsUniqueViolation(err)` (код `23505`).
- **sqlc и nullable-параметры.** Параметр, сравниваемый с nullable-колонкой (`finished_at`, `group_id` у сессий), sqlc делает `*T`.
  Если NULL там не нужен — явный каст в запросе: `@from_at::timestamptz`, `@group_id::bigint`.
- **PATCH с «очистить».** Где `null` ≠ «поле не передано» — `groups.OptionalString{Set, Value}` (свой `UnmarshalJSON`); `*string` эти случаи не различает.
- **Межмодульные зависимости.** Модуль получает чужой сервис через узкий интерфейс в своём пакете (`groups.WordLister`), а не `*words.Service`.
  Сервисы без состояния (пул, часы, шина), так что в `main.go` можно создать отдельный экземпляр.
- **Сервис/ошибки.** Сервис сам возвращает `*httpx.Error` для клиентских ошибок (404/409/400), хендлер просто прокидывает `err`.
  Зависимость от шины — через узкий интерфейс `eventbus.Publisher` (его реализует `*eventbus.Bus`), в тестах — фейк.
  Подписчики принимают `eventbus.Subscriber` и подписываются в конструкторе модуля (до старта HTTP). Обработчик события должен быть
  идемпотентным и не терять ничего при ошибке (лог + «догонит следующая проверка»), т.к. вернуть ошибку из шины некуда.
- **Миграции.** `migrations/*.sql` (goose-формат, embed в `migrations.FS`), прогоняются при старте через `db.Migrate`.
- **Массовая вставка.** `:copyfrom` в sqlc (pgx `CopyFrom`) — для тысяч строк (восстановление бэкапа). Ошибка данных из файла
  (класс 23: CHECK/UNIQUE/NOT NULL/FK) → `db.IsIntegrityViolation(err)` → 400, а не 500.
- **Сборка приложения.** Модули регистрируются в `cmd/server/router.go` (`modules(...)`), там же `/healthz` и фронт (`web.Handler()`).

## 15. Тесты и окружение разработки

- Unit: `cd backend && go test ./... -race` — без БД.
- Интеграционные (с живым Postgres) — build-тег `integration` и **отдельная** `LEXIS_TEST_DSN` (тесты откатывают схему до нуля):
  ```
  docker run --rm -d --name lexis-testdb -p 127.0.0.1:5444:5432 \
    -e POSTGRES_USER=lexis -e POSTGRES_PASSWORD=lexis -e POSTGRES_DB=lexis_test postgres:17-alpine
  LEXIS_TEST_DSN='postgres://lexis:lexis@127.0.0.1:5444/lexis_test?sslmode=disable' \
    go test -tags integration -p 1 ./... -race
  docker stop lexis-testdb
  ```
  `-p 1` обязателен: все пакеты делят одну тестовую БД и сбрасывают схему, параллельно они друг другу мешают.
  В тестах модулей БД берётся через `dbtest.New(t)` (`internal/platform/db/dbtest`): пул + сброс схемы + миграции, без `LEXIS_TEST_DSN` — `t.Skip`.
  Данные — через `dbtest.Fixtures{Pool, Now}`: `Group`, `Word`, `Session`, `Review`, `Exec`, `Int` (прямые INSERT-ы с точными уровнями/датами).
  Тесты training/groups пока со своими локальными хелперами — можно перевести на Fixtures при случае.
- ⚠️ На машине Danik заняты контейнерами другого проекта (`application-*`): **8080, 8081, 8082** (HTTP) и **5433, 5434, 5435** (Postgres).
  Решено (2026-11-02): Lexis на хосте — **8090** (`LEXIS_PORT`, внутри контейнера по-прежнему :8080), отладочная БД — **5436**
  (раскомментировать `ports` у db; `.vscode/launch.json` уже на 5436 и `LEXIS_HTTP_ADDR=:8090`), тестовая БД — **5444**.
- **Фронт:** `cd frontend && npm run dev` (Vite :5173 на `localhost` — в macOS это IPv6 `::1`, `127.0.0.1:5173` не ответит; прокси `/api` → `LEXIS_API_URL`, по умолчанию `http://127.0.0.1:8090`),
  `npm run check` (svelte-check, предупреждения = ошибки), `npm test` (vitest, только чистая логика в `src/**/*.test.ts`), `npm run build` → `dist/`.

## 16. Конвенции фронтенда

- **Структура:** `src/lib/{api,i18n,state,components}` — общее, `src/tabs/<Tab>/` — экраны. Логика, которую можно проверить без DOM, — в `.ts`
  рядом с `*.test.ts` (vitest); компоненты проверяются глазами.
- **Состояние:** runes в модулях `*.svelte.ts` (`route`, `toasts`, `i18n`) — объект с геттерами поверх `$state`, без store-библиотек.
- **API:** только через `api.*` из `lib/api/client.ts`; ошибки — `ApiError{status, code}`, текст для пользователя — `errorText(e)` (i18n `errors.<code>`),
  ошибки строк парсера — `lineErrorText(code)`. Коды ошибок бэка ↔ ключи `errors.*` в обоих словарях.
- **i18n:** все строки через `t('ключ', {params})`; ключ типизирован (опечатка = ошибка svelte-check); множественные формы —
  объект `{one, few, many, other}` (ru) / `{one, other}` (en), выбор — `Intl.PluralRules`.
- **Выделения:** смещения с бэка — в рунах; резать строку только через `lib/text.segments` (по `Array.from`), не `String.slice`.
- **Стиль:** значения только из токенов `app.css` (`--c-*`, `--s-*`, `--bw`, `--shadow`, `--dur*`); анимации 200–300ms, `prefers-reduced-motion` их гасит.
- **Зависимости:** TypeScript закреплён на 6.x — `svelte-check` пока не поддерживает TS 7.

