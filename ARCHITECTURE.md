# Архитектура проекта Schulze Election Telegram Bot

## Оглавление
1. [Общий обзор](#общий-обзор)
2. [Архитектура слоев](#архитектура-слоев)
3. [Модели данных](#модели-данных)
4. [Слой базы данных (DB Layer)](#слой-базы-данных-db-layer)
5. [Слой бизнес-логики (Chain Layer)](#слой-бизнес-логики-chain-layer)
6. [Слой бота (Bot Layer)](#слой-бота-bot-layer)
7. [Модуль Schulze](#модуль-schulze)
8. [Вспомогательные модули](#вспомогательные-модули)
9. [Флоу работы системы](#флоу-работы-системы)
10. [Диаграммы взаимодействия](#диаграммы-взаимодействия)

---

## Общий обзор

**Schulze Election Telegram Bot** — это система для проведения электронного голосования среди студентов с использованием метода Шульце. Система построена на трехслойной архитектуре и использует PostgreSQL для хранения данных.

### Технологический стек
- **Язык**: Go 1.23.4
- **База данных**: PostgreSQL 15.1
- **Telegram API**: go-telegram-bot-api/v5
- **Миграции**: goose
- **Контейнеризация**: Docker, Docker Compose
- **Туннелирование**: ngrok (для получения webhook от Telegram)

### Основные возможности
1. **Регистрация делегатов** через email верификацию
2. **Голосование** с ранжированием кандидатов
3. **Подсчет результатов** по методу Шульце
4. **Административная панель** для управления выборами
5. **Логирование** всех действий с отправкой в Telegram

---

## Архитектура слоев

Проект следует принципам **Clean Architecture** с разделением на слои:

```
┌─────────────────────────────────────────────────────┐
│                   Presentation Layer                 │
│              (Telegram Bot Interface)                │
│                  internal/bot/                       │
└─────────────────────┬───────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────┐
│                  Business Logic Layer                │
│           (Transactions & Validation)                │
│                 internal/chain/                      │
└─────────────────────┬───────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────┐
│                  Data Access Layer                   │
│              (Database Operations)                   │
│                   internal/db/                       │
└─────────────────────┬───────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────┐
│                    PostgreSQL                        │
└─────────────────────────────────────────────────────┘
```

### Дополнительные модули

```
internal/
├── bot/           # Presentation Layer - обработка команд Telegram
├── chain/         # Business Layer - бизнес-логика с транзакциями
├── db/            # Data Layer - работа с PostgreSQL
├── models/        # Data Models - структуры данных
├── schulze/       # Algorithm Module - алгоритм Шульце
├── email/         # Email Module - отправка email
└── logger/        # Logger Module - кастомное логирование
```

---

## Модели данных

Файл: `internal/models/models.go`

### Delegate (Делегат)
```go
type Delegate struct {
    DelegateID int           // Шестизначный код из st-email (stXXXXXX)
    TelegramID sql.NullInt64 // ID в Telegram (nullable до верификации)
    Name       string        // ФИО делегата
    Group      string        // Группа (формат: XX.(Б|М)XX-пу)
    HasVoted   bool          // Флаг: проголосовал ли
}
```

**Назначение**: Представляет избирателя (студента), имеющего право голоса.

**Жизненный цикл**:
1. Создается администратором (`AddDelegate`)
2. Верифицируется пользователем через email (`VerificateDelegate`)
3. Голосует (`AddVote` → HasVoted = true)

---

### Candidate (Кандидат)
```go
type Candidate struct {
    CandidateID int    // Шестизначный код
    Name        string // ФИО кандидата
    Course      string // "X бакалавриат" или "X магистратура"
    Description string // Дополнительная информация
    IsEligible  bool   // Допущен ли к выборам
}
```

**Назначение**: Представляет кандидата на выборах.

**Жизненный цикл**:
1. Создается администратором (`AddCandidate`)
2. Может быть заблокирован (`BanCandidate` → IsEligible = false)
3. Участвует в подсчете, если IsEligible = true

---

### Vote (Голос)
```go
type Vote struct {
    ID                int       // Уникальный ID записи
    DelegateID        int       // Кто проголосовал
    CandidateRankings []int     // Ранжированный список кандидатов [1й, 2й, 3й...]
    CreatedAt         time.Time // Время голосования
}
```

**Назначение**: Хранит ранжированный список кандидатов от делегата.

**Структура CandidateRankings**:
```
[301234, 305678, 309012]
   ↓        ↓        ↓
 1-е место 2-е    3-е место
```

---

### Result (Результат)
```go
type Result struct {
    ID                int                 // Уникальный ID
    Course            string              // Курс ("1 бакалавриат", "Global Top" и т.д.)
    WinnerCandidateID []int               // Массив победителей (в случае ничьи > 1)
    Preferences       map[int]map[int]int // Матрица парных предпочтений
    StrongestPaths    map[int]map[int]int // Матрица сильнейших путей Шульце
    Stage             string              // "absolute", "tie-breaker", "tie"
}
```

**Назначение**: Хранит результаты подсчета для каждого курса/категории.

**Preferences** (матрица d[i][j]):
- `d[A][B] = 5` означает: 5 делегатов предпочли A перед B

**StrongestPaths** (матрица p[i][j]):
- `p[A][B] = 8` означает: сильнейший путь от A к B имеет силу 8

---

## Слой базы данных (DB Layer)

**Расположение**: `internal/db/`

**Назначение**: Прямая работа с PostgreSQL. Выполняет CRUD операции без бизнес-логики.

### Структура Storage
```go
type Storage struct {
    dbpool *pgxpool.Pool // Пул соединений PostgreSQL
}
```

### Файлы и ответственность

#### 1. `init.go` - Инициализация
```go
func NewStorage(dsn string) (*Storage, error)
func (s *Storage) Close()
func (s *Storage) Ping(ctx context.Context) error
func (s *Storage) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
```

**Зачем**: Создает пул соединений, проверяет подключение, управляет транзакциями.

---

#### 2. `delegates.go` - Операции с делегатами
```go
func (s *Storage) AddDelegate(ctx, tx, delegate) error
func (s *Storage) GetDelegateByDelegateID(ctx, tx, delegateID) (*Delegate, error)
func (s *Storage) GetDelegateByTelegramID(ctx, tx, telegramID) (*Delegate, error)
func (s *Storage) GetAllDelegates(ctx, tx) ([]Delegate, error)
func (s *Storage) UpdateDelegate(ctx, tx, delegate) error
func (s *Storage) DeleteDelegate(ctx, tx, delegateID) error
```

**SQL операции**:
- `INSERT INTO delegates`
- `SELECT * FROM delegates WHERE ...`
- `UPDATE delegates SET ...`
- `DELETE FROM delegates WHERE ...`

**Особенности**:
- Все операции принимают транзакцию `pgx.Tx`
- Возвращают `nil` при `pgx.ErrNoRows` (не найдено)

---

#### 3. `candidates.go` - Операции с кандидатами
```go
func (s *Storage) AddCandidate(ctx, tx, candidate) error
func (s *Storage) GetCandidateByCandidateID(ctx, tx, candidateID) (*Candidate, error)
func (s *Storage) GetAllCandidates(ctx, tx) ([]Candidate, error)
func (s *Storage) GetAllEligibleCandidates(ctx, tx) ([]Candidate, error)
func (s *Storage) UpdateCandidate(ctx, tx, candidate) error
func (s *Storage) DeleteCandidate(ctx, tx, candidateID) error
```

**Особенность**:
- `GetAllEligibleCandidates` фильтрует только `is_eligible = TRUE`

---

#### 4. `votes.go` - Операции с голосами
```go
func (s *Storage) AddVote(ctx, tx, vote) error
func (s *Storage) GetVoteByDelegateID(ctx, tx, delegateID) (*Vote, error)
func (s *Storage) GetAllVotes(ctx, tx) ([]Vote, error)
func (s *Storage) UpdateVote(ctx, tx, vote) error
func (s *Storage) DeleteVote(ctx, tx, voteID) error
```

**Особенность**:
- PostgreSQL поддерживает массивы: `candidate_rankings integer[]`
- Обновление голоса по `delegate_id` (не по `id`)

---

#### 5. `results.go` - Операции с результатами
```go
func (s *Storage) AddResult(ctx, tx, result) error
func (s *Storage) GetResultByCourse(ctx, tx, course) (*Result, error)
func (s *Storage) GetAllResults(ctx, tx) ([]Result, error)
func (s *Storage) UpdateResult(ctx, tx, result) error
func (s *Storage) DeleteResult(ctx, tx, resultID) error
```

**Особенность**:
- Матрицы `Preferences` и `StrongestPaths` хранятся как JSON:
  ```go
  json.Marshal(result.Preferences) → jsonb в PostgreSQL
  ```

---

### Паттерны использования

#### Пример: Получение делегата
```go
// 1. Начинаем транзакцию
tx, _ := storage.BeginTx(ctx, pgx.TxOptions{})
defer tx.Rollback(ctx)

// 2. Выполняем операцию
delegate, err := storage.GetDelegateByDelegateID(ctx, tx, 123456)

// 3. Коммитим (в chain layer)
tx.Commit(ctx)
```

**Важно**: DB слой **не начинает** и **не коммитит** транзакции — это делает Chain layer.

---

## Слой бизнес-логики (Chain Layer)

**Расположение**: `internal/chain/`

**Назначение**: Управляет транзакциями, выполняет валидацию, реализует бизнес-правила.

### Структура VoteChain
```go
type VoteChain struct {
    storage storage // Интерфейс к DB layer
}
```

### Интерфейс storage
Определяет контракт с DB layer (все методы из `internal/db/`).

---

### Файлы и ответственность

#### 1. `init.go` - Инициализация
```go
func NewVoteChain(storage storage) *VoteChain
```

Определяет интерфейс `storage` с методами DB layer.

---

#### 2. `delegate.go` - Бизнес-логика делегатов

```go
func (vc *VoteChain) AddDelegate(ctx, delegate) error
```
**Флоу**:
1. Начинает транзакцию (Serializable)
2. Проверяет, существует ли делегат
3. Если существует → возвращает ошибку
4. Добавляет делегата
5. Коммитит транзакцию

```go
func (vc *VoteChain) VerificateDelegate(ctx, delegateID, telegramID) error
```
**Флоу**:
1. Начинает транзакцию
2. Получает делегата по delegateID
3. Если не найден → ошибка
4. Обновляет TelegramID
5. Коммитит

```go
func (vc *VoteChain) CheckExistDelegateByDelegateID(ctx, delegateID) (bool, error)
func (vc *VoteChain) CheckExistDelegateByTelegramID(ctx, telegramID) (bool, error)
func (vc *VoteChain) CheckFerification(ctx, delegateID) (bool, error)
```
**Флоу проверок**:
- Создают read-only транзакцию
- Получают запись
- Возвращают `true/false` без ошибки при отсутствии записи

---

#### 3. `candidates.go` - Бизнес-логика кандидатов

```go
func (vc *VoteChain) BanCandidate(ctx, candidateID) error
```
**Флоу**:
1. Начинает транзакцию
2. Получает кандидата
3. Устанавливает `IsEligible = false`
4. Обновляет запись
5. Коммитит

**Важно**: Бан не удаляет кандидата, только исключает из выборов.

---

#### 4. `votes.go` - Бизнес-логика голосования

```go
func (vc *VoteChain) AddVote(ctx, telegramID, votes []int) error
```
**Флоу**:
1. Начинает транзакцию
2. Получает делегата по telegramID
3. Проверяет, голосовал ли уже
4. Если голосовал → **обновляет** голос (UpdateVote)
5. Если нет → добавляет новый голос (AddVote)
6. Обновляет `delegate.HasVoted = true`
7. Коммитит

**Особенность**: Делегат может переголосовать до закрытия выборов.

---

#### 5. `results.go` - Бизнес-логика результатов

```go
func (vc *VoteChain) AddResult(ctx, result) error
```
**Флоу**:
1. Начинает транзакцию
2. Проверяет, есть ли результат для курса
3. Если есть → обновляет (UpdateResult)
4. Если нет → добавляет (AddResult)
5. Коммитит

**Особенность**: Результаты перезаписываются при повторном подсчете.

---

### Паттерн транзакций

Все методы в Chain layer следуют одному паттерну:

```go
func (vc *VoteChain) SomeMethod(ctx context.Context, ...) error {
    // 1. Начать транзакцию с уровнем изоляции Serializable
    tx, err := vc.storage.BeginTx(ctx, pgx.TxOptions{
        IsoLevel: pgx.Serializable,
    })
    if err != nil {
        return fmt.Errorf("can't start transaction: %w", err)
    }
    defer tx.Rollback(ctx) // Откат при панике или ошибке

    // 2. Бизнес-логика с проверками
    existing, err := vc.storage.GetSomething(ctx, tx, id)
    if err != nil {
        return fmt.Errorf("SomeMethod: %w", err)
    }
    if existing != nil {
        return fmt.Errorf("already exists")
    }

    // 3. Операция с БД
    if err := vc.storage.DoSomething(ctx, tx, data); err != nil {
        return fmt.Errorf("SomeMethod: %w", err)
    }

    // 4. Коммит транзакции
    if err := tx.Commit(ctx); err != nil {
        return fmt.Errorf("can't commit: %w", err)
    }
    return nil
}
```

**Зачем Serializable**:
- Предотвращает гонки при одновременных операциях
- Гарантирует консистентность данных

---

## Слой бота (Bot Layer)

**Расположение**: `internal/bot/`

**Назначение**: Обработка webhook от Telegram, маршрутизация команд, управление состоянием пользователей.

### Структура Bot
```go
type Bot struct {
    botAPI    *tgbotapi.BotAPI         // Telegram API клиент
    voteChain voteChain                // Интерфейс к Chain layer
    schulze   schulze                  // Интерфейс к Schulze module
    mu        sync.RWMutex             // Мьютекс для защиты мапов
    
    // Состояние пользователей (in-memory)
    userStates          map[int64]string           // telegramID → состояние
    codeStore           map[int64]int              // telegramID → код верификации
    userEmail           map[int64]int              // telegramID → delegateID
    Candidates          map[int]models.Candidate   // candidateID → кандидат
    sortedCandidatesIDs []int                      // Отсортированные ID для UI
    rankedList          map[int64][]int            // telegramID → бюллетень
}
```

---

### Файлы и ответственность

#### 1. `bot.go` - Основная логика бота

##### Константы состояний
```go
const (
    StateWaitingForEmail = "waiting_for_email"
    StateWaitingForCode  = "waiting_for_code"
)
```

##### Основные методы

```go
func NewBot(botAPI, voteChain, schulze) *Bot
```
Инициализирует бота, создает мапы состояний.

```go
func (b *Bot) HandleWebhook(w http.ResponseWriter, r *http.Request)
```
**Флоу**:
1. Парсит JSON от Telegram
2. Создает контекст с таймаутом 10 сек
3. Вызывает `HandleUpdate`
4. Отправляет HTTP 200

```go
func (b *Bot) HandleUpdate(ctx context.Context, update tgbotapi.Update)
```
**Флоу**:
- Если `update.Message` → `handleCommand` или `handleText`
- Если `update.CallbackQuery` → `handleCallbackQuery`

```go
func (b *Bot) handleCommand(ctx context.Context, message *tgbotapi.Message)
```
**Маршрутизация команд**:
1. Проверяет, от админа ли сообщение (`message.Chat.ID == adminChatID`)
2. Если админ → админские команды
3. Если делегат → пользовательские команды

**Админские команды**:
- `/add_delegate`, `/delete_delegate`
- `/add_candidate`, `/ban_candidate`, `/delete_candidate`
- `/show_delegates`, `/show_candidates`, `/show_votes`
- `/start_voting`, `/stop_voting`
- `/results`, `/print`, `/csv`
- `/log`, `/send_logs`

**Пользовательские команды**:
- `/start` — регистрация
- `/vote` — голосование
- `/help` — помощь

```go
func (b *Bot) handleText(ctx context.Context, message *tgbotapi.Message)
```
Обрабатывает текст в зависимости от состояния:
- `StateWaitingForEmail` → `handleEmailInput`
- `StateWaitingForCode` → `handleCodeInput`

```go
func (b *Bot) SetCandidates() error
```
**Флоу**:
1. Получает всех кандидатов из БД
2. Фильтрует только `IsEligible = true`
3. Сохраняет в `b.Candidates` мапу
4. Сортирует ID для отображения
5. Формирует `candidatesList` строку для UI

---

#### 2. `registration.go` - Регистрация делегатов

```go
func (b *Bot) handleStart(ctx context.Context, message *tgbotapi.Message)
```
**Флоу**:
1. Проверяет, зарегистрирован ли (`CheckExistDelegateByTelegramID`)
2. Если да → "Вы уже зарегистрированы"
3. Если нет → "Введите st-email"
4. Устанавливает состояние: `userStates[telegramID] = StateWaitingForEmail`

```go
func (b *Bot) handleEmailInput(ctx context.Context, message *tgbotapi.Message)
```
**Флоу**:
1. Валидирует формат email (regex: `^st\d{6}$`)
2. Извлекает delegateID из email (`st123456` → `123456`)
3. Проверяет существование делегата (`CheckExistDelegateByDelegateID`)
4. Проверяет, не верифицирован ли уже (`CheckFerification`)
5. Генерирует 6-значный код
6. Отправляет код на email (`emailSender.SendVerificationCodeToEmail`)
7. Сохраняет код: `codeStore[telegramID] = code`
8. Сохраняет delegateID: `userEmail[telegramID] = delegateID`
9. Устанавливает состояние: `StateWaitingForCode`

```go
func (b *Bot) handleCodeInput(ctx context.Context, message *tgbotapi.Message)
```
**Флоу**:
1. Парсит код из текста
2. Сравнивает с сохраненным: `expectedCode == codeStore[telegramID]`
3. Если неверен → "Неверный код"
4. Если верен → `VerificateDelegate(delegateID, telegramID)`
5. Очищает состояние:
   ```go
   delete(userStates, telegramID)
   delete(codeStore, telegramID)
   delete(userEmail, telegramID)
   ```
6. Отправляет: "Регистрация завершена, используйте /vote"

**Вспомогательные функции**:
```go
func isValidEmail(email string) bool // Валидация формата
func generateCode() int              // rand(100000, 999999)
```

---

#### 3. `election.go` - Голосование

##### Глобальные переменные
```go
var helpText = "..." // Инструкция по методу Шульце
var activeVoting bool // Флаг: открыто ли голосование
```

```go
func (b *Bot) handleVote(ctx context.Context, message *tgbotapi.Message)
```
**Флоу**:
1. Проверяет `activeVoting` (открыто ли голосование)
2. Проверяет регистрацию (`CheckExistDelegateByTelegramID`)
3. Отправляет `helpText` с инструкцией
4. Отправляет `candidatesList`
5. Инициализирует бюллетень: `rankedList[telegramID] = []int{}`
6. Вызывает `sendCandidateKeyboard` для отправки клавиатуры

```go
func (b *Bot) sendCandidateKeyboard(ctx, message, editMsg bool)
```
**Флоу**:
1. Создает inline клавиатуру с кнопками кандидатов
2. Пропускает уже выбранных кандидатов
3. Если редактирование (`editMsg = true`):
   - Обновляет текст сообщения с текущим рейтингом
   - Обновляет клавиатуру
4. Если новое сообщение:
   - Отправляет новое сообщение с клавиатурой

**Формат кнопок**:
```
Text: "Иван Иванов, 2 бакалавриат"
Data: "301234" (candidateID)
```

```go
func (b *Bot) handleCallbackQuery(ctx, query *tgbotapi.CallbackQuery)
```
**Флоу** (при нажатии кнопки):
1. Проверяет `activeVoting`
2. Парсит candidateID из `query.Data`
3. Проверяет, не переполнен ли бюллетень
4. Добавляет candidateID в `rankedList[telegramID]`
5. Отправляет callback: "Кандидат учтен"
6. Если все кандидаты выбраны:
   - Проверяет уникальность (`isUniqueCandidates`)
   - Вызывает `sendRankedList` для сохранения
7. Иначе:
   - Обновляет клавиатуру (`sendCandidateKeyboard(editMsg: true)`)

```go
func (b *Bot) sendRankedList(ctx, query *tgbotapi.CallbackQuery)
```
**Флоу**:
1. Формирует текст итогового бюллетеня
2. Обновляет сообщение (убирает клавиатуру)
3. Сохраняет голос: `voteChain.AddVote(telegramID, rankedList[telegramID])`
4. Отправляет: "Ваш бюллетень принят ✅"

```go
func (b *Bot) spoilBallot(telegramID, message)
```
Вызывается при ошибках (двойной клик, множественные бюллетени):
1. Обновляет сообщение: "❌Бюллетень испорчен❌"
2. Просит использовать /vote заново

**Вспомогательные функции**:
```go
func isUniqueCandidates(rankedList []int) bool // Проверка на дубликаты
func contains(slice []int, element int) bool   // Проверка наличия
```

---

#### 4. `admin.go` - Административные команды

##### Команды управления делегатами

```go
func (b *Bot) handleAddDelegate(ctx, message)
```
**Формат**: `/add_delegate [delegate_id], [name], [group]`

**Флоу**:
1. Парсит аргументы через `strings.Split(message.CommandArguments(), ",")`
2. Валидирует:
   - `isValidID(delegate_id)` — 6 цифр
   - `isValidGroup(group)` — формат `XX.(Б|М)XX-пу`
3. Создает `models.Delegate{HasVoted: false}`
4. Вызывает `voteChain.AddDelegate`
5. Логирует: "Делегат успешно добавлен"

```go
func (b *Bot) handleDeleteDelegate(ctx, message)
```
**Формат**: `/delete_delegate [delegate_id]`

**Флоу**:
1. Парсит delegateID
2. Вызывает `voteChain.DeleteDelegate`
3. Логирует: "Делегат успешно удален"

```go
func (b *Bot) handleShowDelegates(ctx, message)
```
**Флоу**:
1. Получает всех делегатов: `voteChain.GetAllDelegates`
2. Формирует список с эмодзи:
   - ✅ зарегистрирован / ❌ не зарегистрирован
   - ✅ проголосовал / ❌ не проголосовал
3. Создает HTML ссылки на профили: `<a href="tg://user?id=...">`
4. Разбивает на сообщения по 4096 символов (лимит Telegram)

##### Команды управления кандидатами

```go
func (b *Bot) handleAddCandidate(ctx, message)
```
**Формат**: `/add_candidate [id], [name], [course], [description]`

**Флоу**:
1. Парсит аргументы
2. Валидирует:
   - `isValidID(candidate_id)` — 6 цифр
   - `isValidCourse(course)` — "X бакалавриат" или "X магистратура"
3. Создает `models.Candidate{IsEligible: true}`
4. Вызывает `voteChain.AddCandidate`

```go
func (b *Bot) handleBanCandidate(ctx, message)
func (b *Bot) handleDeleteCandidate(ctx, message)
func (b *Bot) handleShowCandidates(ctx, message)
```
Аналогично делегатам, но для кандидатов.

##### Команды управления голосованием

```go
func (b *Bot) handleStartVoting(ctx, message)
```
**Флоу**:
1. Обновляет список кандидатов: `b.SetCandidates()`
2. Устанавливает флаг: `activeVoting = true`
3. Логирует: "Голосование открыто!"

```go
func (b *Bot) handleStopVoting(ctx, message)
```
**Флоу**:
1. Устанавливает флаг: `activeVoting = false`
2. Логирует: "Голосование закрыто!"

```go
func (b *Bot) handleShowVotes(ctx, message)
```
Отображает все поданные голоса с временными метками.

##### Команды подсчета результатов

```go
func (b *Bot) handleResults(ctx, message)
```
**Флоу** (последовательный вызов методов Schulze):
1. `schulze.SetCandidates()` — загружает кандидатов
2. `schulze.SetVotes()` — загружает голоса
3. `schulze.SetCandidatesByCourse()` — группирует по курсам
4. `schulze.SetVotesByCourse()` — группирует голоса
5. `schulze.ComputeResults()` — подсчет по методу Шульце
6. `schulze.ComputeGlobalTop()` — общий рейтинг
7. `handleCSV()` — сохранение в CSV и отправка файла

```go
func (b *Bot) handlePrint(ctx, message)
```
**Флоу**:
1. Получает строку результатов: `schulze.GetResultsString()`
2. Разбивает на части по 4096 символов
3. Отправляет каждую часть с HTML форматированием

```go
func (b *Bot) handleCSV(ctx, message)
```
**Флоу**:
1. Сохраняет результаты: `schulze.SaveResultsToCSV()`
2. Читает файл: `logs/results.csv`
3. Отправляет как документ в Telegram

##### Команды логирования

```go
func (b *Bot) handleLog(ctx, message)
```
**Формат**: `/log [Debug|Info|Warn|Error]`

Изменяет уровень логирования через `log.SetLevel()`.

```go
func (b *Bot) handleSendLogs(ctx, message)
```
Отправляет файл `logs/bot.log` администратору.

**Вспомогательные функции**:
```go
func splitMessage(message string, maxSize int) []string
func toStrDelegatID(number string) string   // Форматирование с нулями
func toStrTelegramID(number string) string
func isValidID(number string) bool
func isValidCourse(course string) bool
func isValidGroup(group string) bool
```

---

## Модуль Schulze

**Расположение**: `internal/schulze/`

**Назначение**: Реализация алгоритма Шульце для определения победителя выборов на основе ранжированных голосов.

### Структура Schulze
```go
type Schulze struct {
    voteChain chain                           // Интерфейс к Chain layer
    
    votes      []models.Vote                  // Все голоса
    candidates []models.Candidate             // Все кандидаты
    
    votesByCourse      map[string][]models.Vote      // Курс → голоса
    candidatesByCourse map[string][]models.Candidate // Курс → кандидаты
}
```

---

### Файлы и ответственность

#### 1. `init.go` - Инициализация и загрузка данных

```go
func NewSchulze(voteChain chain) *Schulze
```
Создает экземпляр Schulze с пустыми мапами.

```go
func (s *Schulze) SetCandidates() error
```
**Флоу**:
1. Вызывает `voteChain.GetAllEligibleCandidates()`
2. Сохраняет в `s.candidates`

```go
func (s *Schulze) SetVotes() error
```
**Флоу**:
1. Вызывает `voteChain.GetAllVotes()`
2. Сохраняет в `s.votes`

```go
func (s *Schulze) SetCandidatesByCourse() error
```
**Флоу**:
1. Группирует кандидатов по полю `Course`
2. Заполняет `s.candidatesByCourse`

**Пример**:
```go
candidatesByCourse = {
    "1 бакалавриат": [candidate1, candidate2],
    "2 магистратура": [candidate3],
}
```

```go
func (s *Schulze) SetVotesByCourse() error
```
**Флоу**:
1. Для каждого голоса:
   - Определяет курс каждого кандидата в бюллетене
   - Фильтрует бюллетень по курсу
2. Заполняет `s.votesByCourse`

**Пример**:
```go
// Оригинальный голос: [301234, 305678, 309012]
// 301234 - "1 бакалавриат"
// 305678 - "2 бакалавриат"  
// 309012 - "1 бакалавриат"

votesByCourse = {
    "1 бакалавриат": Vote{[301234, 309012]},
    "2 бакалавриат": Vote{[305678]},
}
```

---

#### 2. `calculate.go` - Основной алгоритм Шульце

##### Метод ComputeResults

```go
func (s *Schulze) ComputeResults(ctx context.Context) error
```

**Флоу** (для каждого курса):

**Шаг 1: Подсчет парных предпочтений**
```go
preferences := s.computePairwisePreferences(votes, candidates)
```

Создает матрицу `d[i][j]`:
- `d[A][B]` = количество делегатов, предпочитающих A перед B

**Пример**:
```
Голоса: [A, B, C], [B, A, C], [A, C, B]

d[A][B] = 2 (голоса 1 и 3: A выше B)
d[B][A] = 1 (голос 2: B выше A)
d[A][C] = 3 (все голоса: A выше C)
d[C][A] = 0
...
```

**Реализация**:
```go
for _, vote := range votes {
    for i, candidate1 := range vote.CandidateRankings {
        for _, candidate2 := range vote.CandidateRankings[i+1:] {
            pairwisePreferences[candidate1][candidate2]++
        }
    }
}
```

**Шаг 2: Построение сильнейших путей**
```go
strongestPaths := s.computeStrongestPaths(preferences, candidates)
```

Использует **алгоритм Флойда-Уоршелла** для создания матрицы `p[i][j]`:
- `p[A][B]` = сила сильнейшего пути от A к B

**Инициализация**:
```go
if d[A][B] > d[B][A] {
    p[A][B] = d[A][B]  // Прямая победа
} else {
    p[A][B] = 0        // Нет победы
}
```

**Алгоритм Флойда-Уоршелла**:
```go
for i in candidates {
    for j in candidates {
        for k in candidates {
            // Ищем более сильный путь j → i → k
            potentialPath = min(p[j][i], p[i][k])
            if potentialPath > p[j][k] {
                p[j][k] = potentialPath
            }
        }
    }
}
```

**Пример**:
```
d[A][B] = 5, d[B][A] = 2 → p[A][B] = 5
d[B][C] = 6, d[C][B] = 1 → p[B][C] = 6

Путь A → B → C: min(5, 6) = 5
Если p[A][C] < 5, то p[A][C] = 5
```

**Шаг 3: Поиск победителей**
```go
potentialWinners := s.findPotentialWinners(strongestPaths, candidates)
```

Кандидат A — победитель, если для всех B:
```
p[A][B] >= p[B][A]
```

**Пример**:
```
p[A][B] = 5, p[B][A] = 3 ✓
p[A][C] = 7, p[C][A] = 4 ✓
→ A — победитель
```

**Шаг 4: Разрешение ничьи (если > 1 победителя)**
```go
potentialWinners, err := s.tieBreaker(potentialWinners, ...)
```

См. ниже раздел "Алгоритм Tie Breaker".

**Шаг 5: Сохранение результатов**
```go
result := models.Result{
    Course:            course,
    WinnerCandidateID: []int{winner.CandidateID},
    Preferences:       preferences,
    StrongestPaths:    strongestPaths,
    Stage:             "absolute" | "tie-breaker" | "tie",
}
voteChain.AddResult(ctx, result)
```

**Стадии**:
- `"absolute"` — однозначный победитель
- `"tie-breaker"` — победитель после разрешения ничьи
- `"tie"` — неразрешимая ничья (несколько победителей)

---

##### Алгоритм Tie Breaker

```go
func (s *Schulze) tieBreaker(potentialWinners, candidates, preferences, strongestPaths) ([]Candidate, error)
```

**Назначение**: Разрешить ситуацию, когда `p[A][B] == p[B][A]` (ничья между A и B).

**Основная идея** (из статьи https://arxiv.org/pdf/1804.02973):
1. Найти общие слабейшие звенья в путях A → B и B → A
2. Удалить эти звенья из графа
3. Пересчитать сильнейшие пути
4. Проверить, разрешилась ли ничья

**Флоу**:

1. Берем первых двух кандидатов из `potentialWinners`
2. Копируем матрицы (чтобы не испортить оригинал)
3. Цикл удаления звеньев:
   ```go
   for {
       // Найти все слабейшие звенья A → B
       weakestEdgesAB := s.findAllWeakestEdges(prefs, paths, A, B, candidates)
       
       // Найти все слабейшие звенья B → A
       weakestEdgesBA := s.findAllWeakestEdges(prefs, paths, B, A, candidates)
       
       // Найти общие звенья
       equalLinks := s.findEqualLinks(weakestEdgesAB, weakestEdgesBA)
       
       // Если нет общих — выход
       if equalLinks == nil {
           break
       }
       
       // Удалить первое общее звено
       tmpPreferences[from][to] = 0
       tmpStrongestPaths[from][to] = 0
       
       // Пересчитать пути
       tmpStrongestPaths = s.computeStrongestPaths(tmpPreferences, candidates)
       
       // Проверить, кто выиграл
       if tmpStrongestPaths[A][B] > tmpStrongestPaths[B][A] {
           // Удалить B из победителей
           potentialWinners = removeCandidate(potentialWinners, indexB)
           break
       } else if tmpStrongestPaths[A][B] < tmpStrongestPaths[B][A] {
           // Удалить A
           potentialWinners = removeCandidate(potentialWinners, indexA)
           break
       }
   }
   ```

4. Повторить для оставшихся пар

**Функция findAllWeakestEdges**:

```go
func (s *Schulze) findAllWeakestEdges(preferences, strongestPaths map, start, end int, candidates)
```

**Назначение**: Найти все ребра (звенья) на сильнейшем пути от start к end, сила которых равна силе пути.

**Алгоритм**: DFS (поиск в глубину)
1. Начинаем с `start`
2. Рекурсивно идем по соседям, где `d[current][next] > d[next][current]`
3. Когда достигли `end`:
   - Находим минимальную силу ребра в пути
   - Если сила = сила сильнейшего пути → сохраняем все ребра с этой силой

**Пример**:
```
Путь A → C → E → B с силами: 8 → 7 → 10
Минимум = 7 (слабейшее звено)
Если p[A][B] = 7, то слабейшее звено: C → E
```

---

#### 3. `common.go` - Общий рейтинг и дополнительные методы

```go
func (s *Schulze) ComputeGlobalTop(ctx context.Context) error
```

**Назначение**: Создать общий рейтинг всех кандидатов (не по курсам).

**Флоу**:
1. Получить всех кандидатов и все голоса
2. Исключить победителей по курсам (`excludeCourseWinners`)
3. Подсчитать парные предпочтения и пути
4. Найти потенциальных победителей
5. Построить строгий порядок (`buildStrictOrder`)
6. Сохранить результат с Course = "Global Top"

```go
func (s *Schulze) excludeCourseWinners(ctx, allCandidates, allVotes) ([]Candidate, []Vote, int, error)
```

**Назначение**: Исключить победителей по курсам из общего рейтинга.

**Флоу**:
1. Получить все результаты: `voteChain.GetAllResults()`
2. Собрать ID победителей из `result.WinnerCandidateID`
3. Удалить этих кандидатов из `allCandidates`
4. Удалить их из всех бюллетеней в `allVotes`
5. Вернуть "очищенные" данные

```go
func (s *Schulze) buildStrictOrder(candidates, preferences, strongestPaths, commonPlaces) ([]Candidate, error)
```

**Назначение**: Построить строгий линейный порядок кандидатов (1-е место, 2-е, 3-е и т.д.).

**Флоу**:
1. Найти победителя среди оставшихся
2. Добавить в порядок
3. Удалить из списка кандидатов
4. Повторить для оставшихся
5. Обработать ничьи на одном месте

**Пример результата**:
```
1. Кандидат А
2-3. Кандидат Б (ничья)
2-3. Кандидат В (ничья)
4. Кандидат Г
```

---

#### 4. `print.go` - Вывод результатов

```go
func (s *Schulze) GetResultsString() (string, error)
```

**Назначение**: Сформировать текстовое представление результатов для Telegram.

**Флоу**:
1. Получить все результаты: `voteChain.GetAllResults()`
2. Для каждого результата:
   - Построить строгий порядок кандидатов
   - Вывести таблицу парных предпочтений
   - Вывести таблицу сильнейших путей
3. Форматировать с HTML разметкой

**Формат вывода**:
```html
<b>Результаты по курсу: 1 бакалавриат</b>

Строгий порядок (stage: absolute):
1. Иван Иванов (st301234)
2. Петр Петров (st305678)
...

Матрица парных предпочтений:
     | st301234 | st305678 | ...
-----|----------|----------|----
st301234 |    0     |    5     | ...
st305678 |    3     |    0     | ...

Матрица сильнейших путей:
...
```

**Вспомогательные функции**:
```go
func (s *Schulze) preferencesToString(preferences, order) string
func (s *Schulze) strongestPathsToString(strongestPaths, order) string
func idtos(number int) string // Форматирование ID с ведущими нулями
```

---

#### 5. `csv.go` - Экспорт в CSV

```go
func (s *Schulze) SaveResultsToCSV(ctx context.Context) error
```

**Назначение**: Сохранить результаты в CSV файл для анализа в Excel.

**Флоу**:
1. Создать/открыть файл `logs/results.csv`
2. Для каждого курса:
   - Записать заголовок: `Course: X`
   - Записать строгий порядок
   - Записать матрицу парных предпочтений
   - Записать матрицу сильнейших путей
   - Добавить пустую строку
3. Закрыть файл

**Формат CSV**:
```csv
Course:,1 бакалавриат,Stage:,absolute

Strict Order:
1,Иван Иванов,st301234
2,Петр Петров,st305678

Pairwise Preferences:
,st301234,st305678
st301234,0,5
st305678,3,0

Strongest Paths:
,st301234,st305678
st301234,0,5
st305678,0,0
```

**Вспомогательные функции**:
```go
func writeMatrixToCSV(writer *csv.Writer, matrix map[int]map[int]int) error
```

---

## Вспомогательные модули

### 1. Email Module (`internal/email/`)

**Файл**: `sender.go`

**Назначение**: Отправка кодов верификации на email студентов.

#### Основные функции

```go
func SendVerificationCodeToEmail(email string, code int) error
```
Обертка с таймаутом 15 секунд.

```go
func SendVerificationCodeToEmailWithContext(ctx context.Context, email string, code int) error
```

**Флоу**:
1. Валидация email (`validateEmail`)
2. Валидация кода (`validateVerificationCode`)
3. Получение credentials из env: `SMTP_EMAIL`, `SMTP_PASSWORD`
4. Создание соединения с smtp.mail.ru:2525 с таймаутом
5. Установка TLS (STARTTLS):
   ```go
   tlsconfig := &tls.Config{
       InsecureSkipVerify: false,
       ServerName:         smtpHost,
       MinVersion:         tls.VersionTLS12,
   }
   conn.StartTLS(tlsconfig)
   ```
6. Аутентификация через `smtp.PlainAuth`
7. Отправка письма с MIME заголовками:
   ```
   From: sender@mail.ru
   To: recipient@student.spbu.ru
   Subject: Ваш код подтверждения
   MIME-Version: 1.0
   Content-Type: text/plain; charset=UTF-8
   
   Ваш код подтверждения: 123456
   ```

#### Валидация

```go
func validateEmail(email string) error
```
- Проверка на пустоту
- Длина <= 254 символа
- Regex: `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`

```go
func validateVerificationCode(code int) error
```
- Диапазон: 100000-999999 (6 цифр)

#### Особенности безопасности

✅ **Реализовано**:
- Таймауты 15 секунд для всех операций
- TLS 1.2+ обязателен
- Валидация входных данных
- Проверка сертификата сервера
- Аутентификация после установки TLS
- Логи не содержат email и коды (GDPR)

---

### 2. Logger Module (`internal/logger/`)

**Файл**: `logger.go`

**Назначение**: Кастомное логирование с отправкой уведомлений в Telegram.

#### Структура Logger

```go
type Logger struct {
    loggerBotAPI *tgbotapi.BotAPI // Клиент Telegram
    entry        *logrus.Entry    // Logrus entry
    logFile      *os.File         // Файл логов
}
```

#### Инициализация

```go
func NewLogger(loggerBotAPI *tgbotapi.BotAPI, level string) *Logger
```

**Флоу**:
1. Настройка logrus:
   ```go
   logrus.SetFormatter(&logrus.TextFormatter{
       FullTimestamp:   true,
       TimestampFormat: "15:04:05",
   })
   ```
2. Открытие файла `./logs/bot.log`
3. Установка MultiWriter (консоль + файл):
   ```go
   logrus.SetOutput(io.MultiWriter(os.Stdout, logFile))
   ```
4. Установка уровня логирования

#### Методы логирования

Все стандартные уровни logrus:
```go
func (l *Logger) Debug(args ...interface{})
func (l *Logger) Info(args ...interface{})
func (l *Logger) Warn(args ...interface{})
func (l *Logger) Error(args ...interface{})
func (l *Logger) Fatal(args ...interface{})
func (l *Logger) Panic(args ...interface{})
```

И форматированные версии:
```go
func (l *Logger) Debugf(format string, args ...interface{})
// ... и так далее
```

#### Отправка в Telegram

После каждого лога (если уровень соответствует):
```go
func (l *Logger) sendNotification(message string)
```

**Формат сообщений**:
```html
🐛<a href="tg://user?id=123456789">DEBUG</a>: сообщение
🔎<a href="tg://user?id=123456789">INFO</a>: сообщение
⚠️<a href="tg://user?id=123456789">WARN</a>: сообщение
📛<a href="tg://user?id=123456789">ERROR</a>: сообщение
```

**Первый аргумент** — всегда telegramID пользователя (для ссылки).

**Пример использования**:
```go
log.Info(message.Chat.ID, " Делегат зарегистрирован")
// Вывод в консоль и файл
// Отправка в LOG_CHAT_ID: "🔎<a href="tg://user?id=...">INFO</a>: 123456789 Делегат зарегистрирован"
```

#### Изменение уровня

```go
func (l *Logger) SetLevel(level string) error
```

Позволяет админу динамически менять уровень через `/log Debug`.

---

## Флоу работы системы

### 1. Запуск приложения

**Файл**: `cmd/main.go`

```
1. Читается DATABASE_URL из env
   ↓
2. Создается Storage (db.NewStorage)
   ├─ Создается пул соединений pgxpool
   ├─ Проверяется подключение (Ping)
   └─ defer storage.Close()
   ↓
3. Создается VoteChain (chain.NewVoteChain)
   ↓
4. Читается TELEGRAM_APITOKEN
   ↓
5. Создается Telegram Bot API (tgbotapi.NewBotAPI)
   ↓
6. Создается Schulze (schulze.NewSchulze)
   ↓
7. Создается Bot (bot.NewBot)
   ├─ Инициализируется Logger
   ├─ Создаются мапы состояний
   └─ defer botHandler.Close()
   ↓
8. Регистрируются HTTP handlers:
   ├─ "/" → botHandler.HandleWebhook
   └─ "/results" → (TODO)
   ↓
9. Запускается HTTP сервер на порту 8080
   ↓
10. Ожидается сигнал завершения (SIGINT/SIGTERM)
    ↓
11. Graceful shutdown за 5 секунд
```

---

### 2. Флоу регистрации делегата

```
Делегат → /start
   ↓
Bot: handleStart
   ├─ Проверка: зарегистрирован? (CheckExistDelegateByTelegramID)
   │  └─ Да → "Вы уже зарегистрированы"
   │  └─ Нет ↓
   ├─ Отправка: "Введите st-email"
   └─ Состояние: StateWaitingForEmail
   ↓
Делегат вводит: st123456
   ↓
Bot: handleEmailInput
   ├─ Валидация формата (isValidEmail)
   ├─ Извлечение delegateID: 123456
   ├─ Проверка существования (CheckExistDelegateByDelegateID)
   ├─ Проверка верификации (CheckFerification)
   ├─ Генерация кода: 473829
   ├─ Отправка email (SendVerificationCodeToEmail)
   │  ├─ TCP connect → smtp.mail.ru:2525
   │  ├─ STARTTLS
   │  ├─ Аутентификация
   │  └─ Отправка письма
   ├─ Сохранение: codeStore[telegramID] = 473829
   ├─ Сохранение: userEmail[telegramID] = 123456
   ├─ Состояние: StateWaitingForCode
   └─ Отправка: "Код отправлен на email"
   ↓
Делегат вводит: 473829
   ↓
Bot: handleCodeInput
   ├─ Проверка кода: codeStore[telegramID] == 473829?
   │  └─ Нет → "Неверный код"
   │  └─ Да ↓
   ├─ Верификация делегата:
   │  Chain: VerificateDelegate(delegateID, telegramID)
   │     ├─ BeginTx
   │     ├─ GetDelegateByDelegateID
   │     ├─ UpdateDelegate (TelegramID = ...)
   │     └─ Commit
   ├─ Очистка состояния:
   │  ├─ delete(userStates, telegramID)
   │  ├─ delete(codeStore, telegramID)
   │  └─ delete(userEmail, telegramID)
   └─ Отправка: "Регистрация завершена, используйте /vote"
```

---

### 3. Флоу голосования

#### Открытие голосования (Администратор)

```
Админ → /start_voting
   ↓
Bot: handleStartVoting
   ├─ SetCandidates()
   │  ├─ GetAllCandidates → фильтр IsEligible
   │  ├─ Заполнение b.Candidates
   │  ├─ Сортировка b.sortedCandidatesIDs
   │  └─ Формирование candidatesList
   ├─ activeVoting = true
   └─ Лог: "Голосование открыто!"
```

#### Процесс голосования (Делегат)

```
Делегат → /vote
   ↓
Bot: handleVote
   ├─ Проверка: activeVoting == true?
   ├─ Проверка: зарегистрирован? (CheckExistDelegateByTelegramID)
   ├─ Отправка helpText (инструкция)
   ├─ Отправка candidatesList
   ├─ Инициализация: rankedList[telegramID] = []
   └─ sendCandidateKeyboard(editMsg: false)
      ├─ Создание inline клавиатуры:
      │  [Иван Иванов, 1 бакалавриат]
      │  [Петр Петров, 2 магистратура]
      │  [...]
      └─ Отправка сообщения
   ↓
Делегат нажимает: "Иван Иванов"
   ↓
Bot: handleCallbackQuery
   ├─ Парсинг: candidateID = 301234
   ├─ Проверка: len(rankedList) < len(Candidates)?
   ├─ Добавление: rankedList[telegramID].append(301234)
   ├─ Callback: "Кандидат учтен"
   ├─ Проверка: все выбраны?
   │  └─ Нет → sendCandidateKeyboard(editMsg: true)
   │            ├─ Обновление текста:
   │            │  "Выберите...\n\n1. Иван Иванов"
   │            └─ Обновление клавиатуры (без выбранных)
   │  └─ Да ↓
   ├─ Проверка уникальности: isUniqueCandidates?
   │  └─ Нет → spoilBallot
   │  └─ Да ↓
   └─ sendRankedList
      ├─ Обновление сообщения:
      │  "Ваш итоговый бюллетень:
      │   1. Иван Иванов
      │   2. Петр Петров
      │   3. ..."
      ├─ Сохранение голоса:
      │  Chain: AddVote(telegramID, rankedList)
      │     ├─ BeginTx
      │     ├─ GetDelegateByTelegramID
      │     ├─ GetVoteByDelegateID
      │     ├─ Если существует → UpdateVote
      │     │  └─ Иначе → AddVote
      │     ├─ UpdateDelegate (HasVoted = true)
      │     └─ Commit
      └─ Отправка: "Ваш бюллетень принят✅"
```

#### Закрытие голосования

```
Админ → /stop_voting
   ↓
Bot: handleStopVoting
   ├─ activeVoting = false
   └─ Лог: "Голосование закрыто!"
```

---

### 4. Флоу подсчета результатов

```
Админ → /results
   ↓
Bot: handleResults
   ↓
1. schulze.SetCandidates()
   └─ GetAllEligibleCandidates → s.candidates
   ↓
2. schulze.SetVotes()
   └─ GetAllVotes → s.votes
   ↓
3. schulze.SetCandidatesByCourse()
   └─ Группировка: s.candidatesByCourse["1 бакалавриат"] = [...]
   ↓
4. schulze.SetVotesByCourse()
   └─ Фильтрация голосов по курсам: s.votesByCourse["1 бакалавриат"] = [...]
   ↓
5. schulze.ComputeResults(ctx)
   │
   Для каждого курса:
   │
   ├─ Шаг 1: computePairwisePreferences
   │  └─ Создание матрицы d[i][j]
   │
   ├─ Шаг 2: computeStrongestPaths
   │  └─ Флойд-Уоршелл → матрица p[i][j]
   │
   ├─ Шаг 3: findPotentialWinners
   │  └─ Поиск кандидатов, где ∀j: p[i][j] >= p[j][i]
   │
   ├─ Шаг 4: Если len(winners) > 1 → tieBreaker
   │  ├─ Поиск общих слабейших звеньев
   │  ├─ Удаление звеньев
   │  ├─ Пересчет путей
   │  └─ Проверка победителя
   │
   └─ Сохранение результата:
      Chain: AddResult(result)
         ├─ BeginTx
         ├─ GetResultByCourse
         ├─ Если существует → UpdateResult
         │  └─ Иначе → AddResult (JSON.Marshal для матриц)
         └─ Commit
   ↓
6. schulze.ComputeGlobalTop(ctx)
   ├─ excludeCourseWinners (убрать победителей по курсам)
   ├─ computePairwisePreferences (для всех кандидатов)
   ├─ computeStrongestPaths
   ├─ findPotentialWinners
   ├─ buildStrictOrder (линейный порядок с учетом ничьих)
   └─ AddResult(Course: "Global Top")
   ↓
7. bot.handleCSV
   ├─ schulze.SaveResultsToCSV
   │  ├─ GetAllResults
   │  ├─ Создание logs/results.csv
   │  ├─ Запись каждого курса:
   │  │  ├─ Strict Order
   │  │  ├─ Pairwise Preferences Matrix
   │  │  └─ Strongest Paths Matrix
   │  └─ Закрытие файла
   ├─ Чтение файла results.csv
   └─ Отправка файла админу
```

---

### 5. Просмотр результатов

```
Админ → /print
   ↓
Bot: handlePrint
   ↓
schulze.GetResultsString()
   ├─ GetAllResults
   ├─ Для каждого результата:
   │  ├─ buildStrictOrder
   │  ├─ preferencesToString (матрица d[i][j])
   │  └─ strongestPathsToString (матрица p[i][j])
   └─ Форматирование HTML
   ↓
Bot: splitMessage (по 4096 символов)
   ↓
Bot: Отправка каждой части с ParseMode: "HTML"
```

---

## Диаграммы взаимодействия

### Зависимости модулей

```
cmd/main
   ├─> internal/bot
   │      ├─> internal/chain
   │      │      └─> internal/db
   │      │             └─> PostgreSQL
   │      ├─> internal/schulze
   │      │      └─> internal/chain
   │      ├─> internal/email
   │      └─> internal/logger
   └─> internal/models (используется везде)
```

### Взаимодействие слоев

```
Telegram → Webhook → HTTP Server (cmd/main)
                          ↓
                    Bot Layer (internal/bot)
                    ├─ Команды
                    ├─ Состояния
                    └─ UI
                          ↓
                    Chain Layer (internal/chain)
                    ├─ Транзакции
                    ├─ Валидация
                    └─ Бизнес-правила
                          ↓
                    DB Layer (internal/db)
                    ├─ SQL запросы
                    └─ CRUD операции
                          ↓
                    PostgreSQL Database
                    ├─ delegates
                    ├─ candidates
                    ├─ votes
                    └─ results
```

### Поток данных при голосовании

```
Делегат (Telegram)
   │
   │ 1. Нажатие кнопки [Кандидат A]
   ↓
Bot.handleCallbackQuery
   │ candidateID из callback data
   │
   │ 2. Добавление в rankedList
   ↓
Bot.rankedList[telegramID] = [301234, ...]
   │
   │ 3. Все выбраны?
   ↓
Bot.sendRankedList
   │
   │ 4. Вызов Chain layer
   ↓
Chain.AddVote(telegramID, rankedList)
   │
   │ 5. Начало транзакции
   ↓
DB.GetDelegateByTelegramID
   │
   ↓
DB.GetVoteByDelegateID
   │
   ├─ Существует? → DB.UpdateVote
   └─ Нет?       → DB.AddVote
   │
   ↓
DB.UpdateDelegate(HasVoted = true)
   │
   │ 6. Коммит транзакции
   ↓
PostgreSQL: INSERT/UPDATE votes
   │
   │ 7. Подтверждение
   ↓
Делегат (Telegram)
   "Ваш бюллетень принят✅"
```

---

## Ключевые особенности архитектуры

### 1. Разделение ответственности

- **Bot Layer**: Только UI и маршрутизация
- **Chain Layer**: Только бизнес-логика и транзакции
- **DB Layer**: Только SQL операции

### 2. Управление транзакциями

Все транзакции в Chain Layer:
- Уровень изоляции: `Serializable`
- Pattern: BeginTx → Logic → Commit/Rollback
- Атомарность всех операций

### 3. Состояние в памяти

Bot хранит временное состояние:
- `userStates`: состояние регистрации
- `codeStore`: коды верификации
- `rankedList`: незаконченные бюллетени

**Недостаток**: При перезапуске бота состояние теряется.

**Решение**: Можно добавить Redis для персистентности.

### 4. Безопасность

- ✅ Email коды не логируются
- ✅ TLS 1.2+ для SMTP
- ✅ SQL injection защита (pgx parameters)
- ✅ Валидация всех входных данных
- ✅ Таймауты на всех операциях

### 5. Масштабируемость

**Ограничения**:
- Один экземпляр бота (состояние в памяти)
- Webhook на один URL

**Возможные улучшения**:
- Redis для состояния
- Message queue (RabbitMQ) для обработки
- Горизонтальное масштабирование обработчиков

---

## Конфигурация и переменные окружения

### Обязательные переменные

```env
# Telegram
TELEGRAM_APITOKEN=...         # Токен бота от BotFather

# Database
DATABASE_URL=postgres://...   # DSN PostgreSQL

# SMTP (Email)
SMTP_EMAIL=...                # Логин mail.ru
SMTP_PASSWORD=...             # Пароль приложения

# Admin
ADMIN_CHAT_ID=...             # Telegram ID администратора
LOG_CHAT_ID=...               # Telegram ID чата для логов
```

### Docker Compose

```yaml
services:
  postgres:
    image: postgres:15.1
    ports: ["5432:5432"]
    volumes: [./postgres_data:/var/lib/postgresql/data]
  
  bot:
    build: .
    ports: ["8080:8080"]
    environment:
      - TELEGRAM_APITOKEN
      - DATABASE_URL
      - SMTP_EMAIL
      - SMTP_PASSWORD
    depends_on: [postgres]
  
  ngrok:
    image: ngrok/ngrok
    command: http --url=${NGROK_URL} bot:8080
    environment: [NGROK_AUTHTOKEN]
    depends_on: [bot]
```

---

## Заключение

Проект представляет собой хорошо структурированное приложение с четким разделением слоев:

1. **Presentation Layer (Bot)** — взаимодействие с пользователями
2. **Business Layer (Chain)** — бизнес-логика и транзакции
3. **Data Layer (DB)** — работа с базой данных
4. **Algorithm Module (Schulze)** — математические вычисления
5. **Support Modules** — email, логирование

Система позволяет проводить выборы с гарантией корректного подсчета голосов по методу Шульце, обеспечивая прозрачность и защиту данных.

