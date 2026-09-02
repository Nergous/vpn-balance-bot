# Последовательный план реализации VPN Balance Bot

Этот документ является единым пошаговым планом реализации MVP. Этапы выполняются строго по порядку: следующий этап начинается только после выполнения проверок и критерия готовности предыдущего.

Каждый этап должен завершать один самостоятельный слой или пользовательский сценарий. Тесты создаются вместе с функциональностью, а не переносятся в отдельный поздний этап. Уже завершённый слой не должен переделываться позднее, кроме исправления обнаруженного дефекта или подключения через заранее определённую точку композиции.

## 1. Зафиксированные решения

- Язык: Go, актуальная стабильная версия.
- Telegram-клиент: `github.com/go-telegram/bot`.
- База данных: `database/sql` и `modernc.org/sqlite`, без CGO.
- Один процесс, Telegram long polling и одна SQLite-база.
- Деньги хранятся в целых копейках через `int64`; `float32` и `float64` для денежных расчётов запрещены.
- Временные метки хранятся как UTC Unix seconds.
- Календарные даты хранятся как `YYYY-MM-DD` и рассчитываются в `APP_TIMEZONE`.
- Финансовая история хранится в неизменяемом ledger.
- Ошибочные финансовые операции исправляются через `reversal`; старые ledger-записи не редактируются и не удаляются.
- Один администратор определяется числовым `ADMIN_TELEGRAM_ID`.
- Telegram username не используется для авторизации.
- Тесты используют временную SQLite-базу и fake Telegram client.
- Тесты не читают production `.env`, не открывают production DB и не отправляют сообщения реальным пользователям.
- MVP не включает платёжные системы, веб-панель, публичный HTTP API и управление VPN.

## 2. Архитектурные границы

### Domain

`internal/domain` содержит только бизнес-сущности, value objects, перечисления и чистые правила:

- пользователь и его статус;
- ledger entry и тип финансовой операции;
- reminder delivery и его статус;
- правила денег и календарных дат.

Domain не импортирует SQLite, Telegram, config или `database/sql`.

### Service

`internal/service/<feature>` содержит use cases и узкие интерфейсы зависимостей. Интерфейс принадлежит потребителю, а не реализации.

Примеры:

- `internal/service/account/storage.go` — операции, необходимые account service;
- `internal/service/billing/storage.go` — операции, необходимые billing service;
- `internal/service/reminder/storage.go` — операции, необходимые reminder service;
- `internal/service/reminder/sender.go` — минимальный интерфейс отправки сообщений.

`Params`, `Command` и filter-типы хранятся рядом с сервисом, который определяет их смысл. Не создавать общий пакет с несвязанными параметрами.

### Infrastructure

`internal/storage/sqlite` реализует service-интерфейсы и содержит только SQLite-детали:

- соединение и PRAGMA;
- SQL-запросы и транзакции;
- преобразование `NULL` через `sql.Null*`;
- миграции, backup и integrity check.

`internal/telegram` содержит адаптер Telegram API, handlers и преобразование Telegram updates в service commands.

### Composition root

`internal/app` создаёт concrete-зависимости и управляет lifecycle. `cmd/vpn-balance-bot/main.go` остаётся минимальным: создаёт root context, загружает config, настраивает logger и вызывает `app.Run`.

## 3. Целевая структура

```text
cmd/vpn-balance-bot/main.go

internal/app/
  app.go

internal/config/
  config.go
  errors.go
  config_test.go

internal/domain/
  user.go
  ledger.go
  reminder.go
  calendar.go
  calendar_test.go

internal/service/account/
  service.go
  storage.go
  params.go
  errors.go
  service_test.go

internal/service/billing/
  service.go
  storage.go
  service_test.go

internal/service/reminder/
  service.go
  storage.go
  sender.go
  service_test.go

internal/service/scheduler/
  scheduler.go
  scheduler_test.go

internal/storage/sqlite/
  store.go
  migrate.go
  users.go
  invites.go
  ledger.go
  billing.go
  reminders.go
  backup.go
  *_test.go

internal/telegram/
  bot.go
  client.go
  auth.go
  user_handlers.go
  admin_handlers.go
  callbacks.go
  wizard.go
  fake_test.go

internal/testutil/
  sqlite.go
  telegram.go

migrations/
  embed.go
  001_initial.sql

deploy/systemd/
  vpn-balance-bot.service

.env.example
README.md
```

Файлы можно делить дополнительно, когда конкретный файл становится трудно читать. Не создавать пустые пакеты и интерфейсы «на будущее».

## 4. Общие правила выполнения этапов

Для каждого этапа действует один порядок:

1. Создать или изменить только перечисленные в этапе компоненты.
2. Сразу добавить unit- или integration-тесты для нового поведения.
3. Запустить узкие тесты изменённого пакета.
4. Запустить `go test ./...`, `go vet ./...` и `go build ./...`.
5. Исправить ошибки до перехода к следующему этапу.
6. Зафиксировать этап отдельным логическим коммитом или небольшой серией атомарных коммитов.

Во всех тестах базы данных:

- использовать только `t.TempDir()`;
- создавать отдельный файл SQLite для каждого теста;
- не использовать `DATABASE_PATH` из окружения;
- включать те же PRAGMA, что и в приложении;
- закрывать соединение через `t.Cleanup`;
- не выполнять тесты на production DB.

## 5. Этап 0 — базовый репозиторий и CI

### Цель

Подготовить минимальный Go-проект, который собирается и проверяется CI, но ещё не содержит бизнес-функций.

### Реализация

1. Проверить наличие `README.md`, `.gitignore`, `.gitattributes`, `.editorconfig`, `LICENSE` и `.github/workflows/ci.yml`.
2. Убедиться, что git игнорирует:
   - `.env`;
   - `data/`;
   - `*.db`, `*.db-wal` и `*.db-shm`;
   - логи;
   - backups;
   - собранные бинарники;
   - `.workflow/`, если она не должна версионироваться.
3. Инициализировать Go-модуль.
4. Создать минимальный `cmd/vpn-balance-bot/main.go` без подключения будущих компонентов.
5. Настроить CI на выполнение форматирования, vet, тестов и сборки.
6. Не добавлять Telegram и SQLite зависимости до этапов, где они впервые используются.

### Проверка

```bash
go mod verify
go test ./...
go vet ./...
go build ./...
```

### Этап готов, когда

- чистый checkout собирается;
- CI выполняет Go-проверки;
- локальные данные и секреты не попадают в git.

## 6. Этап 1 — конфигурация и логирование

### Зависит от

Этапа 0.

### Цель

Получить проверенную конфигурацию и готовый `slog.Logger`, не открывая БД и не обращаясь к Telegram API.

### Файлы

- `internal/config/config.go`;
- `internal/config/errors.go`;
- `internal/config/config_test.go`;
- `internal/logger/logger.go`;
- `internal/logger/logger_test.go`;
- `.env.example`.

### Переменные окружения

- `APP_ENV`: `development`, `test` или `production`;
- `TELEGRAM_BOT_TOKEN`: обязательный непустой token;
- `ADMIN_TELEGRAM_ID`: обязательный положительный `int64`;
- `DATABASE_PATH`: обязательный путь к SQLite;
- `APP_TIMEZONE`: IANA timezone, default `Europe/Moscow`;
- `REMINDER_HOUR`: `0..23`, default `9`;
- `INVITE_TTL`: положительный duration, default `168h`;
- `LOG_LEVEL`: `debug`, `info`, `warn` или `error`, default `info`;
- `TIMEOUT`: положительный operation timeout, default `30s`.

### Порядок реализации

1. Определить `APP_ENV` до попытки загрузить `.env`.
2. В `development` разрешить локальный `.env`.
3. В `test` и `production` никогда не загружать `.env`.
4. Прочитать переменные, удалить окружающие пробелы и применить defaults.
5. Проверить числовые значения, duration, timezone и допустимый log level.
6. В production отклонить `:memory:`, временные и тестовые DB paths.
7. Не выполнять сетевой `getMe` внутри config package. Проверка доступности Telegram относится к Telegram adapter.
8. Не включать bot token, admin ID и другие секретные значения в ошибки.
9. Создать logger:
   - `TextHandler` для development и test;
   - `JSONHandler` для production;
   - `AddSource=true` только для development;
   - минимальный уровень из `LOG_LEVEL`.

### Тесты этапа

- defaults;
- отсутствие обязательных переменных;
- неправильный и неположительный admin ID;
- неправильный reminder hour;
- неизвестная timezone;
- неправильные durations;
- неизвестный log level;
- запрет unsafe production DB path;
- отсутствие `.env` загрузки в test и production;
- отсутствие секретов в тексте ошибок;
- фильтрация каждого log level;
- Text/JSON format по environment.

### Этап готов, когда

Config и logger полностью протестированы и не имеют side effects кроме чтения разрешённого environment-файла.

## 7. Этап 2 — доменные типы, деньги и календарь

### Зависит от

Этапа 1.

### Цель

Зафиксировать бизнес-типы до проектирования storage и сервисов.

### Файлы

- `internal/domain/user.go`;
- `internal/domain/ledger.go`;
- `internal/domain/reminder.go`;
- `internal/domain/calendar.go`;
- `internal/domain/calendar_test.go`.

### Порядок реализации

1. Определить `UserStatus`: `active`, `paused`, `disabled`.
2. Определить `LedgerKind`: `opening_balance`, `payment`, `subscription_charge`, `adjustment`, `reversal`.
3. Определить `ReminderStatus`: `pending`, `sent`, `failed`, `skipped`.
4. Определить `ReminderType`: `before_charge`, `charge_debt`, `overdue_3d`, `overdue_7d`, `manual`.
5. Создать `User`, `LedgerEntry` и `ReminderDelivery` без SQL- и Telegram-типов.
6. Nullable-поля выразить через указатели или собственные domain-типы; `sql.Null*` оставить SQLite adapter.
7. Зафиксировать правило: balance равен сумме `amount_minor` всех ledger entries пользователя.
8. Запретить денежные операции с `float`.
9. Реализовать вычисление следующей расчётной даты с сохранением исходного anchor day.

### Календарные примеры

- 31 января → 28 или 29 февраля → 31 марта;
- 30 января → 28 или 29 февраля → 30 марта;
- 31 марта → 30 апреля → 31 мая;
- 29 февраля високосного года → 29 марта.

### Тесты этапа

- допустимые и недопустимые enum values;
- сложение signed `int64` сумм;
- zero, positive и negative balances;
- переходы между месяцами;
- високосные годы;
- anchor days `1`, `28`, `29`, `30`, `31`;
- работа в заданной timezone без зависимости от timezone машины.

### Этап готов, когда

Domain package не импортирует infrastructure packages, а все денежные и календарные правила проходят unit-тесты.

## 8. Этап 3 — SQLite connection lifecycle

### Зависит от

Этапа 2.

### Цель

Создать надёжное соединение SQLite без таблиц и CRUD.

### Файлы

- `internal/storage/sqlite/store.go`;
- `internal/storage/sqlite/errors.go`;
- `internal/storage/sqlite/store_test.go`.

### Порядок реализации

1. Добавить `modernc.org/sqlite`.
2. Зарегистрировать driver через blank import и использовать driver name `sqlite`.
3. Реализовать `sqlite.New(ctx, path, timeout)`.
4. После `sql.Open` вызвать `SetMaxOpenConns(1)` и `SetMaxIdleConns(1)`.
5. Для соединения включить:
   - `PRAGMA foreign_keys = ON`;
   - `PRAGMA journal_mode = WAL`;
   - `PRAGMA busy_timeout = 5000`.
6. Выполнить `PingContext` с operation timeout.
7. При любой ошибке закрыть уже созданный `*sql.DB`.
8. Реализовать идемпотентный `Close` на уровне application lifecycle.
9. Не добавлять constructor в service-интерфейсы. Constructor остаётся функцией concrete SQLite package.

### Тесты этапа

- открытие БД только в `t.TempDir()`;
- неправильный или недоступный path;
- успешный `PingContext`;
- `foreign_keys=ON`;
- `journal_mode=WAL`;
- `busy_timeout=5000`;
- `SetMaxOpenConns(1)`;
- закрытое соединение больше не принимает запросы.

### Этап готов, когда

Приложение может безопасно открыть и закрыть пустую SQLite DB с production-настройками соединения.

## 9. Этап 4 — миграции и полная схема данных

### Зависит от

Этапа 3.

### Цель

Создать окончательную MVP-схему до реализации repository methods, чтобы следующие этапы не переделывали таблицы.

### Файлы

- `migrations/embed.go`;
- `migrations/001_initial.sql`;
- `internal/storage/sqlite/migrate.go`;
- `internal/storage/sqlite/migrate_test.go`;
- `internal/testutil/sqlite.go`.

### Порядок реализации

1. Хранить SQL-файлы в root package `migrations`.
2. Подключить их через `go:embed` из `migrations/embed.go`; embed path считается относительно этого package.
3. Выполнять миграции в лексикографическом порядке.
4. Создать `schema_migrations`, чтобы каждая версия применялась один раз.
5. Каждую новую миграцию выполнять в транзакции вместе с записью версии.
6. Повторный вызов `Migrate` не должен менять данные или падать.
7. Создать таблицы `users`, `invite_tokens`, `ledger_entries` и `reminder_deliveries`.
8. Добавить все `NOT NULL`, `CHECK`, `UNIQUE`, foreign keys и индексы из MVP до начала CRUD.
9. Календарные даты хранить как ISO `YYYY-MM-DD`; timestamps — UTC Unix seconds.
10. Не использовать schema prefix вроде `vpn_balance_bot.users`: SQLite-файл уже является отдельной БД.

### Обязательные ограничения схемы

- nullable unique `telegram_user_id` и `telegram_chat_id`;
- `monthly_fee_minor > 0`;
- `billing_anchor_day BETWEEN 1 AND 31`;
- только `RUB` в MVP;
- допустимые user, ledger и reminder statuses;
- `amount_minor <> 0`;
- unique `reverses_entry_id`;
- один `subscription_charge` на `user_id + billing_period_on`;
- один reminder на `user_id + billing_date + reminder_type`;
- foreign keys от invite, ledger и reminder к user.

### Тестовый helper

`internal/testutil.NewSQLite(t)` должен:

1. создать path внутри `t.TempDir()`;
2. открыть БД через тот же constructor, что production;
3. применить миграции;
4. зарегистрировать cleanup;
5. никогда не читать application config или environment DB path.

### Тесты этапа

- миграция с нуля;
- повторный `Migrate`;
- rollback всей версии при ошибке SQL;
- наличие таблиц и индексов;
- foreign key violation;
- duplicate Telegram ID;
- zero ledger amount;
- duplicate subscription charge;
- second reversal;
- duplicate reminder;
- `PRAGMA integrity_check` после миграции.

### Этап готов, когда

Пустая БД получает окончательную MVP-схему, повторная миграция безопасна, а тесты доказывают все ограничения.

## 10. Этап 5 — профили пользователей и статусы

### Зависит от

Этапа 4.

### Цель

Полностью реализовать управление профилями без Telegram handlers и финансовых операций.

### Файлы

- `internal/service/account/storage.go`;
- `internal/service/account/params.go`;
- `internal/service/account/service.go`;
- `internal/service/account/errors.go`;
- `internal/service/account/service_test.go`;
- `internal/storage/sqlite/users.go`;
- `internal/storage/sqlite/users_test.go`.

### Service API

- создать пользователя;
- получить пользователя по internal ID;
- найти по Telegram user ID;
- получить список с фильтром по status;
- изменить тариф только для будущих списаний;
- pause;
- resume с обязательной новой `next_charge_on`;
- disable.

### Порядок реализации

1. Определить узкий account storage interface рядом с account service.
2. Хранить `CreateUserParams`, filters и commands в `internal/service/account`.
3. Реализовать SQLite queries и преобразование nullable полей.
4. В service проверить display name, положительный тариф, currency, anchor day и даты.
5. `pause` и `disable` не удаляют историю и профиль.
6. `resume` требует новую дату следующего списания.
7. Смена тарифа обновляет только user profile и не меняет старые ledger entries.

### Тесты этапа

- создание и чтение;
- duplicate Telegram IDs;
- поиск по Telegram ID;
- фильтрация statuses;
- недопустимый тариф и anchor day;
- pause/resume/disable transitions;
- обязательная дата для resume;
- смена тарифа без изменения истории.

### Этап готов, когда

Account service управляет профилями через интерфейс, а SQLite является одной из его реализаций.

## 11. Этап 6 — invite tokens и привязка Telegram

### Зависит от

Этапа 5.

### Цель

Реализовать безопасную одноразовую привязку Telegram account к существующему профилю.

### Файлы

- `internal/service/account/invite.go`;
- `internal/service/account/invite_test.go`;
- `internal/storage/sqlite/invites.go`;
- `internal/storage/sqlite/invites_test.go`.

### Порядок реализации

1. Сгенерировать token через `crypto/rand`.
2. Вернуть raw token вызывающему коду только один раз.
3. Сохранить в БД только SHA-256 hash или другой зафиксированный cryptographic hash.
4. Использовать `INVITE_TTL` для `expires_at`.
5. Реализовать атомарный consume:
   - найти hash;
   - проверить `used_at IS NULL`;
   - проверить срок;
   - проверить, что профиль ещё не связан конфликтующим Telegram ID;
   - записать `telegram_user_id` и `telegram_chat_id`;
   - установить `used_at`;
   - commit.
6. Не использовать username как identifier.

### Тесты этапа

- в БД отсутствует raw token;
- успешный consume;
- повторный consume;
- expired token;
- неизвестный token;
- два конкурентных consume одного token;
- конфликт Telegram ID;
- rollback привязки при ошибке.

### Этап готов, когда

Один token может привязать один профиль ровно один раз, а повторные и просроченные попытки безопасно отклоняются.

## 12. Этап 7 — ledger и операции администратора

### Зависит от

Этапа 6.

### Цель

Реализовать финансовое ядро без Telegram UI и автоматического scheduler.

### Файлы

- `internal/service/account/ledger.go`;
- `internal/service/account/ledger_test.go`;
- `internal/storage/sqlite/ledger.go`;
- `internal/storage/sqlite/ledger_test.go`.

### Операции

- `opening_balance`;
- `payment`;
- `adjustment`;
- `reversal`;
- balance;
- последние 10 ledger entries.

### Правила

1. Balance всегда вычисляется через `SUM(amount_minor)`.
2. Отдельное изменяемое поле balance запрещено.
3. Payment принимает только положительную сумму.
4. Opening balance создаётся отдельным kind и может быть signed согласно подтверждённому начальному состоянию.
5. Adjustment требует ненулевую signed сумму и обязательный комментарий.
6. Reversal:
   - загружает исходную запись;
   - проверяет user;
   - запрещает второй reversal;
   - создаёт сумму с противоположным знаком;
   - записывает `reverses_entry_id`;
   - сохраняет admin Telegram ID и comment.
7. Создание каждой финансовой операции выполняется в транзакции.
8. Update и Delete ledger entries не добавляются в storage API.

### Тесты этапа

- positive payment;
- zero и negative payment;
- signed opening balance;
- adjustment без comment;
- корректный reversal;
- second reversal;
- reversal чужой записи;
- rollback;
- balance после последовательности операций;
- порядок и лимит последних 10 операций;
- отсутствие update/delete методов.

### Этап готов, когда

Все ручные финансовые операции выполняются без Telegram и полностью подтверждены service и SQLite integration tests.

## 13. Этап 8 — billing и восстановление пропущенных списаний

### Зависит от

Этапа 7.

### Цель

Реализовать идемпотентные subscription charges и catch-up как отдельный сервис, ещё без фонового scheduler.

### Файлы

- `internal/service/billing/storage.go`;
- `internal/service/billing/service.go`;
- `internal/service/billing/service_test.go`;
- `internal/storage/sqlite/billing.go`;
- `internal/storage/sqlite/billing_test.go`.

### Алгоритм одного периода

1. Загрузить пользователя внутри транзакции.
2. Повторно проверить status и `next_charge_on`.
3. Для `active` пользователя зарезервировать unique `user_id + billing_period_on`.
4. Создать отрицательный `subscription_charge` по тарифу, действующему в этот момент.
5. Передвинуть `next_charge_on` на один календарный месяц через domain calendar.
6. Commit.
7. Unique conflict считать уже выполненным периодом, а не fatal error.

### Catch-up

Повторять алгоритм, пока `next_charge_on` не станет позже расчётной даты запуска. Каждый период использует отдельную транзакцию, чтобы уже завершённые месяцы не терялись при ошибке следующего.

### Правила статусов

- `active`: участвует в начислениях;
- `paused`: не получает новые начисления;
- `disabled`: полностью исключён из billing selection;
- resume начинает расчёт с явно заданной новой даты.

### Тесты этапа

- одно наступившее списание;
- повторный вызов;
- два конкурентных вызова;
- три пропущенных месяца;
- смена тарифа между периодами;
- paused и disabled;
- даты `29..31`;
- rollback одного периода;
- повторный запуск после частичного catch-up;
- отсутствие дубля после открытия той же БД заново.

### Этап готов, когда

Catch-up можно безопасно вызвать сколько угодно раз, включая параллельные и повторные запуски.

## 14. Этап 9 — Telegram adapter, авторизация и пользовательское меню

### Зависит от

Этапа 8.

### Цель

Подключить Telegram transport и реализовать только пользовательские сценарии.

### Файлы

- `internal/telegram/client.go`;
- `internal/telegram/bot.go`;
- `internal/telegram/auth.go`;
- `internal/telegram/user_handlers.go`;
- `internal/telegram/callbacks.go`;
- `internal/testutil/telegram.go`.

### Порядок реализации

1. Добавить `github.com/go-telegram/bot`.
2. Создать небольшой wrapper над используемыми методами Telegram client, чтобы тесты работали без сети.
3. При создании production adapter выполнить `getMe` и проверить token; config package сеть не использует.
4. Зарегистрировать `/start`, «Мой статус», «История» и «Помощь».
5. Для `/start <invite-token>` вызвать account invite service.
6. Для каждого пользовательского handler получить numeric Telegram user ID и найти связанный профиль.
7. Неавторизованному пользователю не показывать данные.
8. Пользователь не может передавать произвольный internal user ID.
9. «Мой статус» показывает balance, currency, tariff, next charge date, debt или покрытые периоды и последнюю оплату.
10. «История» показывает максимум 10 операций с датой, kind, signed amount и comment.

### Тесты этапа

- fake client без сети;
- неизвестный пользователь;
- успешная invite-привязка;
- повторная и expired invite link;
- пользователь видит только свой профиль;
- статус при долге, нуле и предоплате;
- история ограничена 10 операциями;
- username не влияет на авторизацию;
- Telegram token не появляется в ошибках и логах.

### Этап готов, когда

Привязанный пользователь может безопасно просматривать только собственные данные через fake-tested handlers.

## 15. Этап 10 — административные handlers и wizards

### Зависит от

Этапа 9.

### Цель

Предоставить администратору UI для уже реализованных account и ledger use cases, не добавляя бизнес-логику в handlers.

### Файлы

- `internal/telegram/admin_handlers.go`;
- `internal/telegram/wizard.go`;
- `internal/telegram/admin_handlers_test.go`.

### Обязательная авторизация

Каждый admin handler и callback:

1. сравнивает numeric Telegram ID с `ADMIN_TELEGRAM_ID`;
2. отклоняет запрос до изменения состояния;
3. вызывает service method, который также получает admin Telegram ID;
4. не доверяет user ID, action или amount только из callback payload.

### Сценарии

- dashboard;
- список и карточка пользователей;
- создание пользователя;
- создание invite link;
- запись payment;
- opening balance;
- adjustment;
- reversal;
- смена тарифа;
- pause;
- resume;
- disable.

### Wizards

1. Состояние незавершённой формы хранится только в памяти.
2. До финального подтверждения БД не меняется.
3. Отмена удаляет wizard state.
4. Рестарт приложения отменяет незавершённый wizard.
5. Перед payment показываются user, amount, date, comment и итоговое действие.
6. Повторное callback-подтверждение не должно создавать вторую операцию.

### Тесты этапа

- запрет всех admin actions для обычного пользователя;
- проверка admin ID в handler и service boundary;
- успешные сценарии каждого действия;
- invalid amount/date/comment;
- отмена wizard;
- restart с потерей незавершённого wizard;
- повторное подтверждение;
- подмена callback payload.

### Этап готов, когда

Администратор выполняет все ручные MVP-операции через Telegram, а handlers остаются тонкими адаптерами.

## 16. Этап 11 — reminder persistence и reminder service

### Зависит от

Этапа 10.

### Цель

Реализовать выбор, reservation, отправку и фиксацию результата одного reminder без фонового расписания.

### Файлы

- `internal/service/reminder/storage.go`;
- `internal/service/reminder/sender.go`;
- `internal/service/reminder/service.go`;
- `internal/service/reminder/service_test.go`;
- `internal/storage/sqlite/reminders.go`;
- `internal/storage/sqlite/reminders_test.go`;
- `internal/telegram/reminder_handlers.go`;
- `internal/telegram/reminder_handlers_test.go`.

### Правила выбора

1. Пропустить `paused` и `disabled`.
2. Использовать актуальный balance после billing catch-up.
3. Выбрать максимум один наиболее важный reminder на запуск для пользователя.
4. `before_charge`: за 3 дня, если balance не покрывает ближайшее списание.
5. `charge_debt`: в billing date после списания при отрицательном balance.
6. `overdue_3d` и `overdue_7d`: при сохраняющемся долге.
7. `manual`: только по явному admin action.
8. Предоплата подавляет автоматическое предупреждение.
9. После долгого простоя не отправлять устаревшую последовательность сообщений.

### Delivery algorithm

1. Транзакционно создать `pending` delivery с unique key.
2. Если row уже существует, не отправлять повторно.
3. Отправить сообщение через `Sender` interface.
4. При успехе записать `sent`, `sent_at` и Telegram message ID.
5. При известной ошибке доставки записать `failed` и безопасный error code без текста с секретами.
6. `Forbidden` пометить отдельным error code `unreachable` и не retry автоматически.
7. Retry разрешить только для ошибок, про которые известно, что Telegram не принял сообщение.
8. Не retry автоматически ambiguous timeout, при котором сообщение могло быть принято.
9. Существующий `pending` после рестарта считать неопределённым результатом и не создавать второй automatic delivery; перевести его в `failed` с `delivery_state_unknown` для ручной проверки.
10. Добавить отдельный admin handler «Напомнить сейчас», который вызывает `manual` reminder через тот же service и повторно проверяет `ADMIN_TELEGRAM_ID`.

### Тесты этапа

- выбор каждого reminder type;
- priority при нескольких подходящих типах;
- отсутствие reminder при предоплате;
- paused и disabled;
- duplicate reservation;
- successful send;
- known temporary failure;
- ambiguous timeout;
- Forbidden;
- restart с существующим pending;
- manual reminder и проверка администратора.

### Этап готов, когда

Reminder service безопасно обрабатывает одного пользователя, manual reminder доступен только администратору, а повторная delivery для одного unique key не создаётся.

## 17. Этап 12 — scheduler

### Зависит от

Этапа 11.

### Цель

Оркестрировать готовые billing и reminder services без Telegram handler logic.

### Файлы

- `internal/service/scheduler/scheduler.go`;
- `internal/service/scheduler/scheduler_test.go`.

### Порядок запуска одного цикла

1. Определить текущую календарную дату в `APP_TIMEZONE`.
2. Выполнить billing catch-up для всех подходящих пользователей.
3. После завершения catch-up заново получить актуальные данные для reminders.
4. Обработать reminders по одному пользователю.
5. Ошибку одного пользователя записать в logger и продолжить остальных.
6. Вернуть aggregate result цикла для observability.

### Lifecycle scheduler

1. Выполнить первый catch-up сразу после старта.
2. Рассчитать следующий ежедневный запуск по `REMINDER_HOUR` в `APP_TIMEZONE`.
3. Использовать timer, который можно остановить через context.
4. Не блокировать Telegram handlers.
5. Не запускать два одновременных scheduler cycle в одном процессе.
6. Прерывать ожидание, DB-операции и безопасные retries при `ctx.Done()`.

### Тесты этапа

- immediate startup run;
- billing выполняется раньше reminder;
- расчёт следующего запуска в timezone;
- переход через DST для timezone, где он существует;
- context cancellation;
- ошибка одного пользователя не останавливает остальных;
- защита от overlapping cycles;
- повторный запуск после открытия той же БД.

### Этап готов, когда

Scheduler детерминированно выполняет готовые use cases и полностью тестируется с fake clock и fake services.

## 18. Этап 13 — финальная композиция и graceful shutdown

### Зависит от

Этапа 12.

### Цель

Один раз собрать все готовые компоненты в запускаемое приложение. До этого этапа service и adapters тестируются независимо.

### Файлы

- `cmd/vpn-balance-bot/main.go`;
- `internal/app/app.go`;
- `internal/app/app_test.go`.

### Последовательность запуска

1. В `main` создать root context через `signal.NotifyContext` для `SIGINT` и `SIGTERM`.
2. Загрузить и проверить config.
3. Создать logger и установить его default при необходимости.
4. Вызвать `app.Run(ctx, cfg, logger)`.
5. В `app.Run` открыть SQLite.
6. Применить migrations.
7. Создать account, billing, reminder и scheduler services.
8. Создать Telegram adapter и handlers.
9. Запустить long polling и scheduler как независимые задачи.
10. Если одна критическая задача завершилась с ошибкой, отменить общий child context.
11. Дождаться завершения остальных задач.
12. Остановить Telegram polling и scheduler.
13. Закрыть SQLite после остановки всех пользователей соединения.
14. Вернуть startup или runtime error в `main`.
15. Завершить процесс с ненулевым exit code при fatal error.

### Timeout context

Для отдельных DB и network operations создавать child context:

```go
operationCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
defer cancel()
```

Context-aware операции сами следят за отменой. `ctx.Err()` проверяется отдельно только когда нужно определить причину отмены или когда код самостоятельно ожидает `ctx.Done()`.

### Тесты этапа

- startup с test config, temporary DB и fake Telegram client;
- migrations выполняются до создания repositories;
- scheduler и polling запускаются после полной инициализации;
- ошибка открытия DB;
- ошибка migration;
- ошибка Telegram initialization;
- `SIGINT`/context cancellation;
- отсутствие зависших goroutine;
- DB закрывается после остановки workers;
- fatal error возвращается вызывающему коду.

### Этап готов, когда

Бинарник запускает полный bot stack и корректно освобождает все ресурсы при отмене context или fatal error.

## 19. Этап 14 — backup и обслуживание SQLite

### Зависит от

Этапа 13.

### Цель

Добавить проверяемый consistent backup без обычного копирования работающего WAL-файла.

### Файлы

- `internal/storage/sqlite/backup.go`;
- `internal/storage/sqlite/backup_test.go`.

### Порядок реализации

1. Создавать backup через поддерживаемый SQLite snapshot-механизм, например `VACUUM INTO`, а не через файловое копирование active DB.
2. Создавать snapshot во временный файл в целевом backup-каталоге.
3. Не перезаписывать существующий backup.
4. Открыть snapshot как отдельную SQLite DB.
5. Выполнить `PRAGMA integrity_check` на snapshot.
6. Только после успешной проверки переименовать временный файл в итоговое имя.
7. При ошибке удалить только созданный временный файл.
8. Реализовать отдельный `IntegrityCheck(ctx)` для основной или восстановленной БД.
9. Не включать backup и integrity operations в business service interfaces; это infrastructure lifecycle operations.

### Тесты этапа

- backup работающей WAL DB;
- snapshot содержит committed данные;
- uncommitted данные не попадают в snapshot;
- восстановление snapshot в отдельную test DB;
- `integrity_check=ok`;
- существующий destination не перезаписывается;
- context cancellation;
- cleanup временного файла при ошибке.

### Этап готов, когда

Backup восстанавливается в отдельную БД, проходит integrity check и не требует остановки production DB через небезопасное файловое копирование.

## 20. Этап 15 — deployment, документация и финальная приёмка

### Зависит от

Этапа 14.

### Цель

Подготовить уже реализованный MVP к запуску. На этом этапе не добавляются новые бизнес-функции; исправляются только дефекты, найденные финальными проверками.

### Systemd

Создать `deploy/systemd/vpn-balance-bot.service`:

- отдельный system user без shell;
- `NoNewPrivileges=true`;
- явный working directory;
- данные в `/var/lib/vpn-balance-bot/`;
- бинарник в `/opt/vpn-balance-bot/`;
- environment file `/etc/vpn-balance-bot.env` с правами `0600`;
- отдельный закрытый backup directory;
- зависимость от сети;
- restart policy с ограничением частоты;
- `SIGTERM` для graceful shutdown;
- запрет записи вне разрешённых директорий, где это совместимо с backup.

### README

Описать:

- требования;
- все environment variables без секретных примеров;
- локальный development startup;
- запуск тестов;
- сборку бинарника;
- установку systemd unit;
- создание и проверку backup;
- восстановление только в отдельную БД;
- процедуру обновления и rollback через проверенный backup.

### Финальные автоматические проверки

```bash
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./cmd/vpn-balance-bot
```

### Финальная ручная приёмка

Ручная проверка выполняется только в отдельном test environment с отдельным bot token и отдельной SQLite DB.

1. Неавторизованный пользователь не видит данные.
2. Invite token привязывает профиль один раз.
3. Привязанный пользователь видит только свой account.
4. Администратор создаёт пользователя и записывает payment.
5. Balance равен сумме ledger entries.
6. Adjustment требует comment.
7. Reversal сохраняет исходную запись.
8. Subscription charge создаётся ровно один раз.
9. Catch-up восстанавливает несколько пропущенных месяцев.
10. Даты `29..31` обрабатываются корректно.
11. Новый тариф влияет только на будущие списания.
12. Paused и disabled прекращают начисления и automatic reminders.
13. Resume использует явно заданную новую дату.
14. Предоплата и долг определяются автоматически.
15. Reminder delivery не дублируется.
16. Forbidden фиксируется как unreachable.
17. Перезапуск не меняет проведённые операции.
18. `SIGTERM` корректно останавливает polling, scheduler и DB.
19. Backup восстанавливается в отдельную DB.
20. Восстановленная DB проходит `PRAGMA integrity_check`.

### Этап готов, когда

CI зелёный, ручная приёмка пройдена в test environment, README соответствует реальному запуску, а deployment не требует незадокументированных действий.

## 21. Контрольные точки

### A — foundation

Завершены этапы 0–4: repository, config, logger, domain, SQLite connection и полная схема.

### B — account core

Завершены этапы 5–8: profiles, invites, ledger и idempotent billing работают без Telegram.

### C — Telegram MVP

Завершены этапы 9–10: пользовательские и административные сценарии работают через fake-tested Telegram adapter.

### D — automation

Завершены этапы 11–13: reminders, scheduler и полный application lifecycle собраны и протестированы.

### E — production readiness

Завершены этапы 14–15: backup/restore доказаны, deployment подготовлен, CI и ручная приёмка пройдены.

## 22. Что не делать без отдельного решения

- не добавлять веб-панель;
- не добавлять публичный HTTP API;
- не подключать платёжные системы;
- не управлять VPN;
- не добавлять мультивалютность;
- не хранить деньги в `float`;
- не хранить изменяемый balance в `users`;
- не редактировать и не удалять ledger entries;
- не использовать username как identifier;
- не создавать общий «god interface» Storage для всех сервисов;
- не помещать `sql.Null*` в domain;
- не тестировать на production DB;
- не загружать production `.env` в тестах;
- не отправлять тестовые сообщения реальному пользователю без отдельного разрешения;
- не считать network timeout доказательством того, что Telegram не принял сообщение.
