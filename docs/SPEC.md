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
**v3 (готово, 2026-11-06):** вложенные группы английского, «Игра в жизнь» (дела, распорядок, фокус, награды, статистика), «Спорт» (тренировки, еда, статистика), см. §18.

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
- **Выделение:** разметка `[ ]` задаёт фрагменты предложения. Пробелы внутри скобок обрезаются: `[ go ]` → `[go]`.
  В тренировке без явной разметки интерфейс ищет целое слово, словосочетание или одну из сохранённых форм без учёта регистра.
  Ошибки: незакрытая/лишняя/вложенная скобка, пустые `[ ]`.
- **Хранение примера:** в `word_examples.sentence` лежит нормализованная разметка (`I [go] home`). Предложение без скобок и спаны (руны, `[start, end)`) восстанавливает `words.ParseExample` при чтении и отдаёт API.
- **Три формы глагола:** term с `_` — это формы: `go_went_gone` → `forms = [go, went, gone]`, `term = "go_went_gone"` (части trim'аются). Частей не 3 или есть пустая — ошибка строки. Дефис — всегда часть слова (`well-known`).
  Карточка показывает три формы через тире на обеих сторонах.
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

Перед стартом выбери **группу**, направление EN → RU или RU → EN и обычную или свободную тренировку.
Одно слово даёт одну карточку. Лицевая сторона показывает слово (для глагола — три формы) или перевод согласно направлению.
Оборот показывает слово, все примеры в исходном порядке и перевод слова. У словарных примеров в модели есть текст и разметка выделения.

Поле ответа получает фокус при появлении карточки. Enter раскрывает ответ, 1 / 2 / 3 записывают самооценку.
UI: 3D-flip карточки, прогресс-бар, экран результатов (good/hard/again, время, изменения уровней).

### Порядок карточек в сессии (`training/queue.go`)
1. Корзины по срочности: просроченные → сегодняшние → новые. Внутри корзины — shuffle.
2. Лимит новых слов за сессию: 10 (настраивается в Settings: ключ `new_words_limit` в `settings`, JSON-число; нет/мусор → 10). Лимит берётся после shuffle.
3. «Не знаю» → карточка возвращается через 3–5 позиций, пока не получит Знаю/Затрудняюсь. Очередь держит клиент: `answer` отдаёт `requeueAt` (смещение 3..5) на каждый `again`, в т.ч. повторный.
4. Слова с одинаковым переводом без учёта регистра по возможности разносятся внутри одной корзины.
5. Если карточек нет — сессия не создаётся (`sessionId: null, cards: []`).
6. Состояние сессии — только в БД; «первый ответ на слово в сессии» = нет строки в `reviews` по `(session_id, word_id)`.

### Свободная тренировка (решено: да)
- Флаг `free=true` в `POST /api/training/start` и в `training_sessions.free`.
- Карточки: **все** слова группы (включая уровень 5 «Архив»), один общий shuffle без корзин и без лимита новых; разнесение похожих и requeue «Не знаю» работают как обычно.
- Ответы пишутся в `reviews` (`level_before = level_after`), SRS **не применяется**: `level`, `streak`, `next_review_at`, `last_reviewed_at`, `last_good_at` не меняются, событие `WordLevelChanged` не публикуется.
- Засчитывает группу как «повторённую» сегодня (§5); идёт в статистику, heatmap и серию дней (`SessionFinished{Free: true}` публикуется).


### Точечная тренировка из группы (2026-11-09)

- «Английский → Группы → конкретная группа»: рядом с «Тренировать» — «Выбрать слова».
  Кнопка включает чекбоксы и переводит фокус к списку слов (включая подгруппы, как обычный просмотр).
  Панель показывает число выбранных слов, «Выбрать все», «Снять выбор», «Тренировать выбранные» и отмену.
  Пустой набор запустить нельзя. Чекбоксы доступны с клавиатуры; панель переносится на узком экране.
- Перед запуском — диалог с выбором направления и переключателем свободной тренировки.
  По умолчанию **свободная**: берутся все выбранные слова, включая архив и не-due, без изменения SRS.
  Обычная — только новые/due из выбранных, с прежним лимитом новых слов и применением SRS.
  Если карточек для обычной нет, сессия не создаётся; диалог предлагает свободную с той же выборкой.
  Активную тренировку нельзя перезаписать новой — диалог предлагает вернуться к ней и сначала завершить.
- `POST /api/training/start` расширен необязательным `wordIds: int64[]`.
  Нет поля (или null) — прежняя тренировка группы с поддеревом; `[]`, неположительные ID, неизвестные/чужие слова — 400 `invalid_word_selection`.
  Повторы ID убираются с сохранением порядка первого появления. Слово из нескольких подгрупп приходит один раз.
  Все ссылки проверяются **до фильтрации due/архива**: чужое не-due слово тоже отклоняется, а не пропускается.
  Проверка группы, чтение слов/примеров/лимита и создание сессии — одной REPEATABLE READ транзакцией.
- Состав выборки живёт только локально в `GroupDetail` до старта и передаётся только в стартовом запросе.
  После успешного старта локальная выборка очищается, открывается существующий экран тренировки.
  Очередь держит существующий `state/session.svelte.ts`; при завершении карточки освобождаются, остаются только группа и обычные итоги.
  Сервер отдельный список выбранных слов не хранит; проверка ответов по группе/поддереву и записи фактических ответов остаются прежними.
  Повтор again, первые ответы, SRS, завершение, события и статистика используют существующий путь.
- Никаких новых таблиц, миграций, настроек, сохранённых наборов или полей бэкапа/истории.
  В истории и бэкапе остаются обычная сессия и фактические reviews — это ответы, а не сохранённый состав выборки.
  Завершённая точечная сессия засчитывается группе по прежним правилам, как другая обычная/free-сессия.
- Проверки: build/vet, unit/race и все интеграционные тесты на отдельной временной БД 5444; frontend check/test/build.
  Проверены валидация пустых/чужих/несуществующих ID, дубликаты, подгруппы, normal/free, лимит новых слов,
  повторы/итоги, неизменность чужих уровней, последующий старт всей группы, отсутствие списка в бэкапе и очистка клиентской очереди.
  Реальная проверка браузера/клавиатуры остаётся недоступной из-за отсутствия разрешённого browser/Computer Use доступа в текущем окружении.

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
POST /api/training/start         {groupId, strategy?, free?, wordIds?: id[]} → {sessionId | null, cards[]}
                                 Card = {wordId, term, forms?, translation, level, examples?: [{text, sentence, highlights?}]}
                                 strategy: en_ru или ru_en; отсутствие поля и старый context запускают en_ru
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

---

## 18. v3: вложенные группы → «Игра в жизнь» → Спорт (план, фаза 0 — 2026-11-05)

> Статус: **согласовано 2026-11-05** — все рекомендации («рек.») и ответы на ❓ приняты как есть.
> Порядок реализации: A → B → C (§18.5).

Общие правила v3 (как в v2): только новые миграции (`0005_…`), раздел = модуль `app.Module` + строка в `router.go`,
даты дня — `DATE` + `clock.DateOf/ParseDate`, из запроса — `platform/days`, PATCH с null — `httpx.Optional`,
межмодульные связи — узкие интерфейсы, без новых зависимостей (DnD — нативный HTML5 + клавиатура, граф/таймеры/графики — свои).
Бэкап новых данных — в той же фазе, что и модуль.

### 18.1 Бэкап v3 и очистка

- `BackupVersion = 3`: v2 + `groups[].parentId` (фаза A) + необязательные секции `game` (фазы B) и `sport` (фазы C) — по мере появления,
  как `journal.weeks` в v2. Версию поднимаем в фазе A: старое приложение строго отвергает неизвестные поля, номер версии — честный сигнал.
- Файлы v1/v2 восстанавливаются: `parentId` нет → все группы корневые; секций `game`/`sport` нет → эти разделы не трогаются (правило Q10).
- Восстановление дерева (группы, дела): вставка всех строк, затем `UPDATE parent_id` по карте id (порядок строк в файле не важен);
  `validateBackup` проверяет висячие `parentId`, циклы и глубину → 400 `invalid_backup`.
- Очистка: `scope: english` — английский (группы вместе с деревом), `scope: all` — ещё игра и спорт. ❓ G1 Отдельные «Очистить игру/спорт»? (рек.: нет, пока не попросишь).

### 18.2 Блок A — вложенные группы английского

**Модель (рек.):** `groups.parent_id` (self-FK), без отдельной сущности «папка». Папка = группа без своих слов.
- Группа может одновременно иметь свои слова и подгруппы.
- Имена групп остаются **глобально уникальными**: тег `#аэропорт` однозначно указывает группу, где бы она ни лежала.
- Глубина — не больше **3 уровней** (корень → подгруппа → под-подгруппа). ❓ A2 (рек.: 3; больше на десятках групп только путает).
- Циклы и глубина проверяет чистая функция `groups.CheckMove(parents map[int64]*int64, id, newParent) error` (table-driven),
  в транзакции под `pg_advisory_xact_lock` (дерево — десятки строк, грузим целиком). БД страхует `CHECK (parent_id <> id)`.
- Порядок соседей — **по имени** ❓ A1 (рек.: по имени, как сейчас; ручной порядок ломается от групп, созданных тегами).
  Поэтому DnD «между группами» = «в того же родителя, что и соседи».

**Агрегаты по поддереву (рек.):**
- Слово привязано только к своим группам (`word_groups` не меняется). «Слова родителя» = слова всего поддерева, **уникально** (слово в двух подгруппах — один раз).
- `wordCount`, `due`, `levels`, `% Мастер+Архив` — по поддереву; дополнительно `ownWordCount` (свои слова).
- Тренировка родителя = слова поддерева, уникально (обычная и free). ❓ A4 Переключатель «только свои слова» в тренировке? (рек.: нет, всегда поддерево).
- «Повторена сегодня» ⇔ завершённая сессия сегодня по **самой группе или её предку** (предок тренировал её слова) ИЛИ due поддерева = 0.
  Тренировка подгруппы родителя не засчитывает (кроме случая due = 0). ❓ A5 (рек.: так).
- Аналитика родителя (продуктивность, сессии за неделю/месяц) — сессии по группе **и потомкам**.
- Не меняются: `stats` «по группам-типам» и ачивка `first_archived_group` — по собственным словам группы ❓ A9 (рек.: не трогать).
- SQL: поддерево — `WITH RECURSIVE sub(root, id)` (замыкание «предок → потомок») + `count(DISTINCT w.id) FILTER (…)`; без N+1.
  training: `ListSessionWords` по `group_id = ANY(поддерево)` + `DISTINCT`; `WordInGroup` → «слово в поддереве».

**Удаление родителя:** диалог с выбором ❓ A7 (рек.: по умолчанию «поднять подгруппы на уровень выше», вторая опция — «удалить вместе с подгруппами»).
Слова не удаляются никогда (как сейчас). FK `ON DELETE RESTRICT` — дети обрабатываются в сервисе явно, а не каскадом «по ошибке».

**Ввод тегом (рек.):** `/` — разделитель пути: `#путешествия/аэропорт` → недостающие группы создаются по пути, слово привязывается **к листу**.
- Уже существующая группа с тем же именем, но другим родителем — не перемещаем, строка в `errors[]` с кодом `group_path_conflict`
  (явная разметка, без догадок; превью этого не знает — ошибка придёт после «Создать»). ❓ A6 (рек.: так).
- Путь длиннее 3 частей / пустая часть (`#a//b`) — ошибка парсера `invalid_group_path`.
- Имя одной группы больше не может содержать `/` (`NormalizeGroupName`). ⚠️ Перед миграцией проверю, нет ли в базе групп с `/` в имени — если есть, обсудим.
- CSV-экспорт без изменений (теги — имена групп); CSV/JSON-импорт понимает пути так же, как ввод.

**БД — `0005_group_tree.sql`:**
```sql
ALTER TABLE groups ADD COLUMN parent_id BIGINT REFERENCES groups (id) ON DELETE RESTRICT,
                   ADD CONSTRAINT groups_parent_not_self CHECK (parent_id <> id);
CREATE INDEX groups_parent_idx ON groups (parent_id);
```

**API:**
```
GET    /api/groups                → Group += {parentId|null, ownWordCount}; wordCount/due/levels/repeatedToday — по поддереву (§18.2)
POST   /api/groups                {name, parentId?, color?} → 201 Group   (пустая группа-папка; 400 invalid_group_name | invalid_parent | group_too_deep, 409 duplicate_group)
PATCH  /api/groups/:id            += parentId?: id|null (httpx.Optional; null — в корень) → 400 group_cycle | group_too_deep | invalid_parent
DELETE /api/groups/:id?children=lift|delete   (по умолчанию lift) → 204
GET    /api/groups/:id            аналитика по поддереву; words — слова поддерева
GET    /api/words?group=&subtree=true   (по умолчанию как сейчас — только свои)
POST   /api/training/start        groupId → слова поддерева
```

**Фронт:**
- `lib/groupTree.ts` (чистое, vitest): `buildTree`, `visibleRows(tree, collapsed)` (плоский список с отступом для рендера),
  `descendants`, `canMove(tree, id, target)` (не в себя, не в потомка, глубина), `dropTarget` (into / root).
- Список групп — дерево со сворачиванием (▸/▾; свёрнутые id — localStorage `lexis.groupsCollapsed`), у свёрнутого родителя видны агрегаты.
- DnD (HTML5): строка — «внутрь» (подсветка), зона «В корень» сверху списка появляется при перетаскивании; запрещённая цель — курсор `not-allowed`.
- Без мыши: меню строки «⋯» → «Переместить в…» (`MoveGroupDialog`: дерево-выбор, недоступные цели disabled, «В корень»).
- «Новая группа» (`NewGroupDialog`: имя + родитель), удаление — `ConfirmDialog` с выбором lift/delete, если есть подгруппы.
- Тренировка: выбор группы — тем же деревом (due по поддереву). Карточка группы — хлебные крошки «Путешествия › Аэропорт».

### 18.3 Блок B — «Игра в жизнь»

Раздел `game` в шапке. **Модули (рек.):** `tasks` (типы, колонки, дела, подзадачи, план дня, топ-5) и `focus` (ритуал, фокус-сессии,
перерывы, отвлечения, очки, магазин). `focus` получает дела через узкий интерфейс `focus.Tasks{Get, Complete}` (реализует `*tasks.Service`). ❓ B2 (рек.: два модуля).

**Навигация:**

| Подраздел | Хэш |
|---|---|
| Распорядок (по умолчанию) | `#/game/plan[/<YYYY-MM-DD>]` (без даты — сегодня) |
| Календарь | `#/game/calendar[/<YYYY-MM>]` (без месяца — текущий) |
| Задачи (доска) | `#/game/board[/<taskId>]` (с id — карточка дела + «граф дела») |
| Фокус | `#/game/focus` (активная сессия или «нет активной» → в распорядок) |
| Награды | `#/game/shop` (магазин, баланс, история) |
| Статистика | `#/game/stats` |
| Настройки | `#/game/settings` (типы, колонки, ритуал, шкала дня, лимит, длительность настройки) |

Баланс очков — в шапке раздела (над контентом). Активная фокус-сессия — плашка-таймер в шапке приложения (видна из любого раздела, клик → `#/game/focus`).

#### B1. Дела, типы, доска
- Дело: `title` (1..200), `blocks` — объём в блоках по 30 мин (1..40), `importance` 1..10, `deadline` DATE (❓ B3 необязателен? рек.: да),
  `typeId` (необязателен), колонка, `note` (простой текст ≤ 20 000).
- Карточка задачи и её диалог показывают метки из `note`. Тег начинается с `#` на границе слова, дальше идут буквы (в том числе кириллица и латиница), цифры, дефисы или подчёркивания. Одиночный `#` и фрагменты URL пропускаются. Метки приводятся к нижнему регистру и выводятся один раз в порядке появления. Хранится исходный текст. Метки вычисляются при отображении, включая задачи из старых данных и бэкапов.
- Типы — свои: имя (1..40, уникально без учёта регистра), цвет (палитра групп), порядок ↑↓.
- **Колонки (рек.):** пользовательские списки (имя, порядок), DnD дел между колонками и внутри (ручной порядок) + фильтр доски по типу. ❓ B1
  (рек.: свои списки; колонки по статусам дублировали бы «выполнено», по типам — фильтр). Первая колонка «Входящие» создаётся миграцией.
  Удалить колонку можно только пустую (409 `column_not_empty`).
- Доска показывает **листья** невыполненных дел (§B2), выполненные — переключатель «Показать выполненные». Карточка: тип (цвет), важность, объём (Nч Mм), дедлайн (просрочен — красным).
- Без мыши: на карточке «⋯» → «Переместить в колонку…», ↑↓.
- При перетаскивании карточки к левому или правому краю доска прокручивается к скрытым колонкам. Место вставки определяется по позиции указателя в момент отпускания.

#### B2. Подзадачи и граф
- `tasks.parent_id` (self-FK, `ON DELETE CASCADE` — удаление дела удаляет его шаги, с подтверждением). Глубина произвольная, страховка — 10.
- «Построить промежуточные шаги» → форма списка шагов → `POST /api/tasks/{id}/split` одной транзакцией.
- **Наследование (рек.):** у нового шага по умолчанию тип, важность, дедлайн и колонка — от родителя (редактируются); объём вводится (по умолчанию 1).
  Объём родителя = Σ объёмов листьев (вычисляется; исходная оценка сохраняется и показывается в графе «было 6, шаги 8»).
- **Выполнено (рек.):** `done_at` — только у листьев. Родитель выполнен ⇔ все листья выполнены (вычисляется, не хранится).
  Добавить шаг к выполненному листу нельзя (409 `task_done`); разбить дело с активной фокус-сессией — 409 `focus_active`.
- После разбиения родитель пропадает с доски и из плана (его пункты плана удаляются, на распорядке — подсказка «добавь шаги»).
- Граф: свой SVG, раскладка `lib/taskTree.layout` (чистая: листья по порядку по x, родитель — по центру детей), выполненные — светло-серым, остальные — белым, клик по узлу → карточка.

#### B3. Распорядок дня и топ-5
- Слева — шкала дня (по умолчанию 9:00–23:00, настройка), справа — топ-5 + «+» (поиск по всем листьям).
- ~~Без привязки ко времени~~ → **с привязкой (решено 2026-11-05, см. §18.5.4):** у пункта есть `start` (по получасам), дело ставится
  перетаскиванием в любое время; наложилось на другое — меняются местами; кнопок ↑↓ нет.
- Лимит дня — 18 блоков (9 ч, настраивается). **Превышение показываем, не запрещаем** ❓ B6 (рек.: полоса «12/18 ч» краснеет, тост-предупреждение).
- Планировать можно сегодня и любой будущий день; прошлые — только чтение (400 `past_day`). На одно дело — один пункт на день, но разные дни можно.
- Недели в сетке календаря начинаются с понедельника. Месяцы листаются в обе стороны, кнопка «Сегодня» возвращает к текущему месяцу. По ячейке открывается распорядок даты, по «+» в ячейке сегодняшнего или будущего дня — поиск по открытым делам-листьям, которых ещё нет в плане этой даты. Ячейки получают время и название дела из `plan_items` и `tasks` одним запросом за видимый месяц. Дедлайн хранится независимо. «Сегодня» вычисляется сервером по `LEXIS_TZ`.
- В распорядке рядом с выбором существующего дела есть «Новая задача». Форма использует поля доски, включая выбор колонки; задача и пункт плана выбранной даты записываются одной транзакцией. При ошибке запись откатывается, диалог сохраняет введённые значения.
- Дела из календаря и распорядка добавляются одним правилом. Ищется свободный получасовой слот от начала шкалы (для сегодня — с учётом текущего времени), затем пробуется место после последнего пункта в пределах суток. При отсутствии места возвращается `plan_no_slot`. Существующие пункты сохраняют время и объём.
- Незакрытое из прошлых планов — **не переносим автоматически**, на сегодня блок «Не доделал вчера: … [Перенести]». ❓ B8 (рек.: так).
- **Формула топ-5** (`tasks.Score`, чистая, table-driven) — кандидаты: невыполненные листья, не в плане этого дня:
  ```
  I   = importance / 10                                  ∈ [0.1, 1]
  d   = deadline − day (дни), dEff = d − ⌈blocks / 6⌉     -- крупное дело «срочнеет» раньше: 6 блоков ≈ день работы над ним
  U   = 0, если дедлайна нет; 1, если dEff ≤ 0; иначе 2^(−dEff / 7)   -- полураспад срочности — неделя
  Q   = 0.05, если blocks ≤ 2 (быстрая победа), иначе 0
  score = 0.6·I + 0.4·U + Q
  ```
  Тай-брейк: раньше дедлайн → старше дело. В API отдаём `score` и `reasons` (`deadline_soon`, `overdue`, `important`, `quick`) — UI пишет «почему».
  Примеры (день = пн):
  | Дело | imp | blocks | дедлайн | U | score |
  |---|---|---|---|---|---|
  | Отчёт | 5 | 2 | завтра (d=1, dEff=0) | 1 | 0.30+0.40+0.05 = **0.75** |
  | Просроченная мелочь | 3 | 1 | вчера | 1 | 0.18+0.40+0.05 = **0.63** |
  | Курсовая | 7 | 12 | через 14 дн (dEff=12) | 0.30 | 0.42+0.12 = **0.54** |
  | Мечта без срока | 9 | 8 | — | 0 | **0.54** |
  | Разобрать шкаф | 2 | 4 | через 30 дн | 0.06 | 0.12+0.02 = **0.14** |

#### B4. Ритуал фокуса
Состояние — в БД, все отметки времени — из `clock`; фронт считает таймеры от серверных отметок + поправка `serverNow − Date.now()`
(сервер отдаёт `now` в каждом ответе сессии). Одна активная сессия — частичный уникальный индекс.
1. «Начать дело» (из плана или с карточки дела; только невыполненный лист) → сессия, фаза **настройка**, таймер 10 мин (настраивается).
   Истёк — не блокируем, показываем перерасход. ❓ B10 (рек.: так).
2. Чек-лист ритуала (настраиваемый список). Отметки — **на фронте** (localStorage по id сессии), сервер их не хранит ❓ B9
   (рек.: так — это личный самоконтроль, не данные для статистики; перезагрузка их не сбрасывает). «Начал делать» активна, когда всё отмечено.
3. **Работа:** таймер на `blocks` из плана — минимум, после него идёт дальше (перебор — зелёным «+12 мин»).
4. **Перерывы:** накоплено = ⌊чистая работа / 30 мин⌋ × 5 мин; доступно = накоплено − потрачено (не меньше 0). Перерыв — отдельный таймер
   обратного отсчёта, кончается кнопкой «Вернулся». Сверх доступного — можно, перерасход красным, в работу не идёт и не вычитается ❓ B12 (рек.: так).
   Расчёт — чистая `focus.Timeline(session, breaks, now) → {setupSec, workSec, breakSec, breakAvailableSec, overrunSec, phase}` (table-driven).
5. **Отвлечения:** триггер (≤ 200, автодополнение из прошлых — `GET /api/focus/triggers?q=`) + «поддался» / «справился», пишутся сразу.
6. **Завершение (рек., ❓ B11):** `done` — «Я закончил» (лист выполнен), `stopped` — «На сегодня всё» (дело остаётся, время и очки засчитываются),
   `abandoned` — «Отменить» (только в фазе настройки, в статистику не идёт). Завершение во время перерыва закрывает перерыв.

#### B5. Статистика
- Считается на лету: за день — выполненные дела, время (настройка/работа/перерывы), отвлечения (поддался/справился);
  неделя/месяц — число выполненных дел и список, часы в фокусе по дням (`chart.bars`), топ-10 триггеров с долей «поддался».
- **Достижения через eventbus (рек., ❓ B16):** теперь есть подписчик — добавляем события `FocusFinished{SessionID, WorkSec, Outcome, At}`
  и `TaskCompleted{TaskID, At}`; `achievements.Evaluate` получает новые метрики снимка. Коды: `game_tasks_10/50/200`, `game_hours_10/50/200`
  (за всё время). «За неделю/месяц» — не ачивки, а плитки-рекорды в статистике («лучшая неделя: 14 ч»). Ачивки игры показываются в её статистике (фильтр по префиксу `game_`).

#### B6. Награды
- **Очки (рек., ❓ B14):** +1 за каждые полные 30 мин **чистой работы** в сессии (без настройки и перерывов), начисляются при завершении (`done`/`stopped`), остаток не переносится.
  Бонуса за выполненное дело нет (иначе выгодно дробить дела).
- Магазин: свои награды (название ≤ 100, цена > 0, порядок, архив). Покупка — одна транзакция под `pg_advisory_xact_lock`: баланс < цены → 409 `not_enough_points`.
- Журнал очков `points_ledger`: начисления и траты, баланс = Σ delta (не уходит в минус — проверяется при покупке). Лента с курсором, как наблюдения.
- ❓ B15 Покупка → дофамин дня? (рек.: галочка «записать в дофамин» в диалоге покупки, по умолчанию выключена; пишет в первый свободный квадратик
  сегодня через узкий интерфейс `focus.Dopamine` (реализует `journal.Service`); все 7 заняты — покупка проходит, тост «шкала полная»).

**БД:**
```sql
-- 0006_tasks.sql (B1)
task_types   (id, name TEXT CHECK 1..40, color TEXT NULL CHECK ~ '^#[0-9a-f]{6}$', position INT, created_at)  UNIQUE INDEX (lower(name))
task_columns (id, name TEXT CHECK 1..40, position INT, created_at)          -- + INSERT «Входящие»
tasks        (id, title TEXT CHECK 1..200, blocks SMALLINT CHECK 1..40, importance SMALLINT CHECK 1..10, deadline DATE NULL,
              type_id → task_types ON DELETE SET NULL, column_id → task_columns ON DELETE RESTRICT NOT NULL, position INT,
              note TEXT NOT NULL DEFAULT '' CHECK ≤ 20000, done_at TIMESTAMPTZ NULL, created_at, updated_at)
              INDEX (column_id, position)
-- 0007_task_tree.sql (B2)
ALTER TABLE tasks ADD parent_id BIGINT REFERENCES tasks ON DELETE CASCADE, CHECK (parent_id <> id);  INDEX (parent_id)
-- 0008_day_plans.sql (B3)
plan_items   (day DATE, task_id → tasks ON DELETE CASCADE, position SMALLINT, blocks SMALLINT CHECK 1..40, PRIMARY KEY (day, task_id))
-- 0009_plan_start.sql (B3): plan_items.start_min SMALLINT (0..1410, по 30 минут)
-- 0009_focus.sql (B4)
ritual_items (id, text TEXT CHECK 1..100, position INT)
focus_sessions (id, task_id → tasks ON DELETE SET NULL, task_title TEXT NOT NULL,   -- снимок: статистика переживает удаление дела
                day DATE, planned_blocks SMALLINT, started_at, work_started_at NULL, finished_at NULL,
                outcome TEXT NULL CHECK IN ('done','stopped','abandoned'), CHECK ((finished_at IS NULL) = (outcome IS NULL)))
                UNIQUE INDEX ON focus_sessions ((true)) WHERE finished_at IS NULL;   INDEX (day)
focus_breaks (id, session_id → CASCADE, started_at, ended_at NULL)  UNIQUE INDEX (session_id) WHERE ended_at IS NULL
focus_distractions (id, session_id → CASCADE, at, trigger TEXT CHECK 1..200, gave_in BOOL)
rewards      (id, title TEXT CHECK 1..100, price INT CHECK > 0, position INT, archived_at NULL, created_at)
points_ledger(id, at, delta INT CHECK <> 0, kind TEXT CHECK IN ('focus','purchase'), title TEXT,     -- снимок названия
              focus_session_id → SET NULL UNIQUE NULL, reward_id → SET NULL NULL)
```
Настройки раздела — ключи в `settings` (бэкапятся сами): `game_day_start`/`game_day_end` (минуты от полуночи, 540/1380),
`game_day_limit` (блоки, 18), `focus_setup_min` (10). Читает/пишет `GET/PUT /api/tasks/settings` с валидацией (`tasks.SettingsPatch.Apply`).

**API:**
```
# tasks
GET/POST /api/tasks/types,  PATCH/DELETE /api/tasks/types/{id},  PUT /api/tasks/types/order {ids}
GET/POST /api/tasks/columns, PATCH/DELETE /api/tasks/columns/{id} (409 column_not_empty), PUT /api/tasks/columns/order {ids}
GET    /api/tasks?done=false|true|all   → {tasks: Task[]}   (плоско, дерево строит фронт)
       Task = {id, parentId|null, title, blocks, ownBlocks, importance, deadline|null, typeId|null, columnId, position, note,
               leaf, done, doneAt|null, workSec, createdAt}
POST   /api/tasks                       {title, blocks, importance, deadline?, typeId?, columnId, note?} → 201 Task
PATCH  /api/tasks/{id}                  {title?, blocks?, importance?, deadline?: date|null, typeId?: id|null, note?, done?} → Task
POST   /api/tasks/{id}/move             {columnId, index} → 204   (одна транзакция: колонка + порядок)
POST   /api/tasks/{id}/split            {steps: [{title, blocks, importance?, deadline?, typeId?}]} → {tasks: поддерево}   (B2)
GET    /api/tasks/{id}/tree             → {tasks: поддерево корня}                                                     (B2)
DELETE /api/tasks/{id}                  → 204 (с шагами)
GET    /api/tasks/plan?from=&to=        → {today, from, to, items: [{day, taskId, start, title, done}]} (границы включительно; без границ — текущий месяц)
GET    /api/tasks/plan/{day}            → Plan (здесь day может быть today)
       Plan = {day, today, editable, items: [{task, start, blocks}], totalBlocks, settings, top: [{task, score, reasons}], leftovers: Task[]}
PUT    /api/tasks/plan/{day}            {items: [{taskId, start, blocks}]} → Plan   (замена плана; порядок по start)
POST   /api/tasks/plan/{day}/items      {taskId} → 201 Plan (добавить дело; 409 plan_task_exists)
POST   /api/tasks/plan/{day}/tasks      {title, blocks, importance, deadline?, typeId?, columnId, note?} → 201 Plan (создать и добавить)
GET/PUT /api/tasks/settings             {dayStart, dayEnd, dayLimit, setupMin}
# focus
GET    /api/focus/active                → {session: Session|null, now}
POST   /api/focus/sessions              {taskId} → 201 Session   (409 focus_active; 400 task_not_leaf | task_done)
POST   /api/focus/sessions/{id}/work | /break | /resume → Session      (409 invalid_phase)
POST   /api/focus/sessions/{id}/distractions {trigger, gaveIn} → Session
POST   /api/focus/sessions/{id}/finish  {outcome} → {session, timeline, points}   (409 session_finished)
       Session = {id, taskId|null, taskTitle, day, plannedBlocks, startedAt, workStartedAt|null, finishedAt|null, outcome|null,
                  breaks: [{startedAt, endedAt|null}], distractions: [{at, trigger, gaveIn}], now}
GET    /api/focus/triggers?q=           → {triggers: string[]}   (частые сверху)
GET/PUT /api/focus/ritual               {items: [{id?, text}]}   (PUT заменяет список)
GET/POST /api/focus/rewards, PATCH/DELETE /api/focus/rewards/{id}, PUT /api/focus/rewards/order
POST   /api/focus/rewards/{id}/buy      {dopamine?: bool} → {balance}   (409 not_enough_points)
GET    /api/focus/points?before=&limit= → {balance, entries: [...], next}
GET    /api/focus/stats?from=&to=       → {days: [{day, workSec, setupSec, breakSec, sessions, tasksDone}], tasksDone: [...], triggers: [{trigger, total, gaveIn}], totals}
```

**Фронт** (`sections/game/`): `Calendar/Calendar` (сетка из `lib/dates`), `Plan/{Plan,TaskPicker}`, `Board/{Board,Column,TaskCard,TaskDialog,TaskTags,SplitDialog,TaskGraph}`,
`Focus/{Focus,Setup,Work,Break,DistractionForm,Summary}`, `Shop/{Shop,RewardDialog,Ledger}`, `Stats/`, `Settings/`; `lib/components/FocusPill` (шапка).
Чистое (vitest): `lib/plan.layout`, `lib/taskTree.{buildTree, leaves, layout}`, `lib/focus.timeline` (зеркало Go-версии), `lib/board.move` (оптимистичный порядок).
Go (table-driven): `tasks.Score`/`Top`, `tasks.Rollup` (объём и «выполнено» родителя), `tasks.CheckSplit`, `focus.Timeline`, `focus.Points`, `focus.CanTransition`.

### 18.4 Блок C — Спорт и еда (модуль `sport`)

**Навигация (рек., ❓ C1):** раздел `sport` — Тренировки `#/sport/workouts[/<понедельник>]` · Еда `#/sport/food[/<YYYY-MM-DD>]` ·
Упражнения `#/sport/exercises[/<id>]` (справочник + прогресс) · Статистика `#/sport/stats`.

**Тренировки:**
- Сверху — неделя (Пн..Вс, ← →, «Сегодня»), сегодня — бледно-зелёный (`--c-green-soft`), в дне — названия тренировок; клик по дню — форма на эту дату.
- Тренировка: дата, название (≤ 100, подсказки из прошлых ❓ C7 — рек.: свободный текст с автодополнением, без справочника типов),
  «как прошла» (простой текст), длительность (мин) и ккал — вручную. Несколько тренировок в день — можно. Будущие даты — нет (`future_day`).
- Упражнения через «+»: название с автодополнением из справочника; новое имя попадает в справочник при сохранении (get-or-create, как теги).
  При выборе упражнения подходы **подставляются из прошлого раза** (`lastSets`), «+ подход» копирует предыдущий; всё редактируется. ❓ C3 (рек.: так).
- **Упражнения без веса (рек., ❓ C2):** у упражнения `measure`: `weight` (повторы × кг — обязательны оба), `reps` (повторы, вес — необязательный «доп. вес»),
  `time` (секунды, вес необязателен — планка с блином). Тип задаётся при создании, меняется в справочнике. Тоннаж = Σ повторы × вес (где оба есть).
- Сохранение — тренировка целиком (`PUT`, как блоки недели): упражнения и подходы заменяются списком.

**Еда:** за день — список `{текст ≤ 200, ккал?}` (Enter — добавить, как «Что классного»), подсказки из прошлого с последними ккал, сумма за день.
❓ C4 Сравнение с тренировками (рек.: показываем рядом «съел 2 100 · на тренировках 450», **без** «баланса»: часы дают только активные ккал,
без базового обмена разница вводит в заблуждение). Цель ккал в день — ❓ C5 (рек.: пока нет).

**Статистика (неделя/месяц):** ккал тренировок и еды по дням (таблица + среднее), число тренировок, упражнения (сколько раз, подходы, тоннаж),
тоннаж за тренировку/неделю. **Прогресс упражнения (рек.):** по тренировкам — макс. вес, тоннаж, лучшие повторы/время; линейный график (`chart.plot`).
Оценку 1ПМ (Эпли) — ❓ C6 (рек.: нет, макс. веса хватает).

**Apple Health (оценка, не делаем):** экспорт с iPhone (`export.zip` → `export.xml`) содержит `<Workout>` с длительностью и ккал — офлайн
разбирается потоково стандартным `encoding/xml` без зависимостей. Но файл обычно 200 МБ–2 ГБ (лимит импорта 20 МБ), нужен дедуп по времени начала
и маппинг типов тренировок. Реально, отдельной фазой C3 по запросу.

**БД:**
```sql
-- 0010_sport.sql (C1)
exercises         (id, name TEXT CHECK 1..60, measure TEXT CHECK IN ('weight','reps','time'), created_at)  UNIQUE INDEX (lower(name))
workouts          (id, day DATE, title TEXT CHECK 1..100, notes TEXT CHECK ≤ 20000, duration_min INT NULL CHECK 1..1440,
                   kcal INT NULL CHECK 0..20000, created_at, updated_at)   INDEX (day)
workout_exercises (id, workout_id → workouts ON DELETE CASCADE, exercise_id → exercises ON DELETE RESTRICT, position SMALLINT,
                   UNIQUE (workout_id, position))   INDEX (exercise_id)   -- прогресс и lastSets
workout_sets      (workout_exercise_id → CASCADE, position SMALLINT, reps INT NULL CHECK 1..10000,
                   weight DOUBLE PRECISION NULL CHECK 0..1000, seconds INT NULL CHECK 1..86400, PRIMARY KEY (workout_exercise_id, position))
-- 0011_meals.sql (C2)
meals             (id, day DATE, position SMALLINT, text TEXT CHECK 1..200, kcal INT NULL CHECK 0..10000, created_at, UNIQUE (day, position))
```

**API:**
```
GET    /api/sport/workouts?from=&to=     → {workouts: Workout[]}   (по умолчанию текущая неделя)
       Workout = {id, day, title, notes, durationMin|null, kcal|null, exercises: [{exerciseId, name, measure, sets: [{reps|null, weight|null, seconds|null}]}]}
POST   /api/sport/workouts               {day, title, notes?, durationMin?, kcal?, exercises: [{name, measure?, sets}]} → 201
PUT    /api/sport/workouts/{id}          то же → Workout;   DELETE → 204
GET    /api/sport/titles?q=              → {titles: string[]}
GET    /api/sport/exercises?q=           → {exercises: [{id, name, measure, lastSets}]}
PATCH  /api/sport/exercises/{id}         {name?, measure?} → 409 duplicate_exercise | 400 measure_conflict (есть подходы не того вида)
GET    /api/sport/exercises/{id}/progress?from=&to= → {points: [{day, maxWeight|null, volume, bestReps|null, bestSeconds|null}]}
GET    /api/sport/meals/{day}            → {day, meals: [{text, kcal|null}], eatenKcal, burnedKcal}
PUT    /api/sport/meals/{day}            {meals: [...]} → то же   (заменяет список)
GET    /api/sport/meals/suggest?q=       → {meals: [{text, kcal|null}]}
GET    /api/sport/stats?from=&to=        → {days: [{day, workouts, burnedKcal, eatenKcal|null, tonnage}], exercises: [{id, name, workouts, sets, tonnage}], avg}
       400 invalid_date | invalid_range | future_day | invalid_set | invalid_measure | empty_text | text_too_long | invalid_kcal | invalid_duration
```
Чистое: Go — `sport.ValidateSet(measure, set)`, `sport.Tonnage`, `sport.Progress`, `sport.DayTotals`; фронт — `lib/sport.{weekDays, setDefaults, formatSet, parseNumber}`.

### 18.5 Фазы v3

| Фаза | Срез | Миграция | Бэкап |
|---|---|---|---|
| A ✅ 2026-11-05 | Дерево групп: модель, агрегаты по поддереву, тренировка поддерева, `POST /groups`, перенос/удаление, теги с путём; фронт — дерево, DnD, «Переместить в…», новая группа, выбор группы в тренировке | 0005 | v3, `parentId` |
| B1 ✅ 2026-11-05 | Раздел «Игра» (`route.ts`/`nav.ts`), типы, колонки, дела, доска с DnD, настройки раздела | 0006 | `game.{types, columns, tasks}` |
| B2 ✅ 2026-11-05 | Подзадачи (split, свёртка), граф дела | 0007 | `tasks[].parentId` |
| B3 ✅ 2026-11-05 | Распорядок дня: план, шкала, лимит, топ-5, «не доделал вчера» | 0008 | `game.plans` |
| B4 ✅ 2026-11-05 (миграция 0010 — 0009 занята под время плана) | Фокус-ритуал (настройка, чек-лист, работа, перерывы, отвлечения), плашка в шапке, очки, магазин (+ дофамин, если B15 = да) | 0009 | `game.{ritual, sessions, rewards, ledger}` |
| B5 ✅ 2026-11-05 | Статистика игры, события `FocusFinished`/`TaskCompleted` → достижения | — | — |
| C1 ✅ 2026-11-06 | Раздел «Спорт»: неделя, тренировки, справочник упражнений, подстановка подходов | 0011 (сдвиг: 0010 — фокус) | `sport.{exercises, workouts}` |
| C2 ✅ 2026-11-06 | Еда, статистика спорта (месяц), прогресс упражнения | 0013 (0012 — упрощение упражнений) | `sport.meals` |

Каждая фаза — вертикальный срез; в конце: `go build/vet/test -race`, интеграционные (5444), `npm run check && npm test && npm run build`, отчёт, отметка здесь, стоп до «ок».
Перед первой миграцией (фаза A) — напомнить о бэкапе (⚙ → Данные → экспорт JSON).

### 18.5.1 Фаза A — сделано (2026-11-05)

- **Бэк:** миграция `0005_group_tree`; пакет `internal/groups/tree` (чистый, table-driven: `Depth/Height/Subtree`, `CheckMove`, `CheckNew`,
  `CheckPath`, `CheckPaths`, `Validate`, `LockKey`) — его используют groups, words и importexport (отдельный пакет, т.к. groups импортирует words).
  groups: агрегаты по поддереву одним запросом (`WITH RECURSIVE` замыкание + `count(DISTINCT …)`), `POST /api/groups`, `PATCH parentId`,
  `DELETE ?children=lift|delete`; words: теги с путём, `GET /api/words?subtree=true`; training: слова и проверка ответа — по поддереву.
  Бэкап v3 с `groups[].parentId` (второй проход `SetGroupParent`; v1/v2 с `parentId` → `invalid_backup`).
- **Фронт:** `lib/groupTree.ts` (+ тесты), дерево в «Группах» (сворачивание — localStorage `lexis.groupsCollapsed`, предки открытой группы
  разворачиваются сами), DnD, «⋯» → `MoveGroupDialog`, `NewGroupDialog`, хлебные крошки и выбор lift/delete в карточке группы,
  выбор группы в тренировке — в порядке дерева с путём предков. Шпаргалка ввода — про `#a/b`.
- **Уточнения по ходу реализации:**
  - Путь в теге **относительный**: первая часть может лежать где угодно, каждая следующая обязана быть ребёнком предыдущей
    (`#airport/gates` работает, даже если airport внутри travel). Несколько путей одной карточки проверяются вместе (`CheckPaths`):
    `#x/y #z/y` — конфликт. Строка с ошибкой не сохраняет ни слово, ни новые группы.
  - Ошибки сохранения (`group_path_conflict`, `group_too_deep`) приходят в том же `errors[]` bulk/импорта, отсортированные по строке;
    `words.SaveCards` теперь возвращает `SaveResult{Created, Merged, Errors}` (текст строки подставляет вызывающий). Коды разбора: `invalid_group_path`.
  - Имя группы (переименование, создание) не может содержать `/`. В рабочей базе таких групп не было (проверено 2026-11-05, 14 групп).
  - DnD: середина строки — внутрь, верхний/нижний край — рядом (к тому же родителю); зона «На верхний уровень» появляется при перетаскивании
    вложенной группы. Перенос на то же место не подсвечивается.
  - Выбор lift/delete в диалоге удаления показывается, только если у группы есть подгруппы. Новый код ошибки — `invalid_children`.
  - `parentId: 0` → 400 `invalid_parent` (а не «в корень»): корень — только `null`.
  - CSV-экспорт без изменений: тег = имя группы; при реимпорте слово попадает в существующую группу, где бы она ни лежала.

### 18.5.2 Фаза B1 — сделано (2026-11-05)

- **Бэк:** миграция `0006_tasks` (`task_types`, `task_columns`, `tasks`), модуль `internal/tasks` (`/api/tasks`):
  типы (CRUD + `PUT /types/order`), колонки (CRUD + `PUT /columns/order`, `DELETE /columns/{id}?to=<id>`), дела
  (`GET ?done=false|true|all`, `POST`, `PATCH` с `done`, `POST /{id}/move {columnId, beforeId}`, `DELETE`).
  Чистое (table-driven): `TypeInput.Normalize`, `TypePatch.Apply`, `ColumnName`, `TaskInput.Normalize`, `Patch.Apply`, `Reorder`, `CheckOrder`.
  Бэкап v3 — секция `game {types, columns, tasks}` (по разделам, как habits; v1/v2 с `game` → `invalid_backup`), `restored.tasks`;
  «Очистить всё» чистит и игру.
- **Фронт:** раздел `game` в шапке (иконка-геймпад), подразделы пока «Задачи» и «Настройки» — остальные появятся в своих фазах
  (`SECTIONS.game` растёт по фазам). Доска: колонки с горизонтальным скроллом, DnD карточек (вставка по серединам карточек —
  `lib/tasks.dropBefore`), оптимистичный перенос (`moveTask` — зеркало Go `Reorder`), без мыши — кнопки ← ↑ ↓ → на карточке,
  фильтр по типу и «Показать выполненные» (запоминаются, localStorage `lexis.board`), быстрое добавление внизу колонки
  (по умолчанию 1 ч, важность 5), диалог дела (объём, важность ползунком 1..10, дедлайн, тип, колонка, заметка), `#/game/board/<id>` открывает дело.
  Настройки: колонки и типы — переименование по клику, порядок ↑↓, цвет типа, удаление колонки с выбором, куда перенести дела.
- **Уточнения по ходу:**
  - Колонку «Входящие» миграция **не** создаёт (в миграции нет clock, а название зависит от языка): на пустой доске —
    форма «Создать колонку» с подставленным из i18n названием.
  - Перестановка — `beforeId`, а не индекс: на доске могут быть скрыты выполненные дела, и индекс фронта не совпал бы с БД.
  - Удаление колонки с делами (в т.ч. выполненными) — только с `?to=`: дела встают в конец целевой колонки в своём порядке; иначе 409 `column_not_empty`.
  - Удаление типа не трогает дела (`ON DELETE SET NULL`).
  - Новые коды ошибок: `invalid_type_name`, `duplicate_type`, `invalid_type`, `invalid_column`, `invalid_column_name`, `column_not_empty`,
    `invalid_title`, `invalid_blocks`, `invalid_importance`, `invalid_task`, `invalid_filter`.
  - Поля `parentId`, `leaf`, `ownBlocks`, `workSec` у `Task` — в фазах B2/B4, вместе со своими данными.

### 18.5.3 Фаза B2 — сделано (2026-11-05)

- **Бэк:** миграция `0007_task_tree` (`tasks.parent_id` → `ON DELETE CASCADE`, `CHECK (parent_id <> id)`).
  `tasks/tree.go` (чистое, table-driven): `Rollup` (лист/родитель, объём = Σ листьев, выполнено и `doneAt` = когда закрыт
  последний лист, глубина, корень), `SplitInput.Normalize` (наследование важности/дедлайна/типа), `CheckSplit`, `PlaceAfter`.
  API: `POST /api/tasks/{id}/split {steps}` → всё дерево, `GET /api/tasks/{id}/tree` → дерево от корня (обход в глубину);
  `Task` += `parentId`, `leaf`, `ownBlocks`; `blocks`/`done`/`doneAt` у родителя вычислены. Список дел фильтрует по вычисленной
  выполненности (грузится целиком — дел сотни). Бэкап: `game.tasks[].parentId` (второй проход, цикл/висячая ссылка/глубина → `invalid_backup`).
- **Фронт:** доска показывает только листья, у шага — хлебные крошки родителей (клик → граф); в диалоге дела — «Разбить на шаги»
  (`SplitDialog`: список «название + объём», Enter — следующий шаг) и «Граф дела» (`TaskGraph`: свой SVG, `lib/taskTree.layoutTree` —
  листья по слотам, родитель над серединой детей; выполненные — светло-серые, клик по узлу — открыть дело).
  Закрыли последний шаг — тост «… выполнено целиком».
- **Уточнения по ходу:**
  - `done` у дела с шагами — 409 `task_not_leaf`; выполненный лист не разбивается — 409 `task_done` (сначала вернуть в работу);
    глубина > 10 — 400 `task_too_deep`; шагов 1..30 — иначе 400 `invalid_steps`.
  - Шаги встают в колонку родителя сразу после него (родитель скрыт — шаги оказываются на его месте).
  - У родителя в диалоге правится только «первоначальная оценка» (`ownBlocks`), колонку не выбираем — шаги живут своей жизнью.
  - Пункты плана дня для родителя (B3) — появятся с планом: разбиение будет убирать родителя из планов.

### 18.5.4 Фаза B3 — сделано (2026-11-05)

- **Бэк:** миграция `0008_day_plans` (`plan_items (day, task_id) PK`, `position`, `blocks 1..40`, индекс по `task_id`).
  `tasks/plan.go` (чистое, table-driven): `Score` (формула §18.3, примеры из SPEC — в тестах), `Top` (тай-брейк: дедлайн → старше → id;
  сравнение с допуском 1e-9), `PlanInput.Normalize`, `SettingsPatch.Apply`. API: `GET/PUT /api/tasks/plan/{day}` →
  `Plan {day, editable, items: [{task, blocks}], totalBlocks, settings, top: [{task, score, reasons}], leftovers}`,
  `GET/PUT /api/tasks/settings {dayStart, dayEnd, dayLimit}` (ключи `game_day_*` в `settings`, мусор → значения по умолчанию).
  Разбиение дела на шаги удаляет его пункты из всех планов. Бэкап: `game.plans` (плоско `{day, taskId, position, blocks}`; в файлах B1/B2 ключа нет — пусто).
- **Фронт:** подраздел «Распорядок» — первый в «Игре» (`#/game/plan[/<день>]`, `DatePager` теперь пускает в будущее: `max` необязателен,
  `today` отдельно). Слева шкала: часовые отметки, дела подряд от начала дня (`lib/plan.layout`), перебор за конец дня — красная штриховка,
  у пункта — ✓, ± блоки, ↑↓, ×, DnD; клик по названию — дело на доске. Сверху — «3 ч из 9 ч» (красное при превышении + тост).
  Справа — «Не доделал» (янтарный блок), топ-5 с причинами (`просрочено`, `скоро дедлайн`, `важное`, `быстро`), «+ Любое дело» (`TaskPicker` с поиском).
  Настройки «Игры» — карточка «Распорядок дня» (начало/конец по получасам, лимит в часах).
- **Доработка 2026-11-05 (по отзыву Danik): дело — в любое время на шкале.** Миграция `0009_plan_start` (`plan_items.start_min`,
  по получасам, `CHECK 0..1410`; старые пункты разложены подряд от 9:00 по `position`). `PlanItemInput{taskId, start, blocks}`,
  `Normalize` проверяет: начало по получасам, дело целиком в сутках (400 `invalid_start`), пункты не пересекаются (400 `plan_overlap`);
  ответ — по времени. Бэкап: `plans[].start` (в файлах первой версии B3 нет — `fillPlanStarts` раскладывает подряд от 9:00, как миграция).
  Фронт (`lib/plan`, чистое): `place` — перетащенное дело на новое время; наложилось — **меняется местами** с ближайшим по началу
  из перекрытых, остаточные пересечения (дела разной длины) снимает `resolve` (последующие сдвигаются вниз, ранние не двигаются никогда);
  `resize` — ± блоки сдвигают последующие; `firstFree` — новое дело в первое свободное окно (сегодня — не раньше «сейчас»).
  Перетаскивание с призраком-предпросмотром (время по верху карточки, с учётом места захвата), кнопки ↑↓ убраны —
  без мыши стрелки ↑/↓ на выделенном деле (те же правила). Дела до начала / после конца дня — шкала расширяется, лишнее заштриховано.
- **Уточнения по ходу:**
  - «Не доделал» — открытые листья из планов **7 дней до** выбранного дня, которых нет в нём; свежие первыми, каждое дело один раз.
  - Выполненное после планирования остаётся в своём дне и пересохраняется вместе с планом; добавить новое выполненное — 400 `task_done`;
    дело с шагами — 400 `task_not_leaf`; прошлый день — 400 `past_day` (GET отдаёт его только для чтения, без топа).
  - Объём пункта по умолчанию — объём дела (не больше 40 блоков); правится на шкале по полчаса.
  - Новые коды: `past_day`, `invalid_day_bounds`, `invalid_day_limit` (+ уже были `invalid_task`, `invalid_blocks`, `too_many_items`).
  - Курсовая (0.542) в топе чуть выше «Мечты» (0.540): в таблице §18.3 обе округлены до 0.54.

### 18.5.5 Фаза B4 — сделано (2026-11-05)

- **Бэк:** миграция `0010_focus` (`ritual_items`, `focus_sessions` с частичным уникальным индексом «одна активная», `focus_breaks`
  с «одним открытым перерывом», `focus_distractions`, `rewards`, `points_ledger` с `UNIQUE focus_session_id`). Модуль `internal/focus` (`/api/focus`).
  Чистое (table-driven): `Compute` (фаза, настройка, чистая работа, перерывы, заработано/доступно/перерасход), `Points`, `CheckAction`, `CheckFinish`,
  валидация отвлечения, ритуала, награды, настроек. Связи — узкие интерфейсы: `focus.Tasks{Get, Complete}` (`*tasks.Service`, + новые
  `tasks.Service.Get/Complete`), `focus.Dopamine{AddDopamine}` (`*journal.Service`, новый `AddDopamine` + чистые `FirstFreeSlot`, `TruncateDopamine`).
  Покупка — под `pg_advisory_xact_lock`; «Я закончил» отмечает дело после коммита (ошибка — в лог, отработанное время не теряется).
  Разбить дело с активной сессией — 409 `focus_active`. Бэкап: `game.{ritual, sessions (с breaks, distractions), rewards, ledger}`.
- **Фронт:** подразделы «Фокус» и «Награды»; `lib/focus.timeline` — зеркало Go `Compute` (те же тест-случаи), `state/focus.svelte.ts` — активная
  сессия на всё приложение (поправка часов по `now`, тик раз в секунду, перечитывание при фокусе вкладки и раз в минуту), `FocusPill` в шапке
  (настройка — жёлтая, работа — зелёная, перерыв — серая; клик → «Фокус»). «▶ Начать» — у дела в распорядке (сегодня) и в диалоге дела.
  «Фокус»: настройка (обратный отсчёт, перерасход красным, чек-лист — отметки в localStorage по id сессии, «Начал делать» после всех отметок),
  работа (время, прогресс к плану, «+N сверх плана», копилка перерыва), перерыв (обратный отсчёт доступного, перерасход), отвлечения
  (подсказки — частые триггеры, «Поддался»/«Справился»), итог (работа, настройка, перерывы, отвлечения, очки). Без сессии — быстрый старт из плана дня.
  «Награды»: баланс, магазин (купить → подтверждение с галочкой «записать в дофамин дня»), правка/архив/удаление/порядок, история очков с подгрузкой.
  Настройки «Игры» — карточка «Ритуал фокуса» (по пункту на строку + минут на настройку).
- **Уточнения по ходу:**
  - Длительность настройки — `GET/PUT /api/focus/settings {setupMin}` (ключ `focus_setup_min`), а не в `/api/tasks/settings`: её читает focus.
  - Код «сессия уже завершена» — `focus_finished` (`session_finished` занят тренировкой английского, у него свой текст).
  - Баланс показывается на «Наградах» (и очки — тостом после сессии), а не над каждым подразделом «Игры».
  - Минимум работы — блоки дела в сегодняшнем плане, нет в плане — объём дела.
  - Покупка в дофамин: все 7 квадратиков заняты — покупка проходит, тост «шкала полная»; длинное название обрезается до 140 символов.
  - `Task.workSec` (время в фокусе на карточке) — в B5 вместе со статистикой.
  - Новые коды: `focus_active`, `invalid_phase`, `focus_finished`, `invalid_outcome`, `invalid_price`, `reward_archived`, `not_enough_points`, `invalid_setup_min`.

### 18.5.6 Фаза B5 — сделано (2026-11-05). Блок B закрыт.

- **События:** `eventbus.FocusFinished{SessionID, WorkSec, Outcome, At}` (focus, после коммита; отменённые не публикуются) и
  `eventbus.TaskCompleted{TaskID, At}` (tasks, когда лист стал выполненным — с доски, в распорядке или «Я закончил»; повторное `done` не дублирует).
  `tasks.NewService` и `focus.NewService` принимают `eventbus.Publisher`.
- **Достижения:** `game_tasks_10/50/200` (дел-листов с `done_at`) и `game_hours_10/50/200` (полных часов чистой работы в завершённых неотменённых
  сессиях — SQL «от начала работы до конца минус перерывы»). Подписчик achievements реагирует и на новые события; GET по-прежнему догоняет.
  В английской статистике ачивки `game_*` не показываются — они в статистике игры.
- **Статистика:** `GET /api/focus/stats?from=&to=` (по умолчанию 7 дней) → `{days[], totals, triggers (топ-10), tasksDone[], sessions[], records}`.
  Чистое: `focus.Summarize` (по дням окна, отменённые и вне окна — не в счёт, топ триггеров: частые → по алфавиту), `focus.BuildRecords`
  (всего часов и дел, лучший день и неделя с понедельника; при равенстве — более ранний). Дела — все закрытые в окне (и с доски), день — в TZ пользователя.
- **`Task.workSec`:** время в фокусе (SQL по сессиям дела), у родителя — своё + всех шагов (`Rollup`); на карточке — зелёная пилюля.
- **Фронт:** подраздел «Статистика» в «Игре»: сегодня / 7 / 30 дней (запоминается), плитки (в фокусе, дел, сессий, отвлечений «поддался/справился»,
  настройка/перерывы), столбики времени по дням (`chart.bars`), «Что сделано», «Что отвлекало» (доля «поддался» красным), сессии, рекорды, ачивки игры.
- **Уточнения по ходу:** «за день» — тот же экран с периодом «Сегодня» (без графика); ачивки «за неделю/месяц» не вводили — это рекорды (как и планировали).

### 18.5.7 Фаза C1 — сделано (2026-11-06)

- **Бэк:** миграция `0011_sport` (`exercises` с `measure` и уникальным `lower(name)`, `workouts`, `workout_exercises` — упражнение `RESTRICT`,
  `workout_sets` с `reps`/`weight`/`seconds`). Модуль `internal/sport` (`/api/sport`). Чистое (table-driven): `ValidateSet` (по мере: weight — повторы и вес,
  reps — повторы + необязательный доп. вес, time — секунды + необязательный вес; лишнее поле — ошибка), `Tonnage`, `WorkoutInput.Normalize`, `ExerciseName`.
  API: `GET /workouts?from=&to=` (по умолчанию текущая неделя), `GET/POST/PUT/DELETE /workouts[/{id}]` (PUT заменяет упражнения и подходы),
  `GET /titles?q=`, `GET /exercises?q=` (частые сверху, `uses`, `lastDay`, `lastSets` — подходы из самой свежей тренировки),
  `PATCH /exercises/{id}` (имя; мера — только если все записанные подходы подходят, иначе 400 `measure_conflict`), `DELETE /exercises/{id}` (409 `exercise_in_use`).
  Упражнение в тренировке — по имени без учёта регистра (get-or-create); мера нужна только новому, у существующего расхождение — 400 `measure_conflict`.
  Бэкап: необязательная секция `sport {exercises, workouts (с exercises → sets)}`, подходы проверяются `ValidateSet`; «Очистить всё» чистит и спорт.
- **Фронт:** раздел «Спорт» (иконка-гантель) — «Тренировки» и «Упражнения» («Еда» и «Статистика» — в C2). «Тренировки» (`#/sport/workouts[/<понедельник>]`):
  неделя Пн..Вс, сегодня — бледно-зелёный, в днях — тренировки (с ккал), «+» в прошедших днях, листание недель (в будущее — нельзя);
  под неделей — форма: название (подсказки), дата, мин, ккал, «как прошла», упражнения (автодополнение из справочника; знакомое — мера из справочника
  и подходы из прошлого раза, пока их не трогали; новое — выбор меры), подходы по мере («+ подход» копирует предыдущий), живой тоннаж.
  «Упражнения»: поиск, переименование по клику, смена меры, удаление неиспользованного, «прошлый раз» в строке.
  Чистое (vitest): `lib/sport` — `initialSets`, `nextSet`, `findExercise`, `toSet`, `toInput`, `draftTonnage`.
- **Доработка 2026-11-06 по отзыву Danik («не могу писать в блоках; нужны подходы, повторы, вес; время не нужно»):**
  - Баг ввода: эффект заполнения формы читал `entries` и перезапускался на каждое нажатие клавиши, стирая ввод. Теперь эффект читает только
    `workout` и `day`.
  - **Модель упроще:** упражнение в тренировке — одна строка `подходы × повторы × вес` (вес необязателен — подтягивания). Миграция `0012_sport_simple`:
    в `workout_exercises` — `sets 1..50`, `reps 1..10000`, `weight 0..1000 | NULL`; `workout_sets` и `exercises.measure` удалены
    (на момент миграции данных не было; правило переноса — число подходов, повторы и вес первого подхода, на время → 1 повтор — проверено на копии).
    API: `exercises: [{name, sets, reps, weight}]`; справочник — `last: {sets, reps, weight} | null` («как в прошлый раз»), `PATCH /exercises/{id} {name}`.
    Чистое: `EntryInput.Normalize`, `Tonnage(sets, reps, weight)`. Бэкап: `sport.workouts[].exercises: [{exerciseId, sets, reps, weight}]`.
  - **Страница:** неделя сверху; клик по дню — выбрать его для новой тренировки, клик по тренировке — правка; форма всегда видна
    («Добавить тренировку» → появляется в неделе): название (подсказки), дата, «Как прошла?», блок «С часов» (длительность, сожжено ккал),
    упражнения таблицей «Упражнение · Подходы · Повторы · Вес» (по умолчанию 3 × 10; знакомое — «как в прошлый раз», подсказка под именем).
  - **Итоги недели** под формой (`lib/sport.weekSummary`, vitest): тренировок, сожжено ккал (и в среднем), время, поднято (кг / т) и средние на тренировку
    (по тем, где значение есть); таблица по дням (тренировок, ккал, мин, тоннаж + итого и среднее); упражнения (раз, подходов, повторов, макс. вес, тоннаж).
- **Уточнения по ходу:** номера миграций сдвинуты (`0011` — спорт, `0012` — упрощение упражнений, `0013` — еда); тренировка без упражнений допустима (просто бег с ккал);
  поля формы — строки, вес принимает «62,5»; новые коды: `invalid_set`, `invalid_measure`, `measure_conflict`, `invalid_exercise_name`,
  `invalid_duration`, `invalid_kcal`, `duplicate_exercise`, `exercise_in_use`.

### 18.5.8 Фаза C2 — сделано (2026-11-06). Блок C и весь план v3 закрыты.

- **Бэк:** миграция `0013_meals` (`meals (day, position, text 1..200, kcal 0..10000 | NULL)`, `UNIQUE (day, position)`). В модуле `sport`:
  `GET/PUT /api/sport/meals/{day}` → `{day, meals, eatenKcal, burnedKcal}` (PUT заменяет список, пустые пункты выбрасываются, будущее — `future_day`),
  `GET /api/sport/meals/suggest?q=` (частые сверху, ккал — из самой свежей записи), `GET /api/sport/stats?from=&to=` (по умолчанию 7 дней),
  `GET /api/sport/exercises/{id}/progress?from=&to=` (по умолчанию полгода). Чистое (table-driven): `NormalizeMeals`, `SumKcal`,
  `Summarize` (все дни окна; `eatenKcal: null` — еду не записывали, а не 0; средние — только по тем, где значение есть; упражнения по тоннажу),
  `Progress` (по дням: макс. вес, тоннаж, подходы, повторы). Бэкап: `sport.meals` (в файлах C1 ключа нет — пусто).
- **Фронт:** «Спорт» — Тренировки · Еда · Упражнения · Статистика. «Еда» (`#/sport/food[/<день>]`): листание дней, «что съел» с подсказками
  (выбрал знакомое — ккал подставились), правка прямо в строке (пустой текст — удалить), плитки «Съедено» / «Сожжено на тренировках» — рядом,
  без «баланса» (C4). «Статистика»: 7 / 30 дней — плитки (тренировок и дней с ними, сожжено и в среднем на тренировку, съедено в день, время, тоннаж),
  столбики «съедено / сожжено» по дням, таблица по дням (итого и «в день»), упражнения (клик → прогресс). «Упражнения» → `#/sport/exercises/<id>`:
  график прогресса — макс. вес линией, тоннаж столбиками (без веса — повторы за день). Чистое (vitest): `lib/sport.findMeal`, `sumKcal`.
- **Уточнения по ходу:** статистика — последние 7 / 30 дней (как у «Дня» и «Игры»), а не календарная неделя — календарная уже есть в «Итогах недели»
  на «Тренировках»; 1ПМ и цель ккал не делали (C5, C6); Apple Health — не делали (оценка в §18.4 в силе).

### 18.6 Открытые вопросы (сводка)

> Все приняты по рекомендации 2026-11-05 — оставлено для истории.


- ❓ A1 порядок подгрупп — по имени · A2 глубина — 3 · A4 тренировка — всегда поддерево · A5 «повторена» — сама/предок или due=0 ·
  A6 путь в теге: создаёт, конфликт → `group_path_conflict` · A7 удаление: по умолчанию «поднять» · A9 stats/ачивка по своим словам.
- ❓ B1 колонки — свои списки + фильтр по типу · B2 модули tasks + focus · B3 дедлайн необязателен · B6 лимит — предупреждение ·
  B8 без автопереноса · B9 чек-лист на фронте · B10 настройка не блокируется · B11 done/stopped/abandoned · B12 перерасход перерыва разрешён ·
  B14 +1 очко за 30 мин чистой работы, без бонуса за дело · B15 покупка → дофамин (галочка) · B16 ачивки игры через eventbus.
- ❓ C1 подразделы · C2 `measure` weight/reps/time · C3 подстановка из прошлого раза · C4 ккал рядом без баланса · C5 цели ккал — нет ·
  C6 1ПМ — нет · C7 название тренировки — свободный текст с подсказками.
- ❓ G1 отдельная очистка игры/спорта — нет.

---

## 19. v4: мечты и большие дела (2026-11-09)

Текущий запрос v4 разрешает реализацию целиком; историческая остановка между фазами v3 (§18.5) к нему не применяется.
Существующие задачи с блоками, доска, фокус и статистика не меняются. Большие дела — отдельная модель без обязательного объёма:
перенос в распорядок и очки не добавляются автоматически.

### 19.1 Навигация и сценарии

- Раздел «Мечты» → «Мечты и желания» (`#/dreams/list`), заголовок «Что я хочу успеть».
  Создание — кнопка «Новое желание» и модальная форма с обязательным названием. Поля используют общий стиль `.input`, действия — `Button`.
  Пояснение, желаемая дата, категория и связь с большим делом — в форме деталей.
  Компактные карточки — как на доске задач: чекбокс исполнения, кликабельное название, метаданные. Правка/создание большого дела/удаление — в меню «⋯».
  Статусы: `want` / `active` / `fulfilled`; исполненное остаётся в общем списке и категории, его можно вернуть в работу.
- Категории пользовательские: название, порядок (место с 0), переименование и удаление. `categoryId: null` — «Без категории».
  Удаление категории сохраняет желания. Общий список включает все статусы и категории.
- В форме желания можно выбрать существующее большое дело или снять связь. «Создать большое дело» запрашивает отдельное
  название и атомарно создаёт пустой проект + ссылку. Тексты, даты и статусы не копируются и не синхронизируются.
  Удаление желания сохраняет проект; удаление проекта сохраняет желание без ссылки.
- «Игра → Большие дела» (`#/game/projects[/<id>]`): список проектов, отдельная доска каждого, цель/описание/дата,
  прогресс, счётчики, ближайшие доступные шаги (до 5), интерактивный SVG-граф на основном экране (обновлено по отзыву 2026-11-09).
  Цель слева → этапы → конечные дела-листья справа; своя раскладка поверх `lib/taskTree.layoutTree`, без зависимостей.
  Сплошные линии — декомпозиция, пунктирные стрелки A → B — A ждёт B. Зависимости можно скрыть.
  Клик по узлу выбирает его и показывает панель деталей; листья имеют чекбокс выполнения прямо в графе.
  «+» у узла открывает быстрый ввод подзадачи в панели; черновик сразу виден отдельным пунктирным узлом.
  Enter сохраняет шаг и оставляет форму для следующего; ответ API перерисовывает структуру/прогресс.
  Зависимость добавляется через кнопку в панели и выбор предпосылки на графе; удаляется крестиком в списке предпосылок.
  Масштаб ±, «Вместить», прокрутка и перемещение за фон; с клавиатуры — Tab/Enter/Space и нативная прокрутка стрелками.
  На узком экране панель находится под графом. Детали шага: название, пояснение, дата, родитель, порядок, зависимости.
  Перенос — выбор родителя и позиции в форме; перетаскивание узлов между родителями не добавлялось.
  С добавлением шестого раздела шапка переходит к иконкам при ширине <1100px; на телефоне список разделов прокручивается по горизонтали. Все действия доступны с клавиатуры.
  Удаление проекта/шага требует подтверждения и удаляет поддерево; зависимости от удалённых шагов снимаются с явным предупреждением.

### 19.2 Модель шагов и прогресс

- Дерево в пределах проекта, до 10 уровней. Шаг не переносится между проектами.
- `A → B` означает **A ждёт завершения B**. Зависимости родителя действуют на всех его потомков.
  Граф ожидания включает рёбра «родитель ждёт детей», явные зависимости и унаследованные зависимости.
  Проверка всего графа запрещает циклы и скрытые тупики вроде «ребёнок ждёт родителя».
- Только листья хранят `done`. Родитель завершён тогда и только тогда, когда завершены все его дети.
  Чтобы добавить ребёнка завершённому листу, нужно сначала открыть лист. Завершить заблокированный лист нельзя.
  Повторное открытие предпосылки уже завершённых зависимых шагов отклоняется: сначала открой зависимые шаги.
- **Прогресс:** каждый корневой этап имеет одинаковый вес. Вес родителя поровну делится между его непосредственными детьми,
  рекурсивно; открытый лист = 0, завершённый = 1. Итог — среднее корневых этапов × 100 с округлением половины вверх.
  Пустой проект — 0%. Например: готов один из двух корневых этапов → 50%; дробление второго на 100 открытых шагов оставляет 50%.
  Изменение структуры или добавление работы может уменьшить процент; это текущая доля завершённой структуры, не счёт наград.
- «Готово / осталось» — число конечных шагов. «Просрочено» — число незавершённых узлов с датой раньше серверного сегодня,
  включая этапы. Дата сегодня ещё не просрочена. Даты необязательные, прошедшие и будущие разрешены.
- Доступный шаг — незавершённый лист без незавершённых явных/унаследованных предпосылок.
  Следующие действия: дата (раньше — выше, без даты — в конце), позиция, id. Блокирующие шаги подписаны текстом.

### 19.3 Backend и хранение

Единый модуль `internal/ambitions` (`app.Module`, регистрация в `cmd/server/router.go`): желания и большие дела
составляют один связанный домен. `model.go` — чистая валидация/расчёт, `service.go` — команды и транзакции,
`repo.go` — отображение DTO через sqlc `store`, `module.go` — HTTP. Новых зависимостей нет.

Миграция **0014_ambitions** (старые не менялись):
- `dream_categories (id, title 1..60, position)`;
- `dreams (id, title 1..200, note ≤20000, deadline DATE NULL, status, position, category_id NULL, project_id NULL)`;
- `big_projects (id, title 1..200, note ≤20000, deadline DATE NULL, position)`;
- `project_steps (id, project_id, parent_id NULL, title 1..200, note ≤20000, deadline DATE NULL, position, done)`;
- `step_dependencies (project_id, step_id, requires_id)` с PK пары и составными FK, не допускающими связи между проектами;
- общая последовательность `ambition_ids`; идентификаторы положительные и не больше JS safe integer.

Параметризованный SQL — `queries/ambitions.sql`, генерация — `sqlc generate`. FK отложены до коммита для восстановления
дерева в любом порядке. Все изменения домена сериализованы одним `pg_advisory_xact_lock(741924)`.
Команда читает ограниченный снимок, чисто применяет изменение, валидирует всю структуру, пишет только изменившиеся сущности;
списки оставшихся id удаляют отсутствующие строки, список зависимостей заменяется только при изменении. Всё одной транзакцией.
Позиция в PUT/POST — индекс вставки (с 0, за пределами списка — ближайший край), соседи перенумеровываются атомарно.
Тексты — простой текст; дата — гражданская DATE через `clock.ParseDate/FormatDate`, сегодня — только серверный `clock` в LEXIS_TZ.

Лимиты для локального single-user приложения: 100 категорий, 200 проектов, 2000 желаний, 2000 шагов суммарно,
10000 зависимостей. Снимок читается целиком, пагинации нет. Формы зависимостей рассчитаны на умеренные списки;
для тысяч шагов может понадобиться поиск/виртуализация.

### 19.4 API

```
GET    /api/ambitions
POST   /api/ambitions/{categories|dreams|projects|steps}
PUT    /api/ambitions/{categories|dreams|projects|steps}/{id}
DELETE /api/ambitions/{categories|dreams|projects|steps}/{id}
POST   /api/ambitions/dreams/{id}/project  {title, note, deadline|null, position}
```

POST/PUT: полная форма сущности (`title, note, deadline|null, position`, для желаний `status, categoryId|null, projectId|null`,
для шагов `projectId, parentId|null, done, requires: id[]`); категория использует `title, position`.
PUT заменяет поля целиком, а не PATCH. Создание — 201, остальные операции — 200; ответ всегда актуальный снимок:
`{categories, dreams, projects, steps, dependencies, today, states, summaries}`.
`states[id] = {done, leaf, ready, blockedBy: id[], overdue, progress: 0..1}`;
`summaries[projectId] = {progress: 0..100, done, open, overdue, next: id[]}`.
400: `invalid_ambition`, `ambition_cycle`, `invalid_title`, `invalid_date`, `text_too_long`, `too_many_items`, `invalid_id`;
404: `not_found`; 409: `ambition_blocked`, `ambition_linked`. Общий формат ошибок httpx, RU/EN `errors.*`.

### 19.5 Бэкап и совместимость

`BackupVersion = 4`; новая необязательная секция `ambitions {categories, dreams, projects, steps, dependencies}`.
Экспорт использует ту же REPEATABLE READ транзакцию, что остальные разделы. Вычисляемые состояния и прогресс не хранятся.
Восстановление проходит общую строгую проверку файла и ту же валидацию структуры, затем запись в общей транзакции восстановления.
ID секции сохраняются (внешних ссылок из других разделов на них нет), последовательность только повышается выше максимума.
Неверная секция/поздняя ошибка другого раздела откатывает все пользовательские данные.
Файлы v1/v2/v3 без секции **не трогают мечты и большие дела** (Q10); `ambitions` в версии ниже 4 — `invalid_backup`.
«Очистить английский» их не затрагивает; «Очистить всё» удаляет оба раздела той же транзакцией.
Старое приложение не сможет прочитать бэкап v4 — нужна новая версия приложения.


### 19.6 Проверка реализации

2026-11-09: `sqlc generate`, `go build ./...`, `go vet ./...`, `go test ./... -race`,
интеграционные `go test -tags integration -p 1 ./... -race` на отдельном временном PostgreSQL 17 (5444) — проходят.
Проверены циклы/глубина/ссылки, унаследованные предпосылки, прогресс, перенос/порядок, удаление, конкурентное создание,
круговой бэкап, сохранение новых данных при v1/v2/v3, восстановление дерева в обратном порядке, откат поздней ошибки,
очистка английского/всего. Frontend: `npm run check` без ошибок/предупреждений, 414 vitest-тестов, `npm run build` — проходят.
Vite выдаёт существующее предупреждение `state_referenced_locally` в `lib/state/route.svelte.ts` (не менялся).
Реальная визуальная/клавиатурная проверка интерфейса не завершена: браузерный host недоступен, доступ Computer Use к Chrome не одобрен.
Рабочая БД не использовалась для тестов и не мигрировалась.


### 19.7 Переработка интерфейса по отзыву (2026-11-09)

Вместо текстового дерева больших дел — `sections/game/Projects/ProjectGraph.svelte`, `lib/projectGraph.ts` и тесты раскладки.
Вместо ряда обычных кнопок и постоянно видимого ввода мечты — `DreamRow.svelte`, диалог создания и контекстное меню.
Переиспользуются `Button`, `Icon`, `Spinner`, `Modal`, `ProgressBar`, общие `.input`, Inter и токены существующей доски задач.
Форма `sections/ambitions/Editor.svelte` приведена к виду `TaskDialog`; устаревший общий `page.css` удалён.
Модель хранения, миграции, API и бэкап не менялись.

Проверки после переработки: `npm run check` — 0 ошибок/предупреждений, `npm test` — 418 тестов,
`npm run build` — успешно (существующее предупреждение route.svelte.ts). Доступ к живому браузеру для визуального QA по-прежнему отсутствует.


## 20. v5: жизненный таймлайн и учебные карточки (2026-11-11)

### 20.1 Решения и интеграция

Расширение существующего `ambitions` для таймлайна; самостоятельный `study` для учебных материалов.
Английские `groups`, `words`, `training`, уровни, статистика и достижения не меняют семантику.
Переиспользуются чистая проверка дерева `groups/tree` (только узлы/родители/глубина), `lib/groupTree`,
`latest`, API/httpx, общие компоненты Modal/Button/ProgressBar, токены стилей и i18n RU/EN.
Данные учебных групп хранятся отдельно: английские группы имеют теги, M:N-слова и SRS, что учебным карточкам не подходит.
Зависимости согласованы пользователем: Marked, DOMPurify, KaTeX; jsdom — только для тестов рендеринга.
Версии закреплены в package.json/lock; шрифты KaTeX входят в офлайн-бандл, CDN не используется.

### 20.2 Таймлайн

`#/dreams/timeline`: горизонт `[год(server.today), min(год+9,9999)]`, сегодня берётся из `ambitions.today`
через `platform/clock` в `LEXIS_TZ`. На границе года дата браузера не используется.
Сохранённые дополнительные годы и годы существующих целей тоже видны (прошедшие планы не теряются).
Шкала горизонтально прокручивается, текущий год имеет отдельную рамку, выбранный — фон; есть переход к текущему году.
«Добавить следующий год» сохраняет год после максимального показанного. Каждый год содержит 12 месяцев.
Годовые цели — отдельный список; месяцы показывают счётчики, подробности — только выбранный месяц.
Форма создания/редактирования позволяет переносить цель в другой год, месяц или на год целиком.

Миграция `0015_timeline_study`:
- `timeline_years(year PK 1..9999)` — явно добавленные годы, не автогенерируемое десятилетие;
- `timeline_goals(id PK, year 1..9999, month NULL|1..12, title 1..200, note ≤20000, dream_id NULL)`;
- FK `dream_id → dreams ON DELETE SET NULL`, отложенный; индекс `(year,month)`.

Годовая цель имеет `month:null`. Самостоятельная цель не создаёт запись мечты.
Связь с мечтой не копирует сущность: UI читает актуальные название/статус и ссылку на большое дело из `dreams`.
`title` цели — резервное название на случай снятия связи (сохраняется при выборе/сохранении связи).
Заметка и период независимы; удаление цели не меняет мечту/проект, удаление мечты сохраняет цель без ссылки.
Статус мечты `fulfilled` и прогресс проекта показываются без новых конкурирующих статусов; самостоятельные цели статуса не имеют.
Можно связать одну мечту с несколькими периодами — это явные отдельные размещения, а не дубликаты мечты.

Новые маршруты того же сервиса и транзакционной блокировки `741924`:

```text
POST   /api/ambitions/years          {year}
POST   /api/ambitions/goals          {year, month:null|1..12, title, note, dreamId:null|id}
PUT    /api/ambitions/goals/{id}      та же полная форма
DELETE /api/ambitions/goals/{id}
```

Ответ — актуальный `Ambitions`, дополненный `timeline:{years:number[],goals:TimelineGoal[]}`.
Для связанной цели сервер устанавливает резервное название из мечты. Годы можно только добавлять;
PUT/DELETE для `years` отклоняются. Лимит 4000 целей; старые миграции не менялись.
Идентификаторы целей выделяет общая последовательность `ambition_ids`, но уникальность целей проверяется
внутри таблицы: старый бэкап без таймлайна может восстановить другие сущности с пересекающимися id.

### 20.3 Учебные группы и карточки

`#/study/cards`: дерево до 3 уровней, произвольные названия (не английские теги), родитель может иметь и свои карточки,
и подгруппы. Просмотр и тренировка родителя включают всё поддерево. В редакторе можно изменить родителя или группу карточки.
Удаление группы разрешено только без непосредственных детей и карточек (409 `study_group_not_empty`);
UI объясняет необходимость переноса. Удаление карточки требует подтверждения.

Таблицы той же новой миграции:
- `study_groups(id PK, title 1..100, parent_id NULL self FK DEFERRABLE)`;
- `study_cards(id PK, group_id NOT NULL FK DEFERRABLE, kind, front TEXT, back TEXT)`;
- `study_ids` — общая последовательность id учебных групп и карточек;
- виды `definition|theorem|remark|property|lemma`, фиксированные пять, без пользовательских типов;
- обе стороны: непустой Markdown, ≤20000 Unicode-символов каждая. При правке исходник хранится как есть;
- 500 групп / 10000 карточек; чтение ограниченного снимка, без пагинации; для больших библиотек может понадобиться поиск/виртуализация.

`study` реализует `app.Module`. SQL только `queries/study.sql` → `sqlc`, команды — в транзакции под
`pg_advisory_xact_lock(741925)`, проверка дерева/ссылок до записи. Снимок читается целиком;
запись обновляет строки и удаляет отсутствующие id, FK отложены до коммита. Циклы/глубина/висячие ссылки отклоняются.

```text
GET    /api/study                        → {groups, cards}
POST   /api/study/groups                 {title, parentId:null|id}
PUT    /api/study/groups/{id}             та же полная форма
DELETE /api/study/groups/{id}
POST   /api/study/cards                  {groupId, kind, front, back}
PUT    /api/study/cards/{id}              та же полная форма
DELETE /api/study/cards/{id}
POST   /api/study/parse                  {text} → {cards:[{record,kind,front,back,...}], errors:[{record,line,code}]}
POST   /api/study/bulk                   {groupId,text} → {groups,cards}
```

Создание — 201, остальное — 200; команды возвращают снимок, кроме parse, который ничего не пишет.
400 `invalid_study`, `invalid_id`, `too_many_items`, `invalid_json`; 404; 409 `study_group_not_empty`.
Пакет сохраняется атомарно только при отсутствии ошибок и наличии хотя бы одной карточки.

### 20.4 Окончательный пакетный синтаксис

Каждая запись начинается отдельной строкой **с колонки 0** `[тип: <тип>]` или `[type: <kind>]`.
Допустимые значения (регистр значения игнорируется):
`определение/definition`, `теорема/theorem`, `замечание/remark`, `свойство/property`, `лемма/lemma`.
Весь текст после заголовка и до **первой** отдельной строки `---` — лицевая сторона, остальное до следующего
заголовка типа — обратная. `||` не имеет структурного значения, нормы и выражения с ним сохраняются.

```text
[тип: определение]
Норма $x$
---
$$\lVert x \rVert = \sqrt{\sum_i x_i^2}$$

[тип: теорема]
Закон больших чисел
---
При подходящих условиях $\overline X_n \to \mathbb E X$.
```

Правила экранирования и границ:
- буквальная строка `---` на лицевой стороне: `\---` (стандартное Markdown-экранирование);
- буквальный заголовок типа: `\[тип: теорема]`;
- после первого разделителя все следующие `---` — содержимое обратной стороны (горизонтальная линия Markdown);
- fenced code: от 3 обратных кавычек или тильд, до 3 начальных пробелов; закрытие тем же символом, не короче открытия;
  внутри кодового блока `---` и заголовки типа не разбираются; inline-код обрабатывает Markdown-рендерер;
- для многострочной формулы в пакетном вводе открывающий/закрывающий `$$` или `\[` / `\]` — отдельные строки;
  внутри них структурные маркеры не разбираются. Незакрытый блок — ошибка, не попытка угадать содержимое;
- CRLF нормализуется в LF, внешние пустые строки сторон отделяют записи и убираются. Остальные пробелы,
  отступы, код и TeX сохраняются. В редакторе отдельных карточек текст сохраняется без такой нормализации;
- парсер — чистая Go-функция; живое превью использует тот же endpoint с debounce и отбрасыванием устаревших ответов;
- ошибки с номером записи и исходной строкой: `study_missing_header`, `study_unknown_kind`, `study_missing_separator`,
  `study_empty_side`, `study_unclosed_block`, `text_too_long`. Повторные записи не сливаются автоматически.

### 20.5 Рендеринг и тренировка

Обе стороны используют один `Markdown.svelte` → `lib/markdown.ts`:
Marked GFM (заголовки, списки, выделения, таблицы, код, ссылки/изображения, переносы строк),
расширения `$…$`, `$$…$$`, `\(…\)`, `\[…\]` → KaTeX. Блоки и inline-код не рендерят формулы.
HTML-токены экранируются; конечный HTML дополнительно обрабатывается DOMPurify, включая ссылки и MathML/SVG KaTeX.
TeX: `trust:false`, отдельные macros на каждый вызов, `maxExpand:1000`, `maxSize:20`, `throwOnError:false`.
Неподдерживаемые команды/ошибочные формулы видны красным, не выполняются. Полный LaTeX, пакеты и `\bbox` не поддерживаются;
есть `\boxed`. Внешние изображения требуют сети; встроенные библиотеки и шрифты — офлайн.
Длинные формулы, таблицы и код прокручиваются; интерфейс перестраивается в одну колонку на узком экране.
Карточка тренировки растёт по содержимому без flex-сжатия по высоте; длинный ответ прокручивается вместе со страницей,
а ширина Markdown ограничена карточкой, чтобы текст и формулы не выходили за её границы.

Из английского UX переиспользованы принцип раскрытия ответа, самооценка, прогресс-бар, выбор группы/поддерева,
временный выбор отдельных карточек и клавиши. Сессия учебных карточек — локальный снимок перемешанной очереди,
без серверной training_session, английских стратегий перевода, SRS или постоянных наборов.
С 2026-11-13 оценённые ответы сохраняются для отдельной статистики (§21).
Два направления: понятие → формулировка, формулировка → понятие/тип (метка скрыта до раскрытия).
С 2026-11-14 ошибки автоматически возвращаются в очередь до правильного ответа (§22).
После ответа — «Вспомнил / Не вспомнил», в конце счётчик;
Enter/Space раскрывают, 1/2 оценивают. Выход/перезагрузка завершают очередь, но сохраняют уже оценённые ответы.
На экране результатов есть «Повторить ещё раз» — новая перемешанная очередь того же набора (§21).

### 20.6 Бэкап и совместимость

В формате v5 (текущая версия — 6, см. §21): необязательные `study:{groups,cards}` и `ambitions.timeline:{years,goals}`.
Экспорт читает их в общей REPEATABLE READ транзакции; восстановление строго валидирует файл и выполняется
в общей транзакции вместе со всеми разделами. ID сохраняются, последовательности только повышаются.
Прогресс локальной тренировки не входит в бэкап.

Бэкапы v1–v4 принимаются. Отсутствие `study` сохраняет учебную библиотеку, отсутствие `ambitions.timeline`
сохраняет таймлайн, даже если файл содержит остальные ambitions. Удалённые восстановлением мечты отвязываются через FK.
Новые секции в версии ниже 5 — `invalid_backup`. Старая версия приложения не сможет прочесть v5.
«Очистить английский» сохраняет новые данные; «Очистить всё» удаляет их в общей транзакции, настройки остаются.
Ответ восстановления дополнен `timelineGoals`, `studyGroups`, `studyCards` (количества реально восстановленных записей).

### 20.7 Проверки реализации

2026-11-11: `sqlc generate`, `go build ./...`, `go vet ./...`, `go test ./... -race` — успешно.
Полный интеграционный набор `go test -tags integration -p 1 ./... -race` — успешно на отдельном временном
PostgreSQL 17 (`lexis-timeline-study-testdb`, порт 5444); после последних правок также проверены ambitions/study/importexport.
Покрыты периоды/связи/удаление цели, типы и пакетный парсер, конкурентное создание карточек, дерево групп,
HTTP-парсинг, круговой бэкап, старые v1–v4, обратный порядок групп, откат поздней ошибки, обе команды очистки.
Рабочая база не использовалась и не мигрировалась; тестовый контейнер остановлен после проверок.

Frontend: `npm run check` — 0 ошибок/предупреждений, `npm test` — 431 тест,
`npm run build` — успешно. Тесты jsdom проверяют HTML-инъекции, опасные URL, TeX trust/macros,
формулы и Markdown, отсутствие рендера формул внутри кода.
Vite сохраняет прежнее предупреждение `state_referenced_locally` в `route.svelte.ts`;
после добавления рендереров также предупреждает о JS-чанке >500 kB (около 869 kB, gzip 258 kB).
Визуальная проверка в живом браузере и проверка клавиатуры на реальном узком экране не выполнены:
Computer Use вернул `The browser access host is unavailable`. Адаптивная вёрстка реализована и проходит svelte-check,
но это не заменяет визуальное QA.


## 21. Повторная тренировка и статистика учебных карточек (2026-11-13)

### 21.1 Тренировка и учёт ответов

Первоначальный повтор всего набора и отдельная кнопка повтора ошибок заменены приоритетными подборками (§22):
ошибки отрабатываются автоматически внутри текущей очереди, а «Следующая подборка» выбирает минимальные актуальные баллы.
Можно также вернуться к материалам или перейти к статистике.

Каждый ответ «Вспомнил / Не вспомнил» сохраняется сразу перед переходом к следующей карточке.
Незаконченный проход тоже входит в статистику; незакрытые/только раскрытые вопросы — нет.
Повторные проходы дают новые оценённые показы: счётчик означает **показы с ответом**, не уникальные карточки.
Английский SRS/статистика/достижения не затрагиваются. Прошлые учебные тренировки не восстанавливаются задним числом:
в v5 их ответы не хранились.

Фронт создаёт UUID на ответ, блокирует повторное нажатие во время запроса и удерживает тот же UUID/оценку
при повторной отправке после ошибки. При ошибке показывает сообщение и «Повторить»; в библиотеку можно вернуться,
но текущий несохранённый ответ тогда не засчитывается. Нет скрытой очереди offline-синхронизации.
Реальная дата ответа — серверное время при первом успешном сохранении, а не часы браузера;
если сеть восстановилась позже полуночи, ранее сохранённый ответ остаётся в исходном дне.

### 21.2 Хранение и API

Новая миграция **0016_study_reviews**, без изменения старых миграций:
`study_reviews(id TEXT UUID PK, card_id BIGINT, reviewed_at TIMESTAMPTZ, known BOOLEAN, reverse BOOLEAN)`.
Индекс по `reviewed_at`. `card_id` — исторический идентификатор, без FK: удаление/перенос карточки или группы
не стирает историю. Тексты карточек и названия групп в историю не копируются.
Время берётся только из `platform/clock`. Изменения сериализуются существующей блокировкой `study` (741925).

```text
POST /api/study/reviews {id:uuid, cardId, known:boolean, reverse:boolean} → 204
GET  /api/study/stats?days=7|30|90|365 → {days:[{day,total,known}],total,known,activeDays,bestStreak}
```

Без `days` — 30. Даты включают серверное сегодня в `LEXIS_TZ`; массив заполнен и нулевыми днями.
Из БД читаются события только выбранного окна. Чистая `FillStats` группирует по гражданским датам,
считает активные дни и максимальную серию дней подряд **в выбранном периоде**.
`known/total` — самооценка пользователя, не автоматически проверенная точность.

Повторный POST с тем же id и содержимым ничего не добавляет и не меняет время.
Тот же id с другой карточкой/оценкой/направлением → 409 `invalid_study_review`.
Неверный UUID/cardId → 400 `invalid_study_review`, неверное окно → 400 `invalid_range`.
SQL — `queries/study.sql` → sqlc; общие httpx и конструкторы с инъекцией clock.

### 21.3 Интерфейс статистики

Новый подраздел **Учебные карточки → Статистика** (`#/study/stats`), RU/EN.
Карточки показателей: оценённые показы, доля/число «вспомнил», дни с занятиями, лучшая серия.
SVG-график с накопленными столбиками «вспомнил / не вспомнил», общая логика столбиков из `lib/chart`.
Календарь: колонки-недели, Пн–Вс по строкам, интенсивность из `lib/heatmap`, обычные кнопки с клавиатурным фокусом
и aria-label. Выбор даты показывает её итог; раскрываемая таблица даёт результаты активных дней.
График и календарь горизонтально прокручиваются на узкой ширине; показатели перестраиваются в две колонки.
При нулевой истории — объяснение, откуда появятся данные. Новых зависимостей нет.

### 21.4 Бэкап

`BackupVersion = 6`. Отдельная необязательная секция `studyHistory: Review[]` (даже пустой массив — явно очистить историю).
Экспорт читает её в той же REPEATABLE READ транзакции, восстановление валидирует UUID, даты и дубликаты,
затем заменяет историю в общей транзакции. Поздняя ошибка другого раздела откатывает историю вместе с остальными данными.
Бэкапы v1–v5 без `studyHistory` сохраняют историю, в том числе v5 с собственной библиотекой `study`.
Секция в версии ниже 6 отклоняется. «Очистить английский» не меняет историю; «Очистить всё» удаляет её.
Новая версия приложения нужна для чтения v6-бэкапа.

### 21.5 Проверки

2026-11-13: `sqlc generate`, `go build ./...`, `go vet ./...`, `go test ./... -race` — успешно.
Полный интеграционный набор `go test -tags integration -p 1 ./... -race` прошёл на отдельном временном PostgreSQL 17
(`lexis-study-stats-testdb`, порт 5444). Проверены конкурентные/повторные ответы, конфликт одного UUID с разными
данными, дата около полуночи, разрывы серий и пустые дни, диапазоны статистики, история в бэкапе v6,
сохранение истории при v1–v5, круговое восстановление, обе очистки и откат поздней ошибки другого раздела.
Рабочая база не менялась. После тестов временный контейнер остановлен.
Frontend: `npm run check` — 0 ошибок/предупреждений; `npm test` — 433 теста; `npm run build` — успешно.
Остаются предупреждения Vite о прежнем `state_referenced_locally` и размере общего JS-чанка.
Визуальное QA не выполнено: Computer Use снова вернул `The browser access host is unavailable`.


## 22. Учебные подборки по баллам и автоматическое повторение (2026-11-14)

### 22.1 Рейтинг и подборка

Балл = число «Вспомнил» минус число «Не вспомнил» по всей `study_reviews` для id карточки,
включая повторные ответы и оба направления. Начальное значение — 0; отрицательные значения допустимы,
нижней/верхней границы нет. Сохранённая ранее история тоже участвует. Идемпотентная отправка одного ответа
с тем же UUID не изменяет балл дважды. Новой таблицы/миграции и нового формата бэкапа не требуется:
история v6 полностью восстанавливает баллы; вычисленные значения не дублируются в файле.

Новый `internal/study/knowledge.go`, SQL только через `queries/study.sql` → sqlc:

```text
GET  /api/study/knowledge
     → [{cardId,groupId,front,kind,groupTitle,score,total,known,lastReviewedAt|null}]
POST /api/study/practice {groupId:null|id,cardIds:[],limit:1..10000}
     → {cards:[{...StudyCard,score}],available}
```

`knowledge` относится только к существующим карточкам. История удалённых карточек остаётся в календаре,
но они не попадают в уровни знаний/подборки. Период календаря не ограничивает рейтинг: рейтинг всегда за всё время.
Подборка использует REPEATABLE READ снимок карточек/групп/рейтингов одной транзакции.
`groupId:null` — вся библиотека, иначе — группа вместе с поддеревом; непустые cardIds дополнительно ограничивают набор.
Выбранный id вне группы/отсутствующий/дубликат → 400 `invalid_study`; неизвестная группа → 404.

Чистая `SelectPractice`: сортировка по `score ASC`, затем ещё не повторявшиеся,
затем `lastReviewedAt ASC`, затем `id ASC`. Берутся первые min(limit,available).
Лимит по умолчанию на фронте — 10, пользователь меняет число; сервер требует явный положительный limit.
На карточках библиотеки/тренировки виден балл. «Следующая подборка» заново запрашивает актуальный приоритет
в той же группе/временной выборке с тем же лимитом, не переиспользует устаревший порядок.

### 22.2 Очередь

Переиспользуется `lib/training.ts` из английского: QueueState/createQueue/answer/current/progress/finished
обобщены по типу карточки, английское поведение и SRS не менялись.
У учебной очереди только good/again: good удаляет голову и увеличивает done; again вставляет голову
после min(3, число оставшихся карточек). Если она последняя — она же следующая. Повтор продолжается до good.
`done + queue.length = total` на каждом шаге, total — исходный размер подборки, прогресс не растёт от ошибок.
После сохранённого ответа сначала учитывается +1/−1, затем меняется очередь; повторная отправка при ошибке сети
сохраняет прежние UUID и оценку. Каждый новый показ имеет новый UUID.
Сводка завершения: закрытые карточки, все ответы, все ошибки (поэтому число ответов может превышать размер набора).
Кнопка ручного «Повторить ошибки» не нужна: ошибки уже автоматически отработаны до завершения.

### 22.3 Статистика знаний

`studyLevel`: new — total=0; weak — score<0; learning — score от 0 до 4 при наличии ответов;
strong — score>=5. Это подписанные ориентиры самооценки, не объективная проверка знаний/интервальные уровни.
Показываются количество карточек и полосы распределения для всей текущей библиотеки за всё время.
Список десяти разобранных карточек с минимальными баллами содержит понятие (безопасный Markdown),
группу/тип, балл, число правильных/неправильных ответов. Если разобранных нет — пустое состояние.
«Отработать 10 самых слабых» запускает ту же серверную подборку по всей библиотеке, включая новые карточки с баллом 0;
направление из статистики — понятие → формулировка. Для собственного размера/направления — старт из материалов.

### 22.4 Проверки

`sqlc generate`, `go build ./...`, `go vet ./...`, `go test ./... -race` — успешно.
Интеграционные `study`/`importexport` с race на отдельном PostgreSQL 17 (`lexis-study-score-testdb`, 5444) — успешно:
суммирование +1/−1, отсутствие двойного учёта при retry, нулевые новые карточки, выбор минимальных баллов,
сохранение/восстановление истории и исключение удалённых карточек из рейтинга при сохранении дневной статистики.
Чистые тесты покрывают поддерево/выборку/лимит/приоритет и порядок при равенстве.
Frontend: check — 0 ошибок/предупреждений, 435 тестов, build — успешно.
Тесты очереди покрывают возврат после трёх вопросов, короткую очередь и единственную карточку до правильного ответа;
все существующие английские тесты проходят. Существующие предупреждения Vite о route/размере чанка остаются.
Рабочая база не менялась. Визуальный прогон в живом браузере не выполнен.

## 23. Словарные карточки и грамматика английского

Словарная сессия использует формат из §7. Исторические записи `training_sessions.strategy` и `reviews` сохраняются.
Миграция словарных таблиц не требуется: все примеры уже привязаны к словам через `word_examples.word_id`.
Формат CSV (`term,translation,example,tags`) остался прежним.

В `#/english/grammar` дерево тем переиспользует `study_groups`. Новая миграция `0017_study_grammar` добавляет
флаг `grammar` для тем и поле `grammar_content` для правил. Существующие учебные группы и карточки получают
прежний тип материала. Непустые темы защищены от удаления. Откат миграции с сохранёнными темами грамматики отклоняется.

Правило сохраняется как `study_cards.kind = 'grammar_rule'`. Поле `front` хранит название, `back` — Markdown-объяснение,
`content` в API — `{formula, examples?: [{text, translation}], mistakes?: [{incorrect, correct}]}`.
API использует существующие `/api/study/groups` и `/api/study/cards`. Темы грамматики создаются с `grammar: true`.
Родитель и дочерние темы принадлежат одной библиотеке. Математические карточки остаются в материалах и тренировке.

Пакет правил начинается с `[тип: правило]` или `[type: grammar_rule]` для каждой записи. После названия и строки
`---` идут Markdown-объяснение и необязательные строки `[формула] …`, `[пример] текст || перевод`,
`[ошибка] ошибка || исправление`. Английские маркеры — `[formula]`, `[example]`, `[mistake]`.
Маркеры распознаются с начала строки вне ограждённых блоков кода и формул. Перевод примера необязателен.
`POST /api/study/parse` получает `{text, grammar: true}` для предпросмотра. `POST /api/study/bulk` определяет
тип по группе назначения и сохраняет пакет одной транзакцией.

Читалка показывает правило целиком и листает выбранное поддерево в порядке добавления карточек.
В читалке доступны счётчик, кнопки листания и клавиши ← / → и Esc.

Полный JSON-бэкап v7 включает правила и темы вместе с учебной библиотекой. При восстановлении v1–v6
текущая грамматика сохраняется. При совпадении идентификаторов с карточками из v5/v6 переназначаются
идентификаторы грамматики и её связей, чтобы учебные карточки сохранили ссылки из истории ответов.
