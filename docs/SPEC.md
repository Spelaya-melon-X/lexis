# Lexis — спецификация

Единый источник правды по продукту и архитектуре. Всё ниже уже согласовано с владельцем проекта (Danik).
Если реализация расходится со спекой, сначала спроси, потом меняй спеку и код вместе.

---

## 1. Что это

Локальное **офлайн** веб-приложение для изучения английских слов. Один пользователь, работает на MacBook Air M4 (macOS).
Запуск: `python3 run.py` → Docker поднимает 2 контейнера → `http://localhost:8090` (`LEXIS_PORT`; 8080 на машине занят другим проектом).

Никаких внешних API: переводы и примеры пользователь готовит заранее (обычно через Claude) и вставляет текстом.

Будущее (НЕ реализуем сейчас, но архитектура должна позволять добавить модулем): планер дня, To-Do, трекер привычек.
**v2 (готово, 2026-11-05):** приложение — личный трекер: разделы «Английский», «День», «Привычки» и ⚙, см. §17.

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
        └── sections/english/{AddWords,Groups,Training,Stats,Settings}/   (до v2 — tabs/)
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

Полная схема: `backend/migrations/0001_init.sql`. С v2 (§17) приложение в ежедневном использовании: `0001` не правим, изменения — только новыми миграциями. Кратко:

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
POST /api/data/reset             {"confirm": true, "scope"?: "english" | "all"} → 204   (400 confirm_required | invalid_scope; подтверждение — на фронте)
                                 english (по умолчанию) — слова, группы, история, достижения; all — ещё дни, недели, привычки (§17.10). settings остаются.
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
- **Навигация:** с v2 — разделы в шапке + подразделы в сайдбаре (§17.1); у английского: Новые (ввод) · Группы · Тренировка · Статистика · Настройки. Основные разделы без скролла страницы; внутри разделов скролл для длинных списков ок.
- **Анимации:** простые — fade, slide, scale; 200–300ms; ease-out или лёгкий spring. Переключение табов — плавное появление; карточка — 3D flip; добавление слова — fade-in; смена уровня — анимация прогресс-бара.
- **i18n:** RU / EN, переключатель в шапке. Все тексты UI — через словари `lib/i18n/{ru,en}.json`.

### Settings
С v2 разделены (§17.1): ⚙ → Общие (язык) и ⚙ → Данные (полный бэкап, восстановление, «Очистить всё»);
Английский → Настройки — лимит новых слов, импорт/экспорт слов (CSV / JSON-список), «Очистить английский».
Файл бэкапа в импорте слов не восстанавливается — подсказка «восстанавливай в ⚙ → Данные».
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

- **Структура:** `src/lib/{api,i18n,state,components}` — общее, `src/sections/<section>/<Page>/` — экраны, `src/sections/nav.ts` — карта навигации. Логика, которую можно проверить без DOM, — в `.ts`
  рядом с `*.test.ts` (vitest); компоненты проверяются глазами.
- **Состояние:** runes в модулях `*.svelte.ts` (`route`, `toasts`, `i18n`) — объект с геттерами поверх `$state`, без store-библиотек.
- **API:** только через `api.*` из `lib/api/client.ts`; ошибки — `ApiError{status, code}`, текст для пользователя — `errorText(e)` (i18n `errors.<code>`),
  ошибки строк парсера — `lineErrorText(code)`. Коды ошибок бэка ↔ ключи `errors.*` в обоих словарях.
- **i18n:** все строки через `t('ключ', {params})`; ключ типизирован (опечатка = ошибка svelte-check); множественные формы —
  объект `{one, few, many, other}` (ru) / `{one, other}` (en), выбор — `Intl.PluralRules`.
- **Выделения:** смещения с бэка — в рунах; резать строку только через `lib/text.segments` (по `Array.from`), не `String.slice`.
- **Стиль:** значения только из токенов `app.css` (`--c-*`, `--s-*`, `--bw`, `--shadow`, `--dur*`); анимации 200–300ms, `prefers-reduced-motion` их гасит.
- **Зависимости:** TypeScript закреплён на 6.x — `svelte-check` пока не поддерживает TS 7.


---

## 17. Трекер жизни (v2)

> Согласовано 2026-11-03: все вопросы Q1–Q14 приняты по рекомендации (✅), Markdown/формулы — убраны (§17.6).

Lexis перестаёт быть только «английским»: это личный трекер, где английский — один из разделов.
Существующий функционал и данные не меняются; новое — новыми модулями `app.Module` и **новыми** миграциями
(`0001_init.sql` больше не правим: приложением ежедневно пользуются, правило §10 «правим на месте» отменено).

### 17.1 Навигация

Двухуровневая: шапка (текущий дизайн) = разделы, слева сайдбар = подразделы раздела.

| Раздел (шапка) | Подразделы (сайдбар) | Хэш |
|---|---|---|
| Английский | Новые · Группы · Тренировка · Статистика · Настройки | `#/english/add`, `#/english/groups[/<id>]`, `#/english/training[/<groupId>]`, `#/english/stats`, `#/english/settings` |
| День | День (сегодня или выбранная дата) · Календарь · Неделя (анализ) · Статистика | `#/day/entry[/<YYYY-MM-DD>]` (без даты — сегодня), `#/day/calendar[/<YYYY-MM>]`, `#/day/week[/<понедельник YYYY-MM-DD>]`, `#/day/stats` |
| Привычки | Отметки · Календарь · Цели · Наблюдения · Мои привычки | `#/habits/check[/<YYYY-MM-DD>]`, `#/habits/calendar[/<YYYY-MM>]`, `#/habits/goals`, `#/habits/notes`, `#/habits/manage` |
| ⚙ (иконка справа в шапке, рядом с RU/EN) ✅ | Общие · Данные | `#/settings/general`, `#/settings/data` |

- Формат хэша: `#/<section>/<page>/<rest>`; неизвестный подраздел → первый в разделе (хвост отбрасывается), мусор → `#/english/add`.
  Реализация: `lib/state/route.ts` (`SECTIONS`, `parseHash`, `toHash`; `Route` — размеченное объединение, чужой подраздел — ошибка типов),
  карта «подраздел → иконка, i18n-ключ, компонент» — `sections/nav.ts`.
- **Редирект старых ссылок** (`history.replaceState`, без записи в историю и без лишнего `hashchange`): `#add` → `#/english/add`, `#groups/12` → `#/english/groups/12`,
  `#training/5`, `#stats`, `#settings` — аналогично. Любой неканонический хэш заменяется на `toHash(parseHash(hash))`. Table-driven тест на все старые формы.
- Последняя открытая страница каждого раздела запоминается (localStorage `lexis.lastPages`): клик по разделу в шапке возвращает туда, где был;
  ссылка в сайдбаре ведёт на подраздел без хвоста («Группы» из открытой группы → список).
- Узкое окно (< 860px): в шапке только иконки разделов, сайдбар сворачивается в колонку иконок.
- **Глобальные настройки:** язык; бэкап/восстановление всего; очистка. Настройки английского (`#/english/settings`):
  лимит новых слов, импорт/экспорт слов CSV/JSON-списком, «Очистить английский». Разделено в фазе 5 ✅.
- ✅ Q7 Общая статистика (серии, активность по всем разделам) — пока нет; у каждого раздела своя «Статистика».
  Позже можно добавить раздел «Обзор» (дашборд «сегодня»: due-слова, оценка дня, неотмеченные привычки).

### 17.2 День (модуль `journal`)

Запись дня — по дате (`DATE` в TZ пользователя, «сегодня» — из `clock`). Создаётся лениво при первой записи.

- **Оценка дня** 1–10 (или не оценён). Клик по той же оценке — снять.
- **«Что классного сделал»** — список коротких пунктов (≤ 200 символов, ≤ 30 за день): добавить (Enter) / править (клик по тексту; пустой = удалить) / удалить / переставить.
  Любое изменение отправляет список целиком; пустые пункты бэк выбрасывает.
- **Рассказ** — свободный текст (§17.6, ≤ 20 000 символов), автосохранение через 1 с паузы, при уходе из поля и при смене дня + индикатор «сохранено».
  Источник правды для рассказа — поле: ответы сервера его не перезаписывают (иначе ответ на старый запрос откатил бы допечатанное).
- **Дофаминовая шкала** — 7 квадратиков. Клик → поле ввода (короткий текст ≤ 140 символов), Enter — сохранить;
  заполненный квадратик «загорается» (зелёный, scale-анимация). Очистить текст = погасить. Уровень дня = число заполненных (0–7).
  - ✅ Q1 всегда ровно 7 на день; в квадратике один пункт (одна строка); править/очищать можно; позиция квадратика
    не важна для уровня (заполнять можно в любом порядке).
  - ✅ Q2 дофамин — часть раздела «День» (блок на странице дня), не отдельный раздел.
- Навигация по датам: ← / → (на день), «Сегодня», выбор даты (нативный пикер по клику на заголовок); будущее и мусор в адресе → сегодня.
  Календарь месяца (`#/day/calendar[/YYYY-MM]`): клетка — цвет по оценке (1–3 красный, 4–6 жёлтый, 7–10 зелёный), число оценки,
  7 делений дофамина; сверху — дней с записями, средняя оценка (по оценённым), средний дофамин (по дням с записями — пустой день значит «не вёл», а не 0).
- Остальные изменения — оптимистично: интерфейс меняется сразу, ошибка → тост и перечитывание дня.
- Раскладка страницы дня (2026-11-03): две независимые колонки по половине ширины — слева «Оценка» + «Что классного», справа «Дофамин» + «Привычки»;
  «Рассказ» — на всю ширину снизу; уже 960px — одна колонка. Шкала оценки 1–10 — квадратные ячейки; в карточке уже 340px — два ряда по 5 (`@container`).
- Прошлые дни можно дополнять. ✅ Q11 Будущие даты — запрещено (400 `future_day`): оценивать день, который не наступил, бессмысленно.
- ✅ Q9 Отметки привычек на странице дня — компактный блок «Привычки» (что отмечено за день) со ссылкой на `#/habits/check/<day>`, без редактирования там. Делается в фазе 3.

### 17.3 Недельный анализ (модуль `journal`)

- Неделя — с **понедельника** (как в группах §5). Ключ — дата понедельника.
- Запись = упорядоченный список блоков `{topic: «О чём» (≤ 200), conclusion: «Вывод» (≤ 20 000, простой текст)}`, ≤ 30 блоков:
  добавить / удалить / переставить (кнопки ↑↓, без drag&drop-библиотек). Сохраняется целиком кнопкой «Сохранить» (⌘/Ctrl+S; PUT заменяет список);
  пустые блоки выкидываются; пустой список удаляет анализ недели. Смена недели с несохранёнными правками — подтверждение.
- `#/day/week[/<понедельник>]` (любой день в адресе приводится к понедельнику своей недели; будущее → текущая).
  Слева — анализ (3/5 ширины), справа — сводка недели: средние (дней с записями, оценка, дофамин), график недели (`DayChart`),
  выполнение привычек (`HabitStats`), цели, пересекающие неделю.
- Будущие недели писать нельзя (`future_day`), прошлые — можно.
- ✅ Q3 писать можно в любой день, в т.ч. текущей недели; в пт/сб/вс текущей недели, пока анализа нет, — подсказка «пора подвести итоги».

### 17.4 Привычки (модуль `habits`)

**Привычка:** название (1–60 символов, уникальное без учёта регистра, пробелы схлопываются), тип `positive` | `negative`, цвет (палитра групп), опциональные поля:
`trackTrigger` (триггер, текст ≤ 200), `trackRating` (оценка 1–10), `trackAmount` (количество, число 0..1 000 000, дробное, + `unit` — свободный текст ≤ 20: «км», «стаканов»).
Флаги можно менять потом: старые значения остаются в БД, валидация действует на новые отметки (пересохранение дня отправляет только включённые поля).
**Тип после создания не меняется**: от него зависит смысл старых отметок («сделал» / «сорвался»).
`startDay` — дата создания в TZ пользователя: до неё у негативной привычки день не считается «удержался».
Порядок — ↑↓ в «Мои привычки» (`PUT /order` с полным списком id, архивные тоже).
✅ Q12 Удаление — «архивировать» (пропадает из отметок, история остаётся) + полноценное удаление с подтверждением.

**Отметка за день** (`#/habits/check[/YYYY-MM-DD]`): форма-список активных привычек (+ архивные, если у них уже есть отметка в этот день);
у каждой — «Сделал» / «Сорвался» + включённые поля → одна кнопка «Сохранить» (⌘/Ctrl+S) на все изменённые строки (пунктирная рамка).
Сохранение — одна транзакция: ошибка в любой строке не пишет ничего. Количество принимает «5,5» и «5.5».
Смена даты с несохранёнными правками — подтверждение; закрытие вкладки — предупреждение браузера.
Задним числом — да; будущие даты — нет (как в §17.2).
- ✅ Q4 Негативная привычка: отметка = «это случилось» (сорвался). Триггер тогда естественно = «что спровоцировало».
  Цвета: positive отмечена — зелёная клетка; negative отмечена — красная; прошедший день без отметки у negative — бледно-зелёный («удержался»),
  у positive — пусто. Будущее — пусто.
- ✅ Q5 максимум одна отметка на привычку в день (`PRIMARY KEY (habit_id, day)`); если бегал дважды — суммируешь количество в одной отметке.
- ✅ Q13 при `trackAmount` количество **обязательно** для отметки; оценка и триггер — необязательны. Поле, которое у привычки выключено, → 400 `field_not_enabled`.

**Календарь** (`#/habits/calendar[/<понедельник> | /<YYYY-MM>]`, неделя / месяц): строки — привычки (архивные — если есть отметки в окне), колонки — дни,
клетка — цвет по правилам Q4 (`lib/habits.cellState`), tooltip — значения; на неделе значения видны в клетке. Клик по дню в шапке → «Отметки» этого дня.
Цели — тонкая полоса над строкой привычки (на неделе с подписью: текст или прогресс), общие цели — отдельные строки сверху.

**Цели на период:** `{habitId | null, title, from, to}` — «с 1 по 9 ноября бегать». Только напоминание: прогресс показываем
(«4 из 9 дн.»; у негативной — «без срыва 4 из 9 дн.»: done = дни без отметки), провалом не считаем. Даты — до 366 дней.
Страница «Цели»: «Сейчас и впереди» (будущие — пунктиром) и «Прошедшие», прогресс-бар цветом привычки. ✅ Q6 привязка к привычке необязательна — можно свободным текстом (тогда title обязателен).

**Наблюдения и выводы:** лента, новые сверху, тип `observation` | `insight`, привязка к привычке или общая, дата — автоматически (момент создания из clock),
текст — простой (§17.6, ≤ 20 000). Фильтр по привычке (все / общие / конкретная) и типу, подгрузка по 20 (курсор `before=<id>`). Править (прямо в ленте) и удалять можно.
Вывод визуально выделен (зелёная полоса слева).

### 17.5 Статистика новых разделов

- День → Статистика (`#/day/stats`): последние 7 / 30 дней (`Segmented`, выбор запоминается в localStorage), плитки — дней с записями,
  средняя оценка, средний дофамин, лучший день (`lib/journal.bestDay`: макс. оценка → больше дофамина → свежее; ссылка на день),
  график (`DayChart`: столбики дофамина на фиксированной шкале 0..7 — `chart.bars(…, scaleMax)`, линия оценки 1..10), выполнение привычек.
  Это и есть «общая статистика жизни» (отдельного раздела нет, Q7).
- Привычки за период (`habits.Count` → `PeriodStat`, `GET /api/habits/stats`): учитываемые дни — с max(from, дата создания) по min(to, сегодня)
  (у новой привычки нет «1 из 30»); positive — «отмечено N из M дн.»; negative — число срывов, доля чистых дней и лучшая серия без срыва;
  сумма количества и средняя оценка, если есть. Архивные — только с отметками в окне.
- Считается на лету из БД, без событий. ✅ Q14 Ачивки для новых разделов — не сейчас; новые события в `eventbus` не добавляем, пока нет подписчика.

### 17.6 Длинные тексты (без Markdown — решено 2026-11-03)

Markdown и формулы **не поддерживаем**: владельцу не нужны, а это +3 зависимости (`marked`, `dompurify`, `katex`) и санитизация.
Рассказ о дне, выводы недели, наблюдения — **простой текст**: `textarea` (авторост высоты), при показе переносы строк сохраняются
(`white-space: pre-wrap`), HTML не интерпретируется (Svelte экранирует `{text}`). Новых зависимостей нет.
Если понадобится — можно добавить маленький свой рендер подмножества (заголовки, **жирный**, *курсив*, списки) с экранированием
сначала, без библиотек.

### 17.7 БД (миграции `0002_journal.sql` ✅, `0003_habits.sql` ✅, `0004_week_reviews.sql` ✅)

Дни — `DATE` (дата в TZ пользователя). В Go — `time.Time` в 00:00 **UTC** («гражданская дата»), создаётся только через новые хелперы
`clock.DateOf(t)` (дата t в её зоне), `clock.TodayDate(c)` и `clock.ParseDate("YYYY-MM-DD")`, обратно — `clock.FormatDate`; в API — строка `YYYY-MM-DD`.
sqlc: глобальный override `date → time.Time` (`*time.Time` для NULL).
Так дата не «съезжает» на день при конвертации таймзон.

```sql
-- 0002_journal.sql
days              (day DATE PK, rating SMALLINT NULL CHECK 1..10, story TEXT NOT NULL DEFAULT '', created_at, updated_at)
day_wins          (id, day → days ON DELETE CASCADE, position SMALLINT, text TEXT CHECK <> '', UNIQUE (day, position))
dopamine_slots    (day → days ON DELETE CASCADE, slot SMALLINT CHECK 0..6, text TEXT CHECK (text <> '' AND char_length(text) <= 140),
                   updated_at, PRIMARY KEY (day, slot))          -- пустой квадратик = нет строки
-- 0004_week_reviews.sql ✅ (фаза 4 — вместе с API: в схеме не держим неиспользуемых таблиц)
week_reviews      (week_start DATE PK CHECK (extract(isodow FROM week_start) = 1), created_at, updated_at)
week_review_blocks(id, week_start → week_reviews ON DELETE CASCADE, position SMALLINT, topic TEXT CHECK ≤ 200, conclusion TEXT CHECK ≤ 20000,
                   CHECK (topic <> '' OR conclusion <> ''), UNIQUE (week_start, position))

-- 0003_habits.sql
habits            (id, name TEXT CHECK 1..60, kind TEXT CHECK IN ('positive','negative'), color TEXT NULL CHECK ~ '^#[0-9a-f]{6}$',
                   track_trigger BOOL, track_rating BOOL, track_amount BOOL, unit TEXT NOT NULL DEFAULT '' CHECK ≤ 20,
                   position INT, archived_at TIMESTAMPTZ NULL, created_at)   UNIQUE INDEX (lower(name))
habit_checkins    (habit_id → habits ON DELETE CASCADE, day DATE, trigger TEXT NULL, rating SMALLINT NULL CHECK 1..10,
                   amount DOUBLE PRECISION NULL CHECK >= 0, created_at, updated_at, PRIMARY KEY (habit_id, day))
                  INDEX (day)                                    -- «все отметки за период»
habit_goals       (id, habit_id → habits ON DELETE CASCADE NULL, title TEXT NOT NULL DEFAULT '', start_day DATE, end_day DATE,
                   created_at, CHECK (end_day >= start_day), CHECK (habit_id IS NOT NULL OR title <> ''))
                  INDEX (end_day, start_day)                     -- «пересекает окно»: end >= from AND start <= to
habit_notes       (id, habit_id → habits ON DELETE SET NULL NULL, kind TEXT CHECK IN ('observation','insight'),
                   body TEXT CHECK 1..20000, created_at, updated_at)  INDEX (habit_id, id); лента — ORDER BY id DESC (порядок создания)
```
- `amount` — `DOUBLE PRECISION`, а не `NUMERIC`: pgx отдаёт NUMERIC как `pgtype.Numeric`, неудобно; точность float64 для «5.2 км» с запасом, фронт округляет до 2 знаков.
- Заметка общая после удаления привычки (`SET NULL`), цели привычки удаляются вместе с ней (`CASCADE`).

### 17.8 API

```
# journal
GET    /api/journal/days/{day}             → Day = {day, rating|null, story, wins: string[], dopamine: [7 × string|null], dopamineLevel}
                                           (дня без записи нет — отдаётся пустой, не 404; читать будущее можно, писать — нет)
PATCH  /api/journal/days/{day}             {rating?: 1..10|null, story?, wins?: string[]} → Day   (нет поля — не менять; wins заменяют список целиком)
PUT    /api/journal/days/{day}/dopamine/{slot}   {text} → Day       (slot 0..6; text trim, ≤ 140 символов; пустой = погасить)
GET    /api/journal/days?from=&to=         → {from, to, days: [{day, rating|null, dopamineLevel, wins, hasStory}]}
                                           (включительно; по умолчанию 30 последних дней; все дни окна, будущие тоже; ≤ 366)
GET    /api/journal/weeks/{monday}         → Week = {weekStart, weekEnd, blocks: [{topic, conclusion}], updatedAt|null, summary: WeekSummary}
                                           (недели без анализа нет — пустые blocks, updatedAt: null)
PUT    /api/journal/weeks/{monday}         {blocks: [...]} → Week   ([] — удалить анализ; будущая неделя — future_day)
       WeekSummary = {days: DaySummary[7], filled, avgRating|null, avgDopamine|null, habits: HabitStat[], goals: Goal[]}
       400 invalid_date | invalid_range | future_day | invalid_rating | invalid_slot | text_too_long | too_many_items | invalid_json
           (+ not_monday для недель)

# habits
GET    /api/habits?archived=true           → {habits: Habit[]}   (по position)
       Habit = {id, name, kind, color, trackTrigger, trackRating, trackAmount, unit, archived, startDay, createdAt}
                                           (без ?archived=true — только активные)
POST   /api/habits                         {name, kind, color?, trackTrigger, trackRating, trackAmount, unit?} → 201 Habit
PATCH  /api/habits/{id}                    {name?, color?: "#rrggbb"|null, trackTrigger?, trackRating?, trackAmount?, unit?, archived?} → Habit
                                           (kind не меняется — неизвестное поле → 400 invalid_json)
DELETE /api/habits/{id}                    → 204
PUT    /api/habits/order                   {ids: [...]} → 204   (все id ровно по разу, иначе 400 invalid_order)
GET    /api/habits/checkins?from=&to=      → {from, to, checkins: [{habitId, day, trigger|null, rating|null, amount|null}]}  (по умолчанию 30 дней)
PUT    /api/habits/checkins/{day}          {checkins: [{habitId, done, trigger?, rating?, amount?}]} → {checkins: все отметки дня}
                                           (одна транзакция; done=false удаляет; привычки не из списка не трогаются; повтор habitId → invalid_habit)
GET    /api/habits/goals[?from=&to=]       → {goals: Goal[]}   (с окном — пересекающие его; без — все, новые сверху)
       Goal = {id, habitId|null, title, from, to, progress: {done, elapsed, total} | null}
POST   /api/habits/goals                   {habitId|null, title, from, to} → 201 Goal
PUT    /api/habits/goals/{id}              то же → Goal   (заменяет целиком)      DELETE /api/habits/goals/{id} → 204
GET    /api/habits/notes?habit=<id>|general&kind=observation|insight&before=<id>&limit=1..100 (20)
                                           → {notes: [{id, habitId|null, kind, body, createdAt, updatedAt}], next: id|null}
POST   /api/habits/notes                   {habitId|null, kind, body} → 201 Note
PUT    /api/habits/notes/{id}              то же → Note   (заменяет целиком)      DELETE /api/habits/notes/{id} → 204
       400 invalid_id | invalid_habit_name | invalid_kind | invalid_color | invalid_unit | invalid_habit | invalid_order
           | field_not_enabled | amount_required | invalid_rating | invalid_amount | text_too_long | empty_text | title_required
           | invalid_date | invalid_range | future_day | invalid_cursor | invalid_limit; 404; 409 duplicate_habit
```
Сводке недели нужны данные привычек: `journal` получает их через узкий интерфейс `journal.HabitSource{PeriodStats, Goals}`
(реализует `*habits.Service`, `journal` импортирует `habits` только ради типов) — как `groups.WordLister`. В `router.go` — отдельный экземпляр `habits.NewService`.
```
GET    /api/habits/stats?from=&to=         → {from, to, habits: HabitStat[]}   (по умолчанию 30 дней)
       HabitStat = {habitId, name, kind, color, unit, trackAmount, trackRating, archived,
                    days, checked, amount|null, avgRating|null, bestClean}
```

### 17.9 Структура кода

```
backend/internal/journal/   module.go (хендлеры) journal.go (типы, валидация) calc.go (FillDays) service.go store/   queries/journal.sql
backend/internal/habits/    module.go (хендлеры) habits.go (типы, Fields/Patch/ValidateCheckin/NoteInput) rules.go (цели, CheckOrder) service.go store/
backend/internal/platform/days/   Parse, ParseRange, CheckWritable — даты из запроса (общие для journal и habits)
backend/internal/platform/clock/  + date.go: DateOf, TodayDate, ParseDate, FormatDate
backend/internal/journal/week.go  недельный анализ: HabitSource, NormalizeBlocks, Averages, Week/SaveWeek
backend/internal/habits/stats.go  Count (чистая) → PeriodStat, PeriodStats, Stats
backend/internal/platform/httpx/  + Optional[T] — PATCH-поле «нет / null / значение» (обобщение groups.OptionalString)
frontend/src/lib/state/route.ts   section/page/rest + редиректы старых хэшей
frontend/src/sections/nav.ts      карта навигации (раздел → подразделы → компоненты)
frontend/src/sections/english/    ← нынешние tabs/* (переезд без изменений поведения)
frontend/src/sections/day/        Entry/{Entry,DopamineScale,WinsList,Story,HabitsBlock}, Calendar/, Week/, Stats/, DayChart, HabitStats
frontend/src/lib/components/      + DatePager, RatingPicker, ColorPicker (общие для разделов)
frontend/src/lib/journal.ts       ratingTone, move, isFilled, summarize; lib/dates.ts + formatDay/today/addDays/isDay/monthGrid/…
frontend/src/sections/habits/     HabitDialog, Check/{Check,CheckRow}, Calendar/, Goals/{Goals,GoalDialog}, Notes/{Notes,NoteEditor}, Manage/
frontend/src/lib/habits.ts        cellState, weekStart, daysRange, goalSpans, parseAmount/formatAmount, draftOf/sameDraft/toInput
frontend/src/sections/settings/   General, Data
```
Чистые функции с table-driven тестами: `journal.Patch.Normalize`, `NormalizeDopamine`, `FillDays`, `NormalizeBlocks`, `Averages`; `days.Parse/ParseRange/CheckWritable/WeekStart/ParseWeek`;
`habits.Fields.Normalize`, `Patch.Apply`, `ValidateCheckin`, `GoalInput.Normalize`, `GoalProgress`, `NoteInput.Normalize`, `CheckOrder`
`habits.Count` (выполнение за период: учитываемые дни, сумма, средняя оценка, лучшая чистая серия);
фронт: `journal.bestDay`, `chart.bars(…, scaleMax)`;
фронт: `parseHash`/редиректы, раскладка календаря привычек, полосы целей.

### 17.10 Бэкап и очистка

- Бэкап `version: 2` = v1 + `journal: {days: [{day, rating, story, wins: string[], dopamine: [{slot, text, updatedAt}], createdAt, updatedAt}]}` ✅
  + `habits: {habits: [{id, name, kind, color, trackTrigger, trackRating, trackAmount, unit, position, archivedAt, createdAt}],
  checkins: [{habitId, day, trigger, rating, amount, createdAt, updatedAt}], goals: [{habitId, title, from, to, createdAt}],
  notes: [{habitId, kind, body, createdAt, updatedAt}]}` ✅ + `journal.weeks: [{weekStart, blocks: [{topic, conclusion}], createdAt, updatedAt}]` ✅ (в бэкапах до фазы 4 ключа нет — пусто).
  Новые секции необязательные, версия остаётся 2.
  Файлы `version: 1` восстанавливаются; v1 с секцией `journal`/`habits` — 400 `invalid_backup`. Ответ импорта — `restored.days`, `restored.habits`.
- ✅ Q10 восстановление заменяет только те разделы, что есть в файле (v1-файл не сотрёт дневник и привычки); битая секция откатывает всё восстановление;
  очистка — по разделам: «Очистить английский» (в его настройках, `scope: english`) и «Очистить всё» (⚙ → Данные, `scope: all`) ✅.
- Бэкап новых данных делаем **в той же фазе, что и модуль** (а не в конце): иначе между фазами данные дня существуют без бэкапа.

### 17.11 Уведомления

✅ Q8 только внутри приложения (как решено в §9): точка на разделе «День» в шапке, если после 18:00 сегодня не оценён;
счётчик неотмеченных привычек на «Привычках». Системных уведомлений нет.
Реализовано: точка (янтарная) на «Дне» — после 18:00 по часам браузера, если сегодня нет оценки; число на «Привычках» — активные позитивные
привычки без отметки сегодня (негативные не считаются: «нет отметки» = удержался). Обновляется при старте, переходах, фокусе вкладки,
раз в 5 минут и после сохранения оценки / отметок. Ошибка сети — напоминания просто не показываются.

### 17.12 Фазы

1. ✅ (2026-11-03) Оболочка навигации: шапка разделов + сайдбар (`App.svelte`, токены `--sidebar-w`/`--sidebar-w-narrow`), `tabs/*` → `sections/english/*`,
   хэш `#/section/page`, редиректы старых хэшей, i18n `tabs.*` → `nav.*`, подразделы «Дня»/«Привычек» — заглушка `Soon`. ⚙ перенесён в фазу 5
   (до разделения настроек ему нечего показывать).
2. ✅ (2026-11-03) День: миграция `0002`, `clock`-хелперы дат, `httpx.Optional`, модуль `journal` (без недель), страница дня, дофамин, календарь месяца, бэкап v2 с днями.
   Интеграционные тесты: сервис (полночь МСК, `future_day`), HTTP, круговой бэкап с днями, v1-бэкап не трогает дневник, очистка английского — тоже.
3. ✅ (2026-11-03) Привычки: `0003`, модуль `habits` (CRUD, отметки, цели с прогрессом, лента наблюдений), `platform/days` (вынесено из journal),
   бэкап — секция `habits`; фронт — «Отметки», «Календарь» (неделя/месяц + цели), «Цели», «Наблюдения», «Мои привычки», блок привычек на странице дня.
   Интеграционные тесты: цикл привычки, отметки задним числом и атомарность формы, цели (прогресс positive/negative), лента с курсором и фильтрами,
   удаление привычки (каскад / заметки становятся общими), круговой бэкап с привычками, v1-бэкап и очистка английского их не трогают.
4. ✅ (2026-11-05) Неделя + статистика: `0004_week_reviews`, `days.WeekStart/ParseWeek`, недельный анализ со сводкой (`journal.HabitSource`),
   `habits.Count` + `GET /api/habits/stats`, страницы «Неделя» и «Статистика» (`DayChart`, `HabitStats`), бэкап `journal.weeks`; заглушка `Soon` удалена.
   Интеграционные тесты: неделя — пустая/сохранить/переставить/очистить, сводка с реальными привычками и целями, прошлая/будущая неделя; круговой бэкап с неделями.
5. ✅ (2026-11-05) Полировка: ⚙ в шапке (раздел `settings`, `header: false` в `nav.ts`) + разделение настроек, `POST /api/data/reset {scope}`,
   напоминания в шапке (`lib/reminders.ts` + `state/reminders.svelte.ts`), вычитка i18n (удалены осиротевшие ключи), README.
   Роутинг: «#settings» (v1, без слэша) — по-прежнему настройки английского, «#/settings» — ⚙ (новый формат всегда со слэшем).

Все фазы v2 закрыты (2026-11-05).
