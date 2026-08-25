# Подробный план реализации VPN Balance Bot

Документ описывает последовательность реализации Telegram-бота из PROJECT_PLAN.md.

## 1. Зафиксированные решения

- Go, актуальная стабильная версия.
- Telegram-клиент: github.com/go-telegram/bot.
- SQLite: database/sql и modernc.org/sqlite, без CGO.
- Telegram long polling, один процесс и одна SQLite-база.
- Деньги хранятся в целых копейках через int64, без float.
- Временные метки хранятся в UTC.
- Расчётные даты используют APP_TIMEZONE, по умолчанию Europe/Moscow.
- Финансовая модель — неизменяемый ledger.
- Ошибки исправляются операцией reversal, старые записи не удаляются.
- Один администратор определяется числовым ADMIN_TELEGRAM_ID.
- Тесты используют временную SQLite-базу и fake Telegram client.
- MVP не включает платежи, веб-панель, публичный API и управление VPN.

## 2. Целевая структура

    cmd/vpn-balance-bot/main.go
    internal/app/
    internal/config/
    internal/domain/
    internal/service/account/
    internal/service/reminder/
    internal/storage/sqlite/
    internal/telegram/
    internal/testutil/
    migrations/
    deploy/systemd/
    .env.example
    README.md

Интерфейсы создавать только на границах service/storage и service/Telegram client, где они нужны тестам. Не создавать интерфейс для каждой структуры заранее.

## 3. Этап 0 — подготовка репозитория

### Задачи

1. Проверить наличие README.md, PROJECT_PLAN.md, .env.example, .gitignore, .gitattributes, .editorconfig, LICENSE и .github/workflows/ci.yml.
2. Проверить, что .env, SQLite, логи, backups и бинарники игнорируются git.
3. Инициализировать Go-модуль командой go mod init.
4. Добавить github.com/go-telegram/bot и modernc.org/sqlite.
5. Выполнить go mod tidy.
6. Убедиться, что data/ создаётся только локально и не коммитится.

### Результат

Появились go.mod и go.sum, пустой каркас собирается, CI начинает выполнять Go-проверки.

### Проверка

    go mod verify
    go test ./...
    go build ./...

## 4. Этап 1 — конфигурация и запуск

### Файлы

- cmd/vpn-balance-bot/main.go
- internal/config/config.go
- internal/config/config_test.go
- internal/app/app.go

### Переменные окружения

- TELEGRAM_BOT_TOKEN — обязательный токен;
- ADMIN_TELEGRAM_ID — обязательный положительный числовой ID;
- DATABASE_PATH — путь к SQLite;
- APP_TIMEZONE — IANA timezone, по умолчанию Europe/Moscow;
- REMINDER_HOUR — 0–23, по умолчанию 9;
- INVITE_TTL — по умолчанию 168h;
- LOG_LEVEL — debug, info, warn или error;
- APP_ENV — development, test или production.

### Правила

1. Не требовать .env в production: production получает окружение от systemd.
2. Не логировать bot token, admin ID и секретные значения.
3. Проверять APP_TIMEZONE через time.LoadLocation.
4. В production отклонять пустой токен, тестовый DB path, неизвестную timezone и некорректный TTL.
5. В test не загружать production .env.

### Последовательность запуска

1. Создать context с обработкой SIGINT и SIGTERM.
2. Загрузить и проверить конфигурацию.
3. Настроить log/slog.
4. Открыть SQLite.
5. Применить миграции.
6. Создать storage, сервисы и Telegram handlers.
7. Запустить scheduler и long polling.
8. При отмене context корректно остановить scheduler, polling и БД.

### Тесты

Проверить defaults, обязательные поля, неправильный ID, неправильный час, неизвестную timezone, production-валидацию и отсутствие секретов в ошибках.

### Результат

Приложение собирается, запускается с тестовой конфигурацией и завершается без зависших goroutine или открытых соединений.

## 5. Этап 2 — SQLite и миграции

### Файлы

- migrations/001_initial.sql;
- internal/storage/sqlite/store.go;
- internal/storage/sqlite/users.go;
- internal/storage/sqlite/ledger.go;
- internal/storage/sqlite/reminders.go;
- internal/storage/sqlite/backup.go;
- internal/testutil/sqlite.go.

Миграции подключить через go:embed. Повторное применение должно быть безопасным.

### Настройка SQLite

Включить foreign_keys=ON, journal_mode=WAL и busy_timeout=5000. Использовать SetMaxOpenConns(1).

### Таблица users

Поля:

- id;
- nullable unique telegram_user_id;
- nullable unique telegram_chat_id;
- display_name и nullable username;
- monthly_fee_minor больше нуля;
- currency, в MVP RUB;
- billing_anchor_day от 1 до 31;
- next_charge_on;
- status: active, paused, disabled;
- created_at и updated_at.

### Таблица invite_tokens

Поля user_id, token_hash, expires_at, used_at и created_at. Исходный токен не хранить. Токен одноразовый, срок по умолчанию 7 дней.

### Таблица ledger_entries

Поля id, user_id, kind, signed amount_minor, occurred_at, billing_period_on, reverses_entry_id, created_by_telegram_id, note и created_at.

kind принимает opening_balance, payment, subscription_charge, adjustment или reversal.

Правила:

- нулевые операции запрещены;
- записи после создания не редактируются;
- удаление ledger-записей запрещено на уровне storage;
- reverses_entry_id уникален;
- для subscription_charge уникальны пользователь и billing date.

### Таблица reminder_deliveries

Поля user_id, billing date, reminder type, scheduled date, status, sent_at, Telegram message ID, error code, created_at и updated_at.

status принимает pending, sent, failed или skipped. Уникальность: пользователь, расчётная дата и тип напоминания.

### Storage API

Реализовать:

- создание, получение и фильтрацию пользователей;
- поиск по Telegram ID;
- создание и одноразовое использование invite token;
- получение баланса и последних 10 операций;
- payment, opening balance, adjustment и reversal;
- резервирование subscription charge;
- выбор пользователей для scheduler;
- создание и обновление reminder delivery;
- согласованный SQLite backup;
- PRAGMA integrity_check.

### Тестовый helper

Создавать БД только в t.TempDir(), применять миграции, использовать те же SQLite-настройки и никогда не читать production .env или production DB path.

### Проверка

    go test ./internal/storage/sqlite/...

Проверить foreign keys, уникальность Telegram ID, одноразовость токенов, повторные миграции, дубль списания, второй reversal, rollback и backup/restore на отдельной БД.

## 6. Этап 3 — доменная модель и ledger

### Файлы

- internal/domain/user.go;
- internal/domain/ledger.go;
- internal/domain/billing.go;
- internal/service/account/service.go;
- internal/service/account/service_test.go.

### Баланс и деньги

Баланс всегда вычисляется как сумма ledger entries. Отдельное изменяемое поле balance не хранить. Суммы проверять как int64 minor units.

### Payment

Сервис проверяет пользователя, положительность суммы и дату, затем транзакционно создаёт положительную операцию с комментарием и Telegram ID администратора.

### Opening balance

Начальный баланс создаётся отдельной операцией opening_balance.

### Adjustment

Adjustment требует ненулевую signed сумму и обязательный комментарий. Старые операции не меняются.

### Reversal

1. Загрузить исходную операцию.
2. Проверить пользователя.
3. Проверить отсутствие предыдущего reversal.
4. Создать противоположную операцию.
5. Связать её через reverses_entry_id.
6. Сохранить комментарий и администратора.

### Тарифы и статусы

Новый тариф действует только для будущих списаний. Старые subscription_charge не пересчитываются.

Paused запрещает новые списания и напоминания. Disabled исключается из scheduler. Resume требует новую дату списания. История никогда не удаляется.

### Календарь

Реализовать вычисление следующей даты:

- дни 1–28 сохраняются;
- для 29–31 в коротком месяце используется последний день;
- исходный anchor day сохраняется;
- в следующем месяце снова используется исходный anchor day.

Обязательные примеры: 31 января → 28 февраля → 31 марта; 30 января → 28 февраля → 30 марта; 31 марта → 30 апреля → 31 мая; 29 февраля високосного года → 29 марта.

### Проверка

    go test ./internal/domain/... ./internal/service/account/...

Использовать table-driven tests для денег, календаря, платежей, adjustment, reversal, смены тарифа и статусов.

## 7. Этап 4 — автоматические списания

### Алгоритм

1. Найти активных пользователей с наступившей датой.
2. Открыть транзакцию.
3. Повторно проверить состояние пользователя.
4. Зарезервировать уникальную пару user + billing date.
5. Создать отрицательную subscription_charge по текущему тарифу.
6. Передвинуть next_charge_on на календарный месяц.
7. Зафиксировать транзакцию.
8. Повторить для следующего пропущенного периода.

Конфликт уникальности считать уже выполненным списанием и обрабатывать безопасно.

### Проверки

- одно наступившее списание;
- два параллельных запуска;
- повторный запуск;
- три пропущенных месяца;
- смена тарифа между периодами;
- paused и disabled;
- даты 29–31;
- rollback при ошибке;
- отсутствие дубля после рестарта.

## 8. Этап 5 — авторизация и пользовательское меню

### Файлы

- internal/telegram/bot.go;
- internal/telegram/auth.go;
- internal/telegram/user_handlers.go;
- internal/telegram/callbacks.go.

### Invite flow

1. Администратор создаёт профиль.
2. Сервис генерирует криптографически случайный токен.
3. В БД сохраняется только hash.
4. Пользователю выдаётся ссылка /start <invite-token>.
5. Токен проверяется по hash, сроку и used_at.
6. Telegram user ID и chat ID привязываются транзакционно.
7. Токен помечается использованным.
8. Повторная и просроченная ссылка отклоняются.

Username не использовать как ключ авторизации.

### Пользовательские handlers

Реализовать /start, «Мой статус», «История» и «Помощь».

Каждый handler получает числовой Telegram ID, ищет только привязанный профиль, запрещает доступ без привязки и не принимает финансовые изменения от пользователя.

Статус показывает баланс, валюту, тариф, дату списания, долг или число покрытых будущих списаний и последнюю оплату.

История показывает последние 10 операций с датой, типом, суммой и комментарием.

## 9. Этап 6 — административные сценарии

### Повторная проверка прав

Каждый admin handler и callback:

1. проверяет ADMIN_TELEGRAM_ID;
2. отклоняет запрос до бизнес-операции;
3. повторно проверяет права в сервисе;
4. не доверяет ID только из callback.

### Сценарии

Реализовать dashboard, список и карточку пользователей, создание пользователя, запись оплаты, adjustment, reversal, смену тарифа, pause, resume, disable и «Напомнить сейчас».

### Wizards

Многошаговые формы хранить только в памяти. До подтверждения БД не менять. Отмена удаляет состояние. Рестарт отменяет wizard. Повторное подтверждение безопасно и не создаёт дубль.

Перед сохранением оплаты показать пользователя, сумму, дату, комментарий и итоговое действие.

## 10. Этап 7 — напоминания

### Типы

Зафиксировать типы before_charge, charge_debt, overdue_3d, overdue_7d и manual.

### Алгоритм дня

1. Восстановить необходимые списания.
2. Вычислить актуальный баланс.
3. Пропустить paused и disabled.
4. Определить подходящие напоминания.
5. Выбрать максимум одно наиболее важное.
6. Зарезервировать delivery транзакционно.
7. Отправить сообщение.
8. Сохранить результат.

Уникальный ключ user + billing date + reminder type защищает от повторной отправки.

Обработать успешную отправку, временную ошибку, Forbidden, рестарт между reservation и отправкой и повторный запуск после sent.

Правила: за 3 дня при нехватке денег, в день списания при отрицательном балансе, через 3 и 7 дней долга. Предоплата подавляет предупреждение. После простоя не отправлять устаревшую цепочку.

## 11. Этап 8 — scheduler и lifecycle

Scheduler выполняет catch-up сразу после старта и ежедневный запуск в APP_TIMEZONE. Он не должен блокировать Telegram handlers.

Ошибки одной записи не останавливают остальных. Транзакции откатываются при ошибке. Временные Telegram-ошибки получают ограниченный retry. Forbidden фиксируется как unreachable. Отмена context прерывает ожидание и отправку.

## 12. Этап 9 — тестирование

### Unit-тесты

Покрыть конфигурацию, календарь, денежный формат, баланс, payment, adjustment, reversal, смену тарифа, pause/resume/disable, invite tokens и выбор напоминаний.

### Integration-тесты SQLite

Проверить миграции с нуля, повторные миграции, foreign keys, уникальные индексы, rollback, идемпотентность списаний, reservation reminders, backup/restore и PRAGMA integrity_check.

### Telegram-тесты

Использовать fake client без сети. Проверить неавторизованного пользователя, изоляцию счетов, запрет admin action, проверку администратора, повторное подтверждение, отмену wizard и Forbidden.

### Изоляция

- APP_ENV=test;
- отдельный тестовый token;
- SQLite только в t.TempDir();
- production .env не загружается;
- сетевых запросов в Telegram нет;
- production service и production DB не запускаются и не изменяются.

### Команды

    gofmt -l .
    go vet ./...
    go test ./...
    go test -race ./...
    go build ./cmd/vpn-balance-bot

## 13. Этап 10 — backup и deployment

### Backup

Не копировать работающий WAL обычной файловой командой. Перед обновлением создать согласованный backup, восстановить его в отдельную тестовую БД и выполнить PRAGMA integrity_check.

### Systemd

Создать deploy/systemd/vpn-balance-bot.service с отдельным пользователем без shell, NoNewPrivileges, рабочим каталогом, отдельным каталогом данных, secret env file с правами 0600, restart policy, graceful stop и зависимостью от сети.

Целевые пути:

- бинарник: /opt/vpn-balance-bot/;
- данные: /var/lib/vpn-balance-bot/;
- секреты: /etc/vpn-balance-bot.env;
- backups: отдельный закрытый каталог.

## 14. Сквозная приёмка MVP

MVP готов, когда:

1. Неавторизованный пользователь не видит данные.
2. Привязанный пользователь видит только свой счёт.
3. Администратор записывает оплату с суммой и датой.
4. Баланс равен сумме ledger entries.
5. Списание создаётся ровно один раз.
6. Пропущенные месяцы восстанавливаются.
7. Дни 29–31 обрабатываются корректно.
8. Новый тариф влияет только на будущие списания.
9. Ошибка исправляется reversal без удаления истории.
10. Предоплата и долг определяются автоматически.
11. Напоминания не дублируются.
12. Paused и disabled прекращают начисления и сообщения.
13. Перезапуск не меняет проведённые операции.
14. Backup восстанавливается в отдельную тестовую БД.
15. Все проверки CI проходят.

## 15. Контрольные точки

### A — запускаемый каркас

Есть go.mod, сборка, конфигурация, shutdown и работающий CI.

### B — надёжное хранилище

Миграции создают схему, ограничения проверены, storage покрыт тестами, backup/restore доказаны.

### C — корректные деньги

Ledger и расчёты работают без Telegram, catch-up идемпотентен, авторизация закрывает чужие данные.

### D — рабочий бот администратора

Администратор выполняет MVP-операции, wizard не меняет БД до подтверждения, списания и reminders защищены от дублей.

### E — production readiness

Scheduler переживает рестарт, backup/restore проверены, systemd подготовлен, README соответствует запуску, CI зелёный, ручная приёмка выполнена на отдельном тестовом окружении.

## 16. Что не делать до отдельного решения

- не добавлять веб-панель;
- не добавлять HTTP API;
- не подключать платёжные системы;
- не управлять VPN;
- не добавлять мультивалютность;
- не хранить деньги в float;
- не удалять ledger entries;
- не использовать username как идентификатор;
- не тестировать на production database;
- не отправлять тестовые сообщения реальному пользователю без отдельного разрешения.
