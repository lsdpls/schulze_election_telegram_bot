package bot

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/logger"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Заголовок, в котором Telegram передаёт secret_token из setWebhook
const secretHeader = "X-Telegram-Bot-Api-Secret-Token"

// Возможные состояния пользователя
const (
	StateWaitingForEmail = "waiting_for_email"
	StateWaitingForCode  = "waiting_for_code"
)

var log *logger.Logger

// Bot struct for managing commands and Telegram API
type Bot struct {
	botAPI    *tgbotapi.BotAPI // Telegram API
	voteChain voteChain        // цепочка для взаимодействия с базой данных
	schulze   schulze          // структура для работы с алгоритмом Шульце
	mu        sync.RWMutex     // Блокировка ресурсов

	// TODO: create user session struct
	userStates          map[int64]string // Состояния пользователей, где ключ — telegramID, а значение — текущее состояние
	codeStore           map[int64]int    // Хранение кодов подтверждения (telegramID -> код)
	userEmail           map[int64]int    // Хранение не верифицированных email пользователей (telegramID -> email)
	Candidates          map[int]models.Candidate
	sortedCandidatesIDs []int
	rankedList          map[int64][]int // Хранение незаполненных бюллетеней
	candidatesList      string          // Список кандидатов для отправки пользователям
	activeVoting        bool            // Флаг активного голосования

	// Антиспам писем с кодом (см. reserveCodeSend), всё под b.mu
	lastCodeByTG       map[int64]time.Time // последняя отправка кода по telegramID
	lastCodeByDelegate map[int]time.Time   // последняя отправка кода по delegateID
	emailsDay          string              // сутки счётчиков, YYYY-MM-DD UTC; при смене суток счётчики ниже обнуляются
	emailsSent         int                 // писем за emailsDay
	codesByTGDay       map[int64]int       // кодов за emailsDay по telegramID
	codesByDelegateDay map[int]int         // кодов за emailsDay по delegateID
	codeAttempts       map[int64]int       // неверных вводов кода подряд по telegramID
}

// NewBot создает новый экземпляр бота
func NewBot(botAPI *tgbotapi.BotAPI, voteChain voteChain, schulze schulze) *Bot {
	log = logger.NewLogger(botAPI, config.LogLevel, config.TelegramLogLevel)
	return &Bot{
		botAPI:         botAPI,
		voteChain:      voteChain,
		schulze:        schulze,
		userStates:     make(map[int64]string),
		codeStore:      make(map[int64]int),
		userEmail:      make(map[int64]int),
		rankedList:     make(map[int64][]int),
		Candidates:     make(map[int]models.Candidate),
		candidatesList: "",
		activeVoting:   false,

		lastCodeByTG:       make(map[int64]time.Time),
		lastCodeByDelegate: make(map[int]time.Time),
		codesByTGDay:       make(map[int64]int),
		codesByDelegateDay: make(map[int]int),
		codeAttempts:       make(map[int64]int),
	}
}

func (b *Bot) Close() error {
	if err := log.Close(); err != nil {
		return err
	}
	return nil
}

type voteChain interface {
	AddDelegate(ctx context.Context, delegate models.Delegate) error
	GetDelegateByDelegateID(ctx context.Context, delegateID int) (*models.Delegate, error)
	GetAllDelegates(ctx context.Context) ([]models.Delegate, error)
	VerificateDelegate(ctx context.Context, delegateID int, telegramID sql.NullInt64) error
	DeleteDelegate(ctx context.Context, delegateID int) error
	CheckExistDelegateByDelegateID(ctx context.Context, delegateID int) (bool, error)
	CheckExistDelegateByTelegramID(ctx context.Context, telegramID int64) (bool, error)
	CheckFerification(ctx context.Context, delegateID int) (bool, error)

	AddCandidate(ctx context.Context, candidate models.Candidate) error
	GetAllCandidates(ctx context.Context) ([]models.Candidate, error)
	BanCandidate(ctx context.Context, candidateID int) error
	DeleteCandidate(ctx context.Context, candidateID int) error

	AddVote(ctx context.Context, telegramID int64, votes []int) error
	GetAllVotes(ctx context.Context) ([]models.Vote, error)
	UpdateVote(ctx context.Context, vote models.Vote) error
	DeleteVoteByDelegateID(ctx context.Context, delegateID int) error

	AddResult(ctx context.Context, result models.Result) error
	GetAllResults(ctx context.Context) ([]models.Result, error)
}

type schulze interface {
	SetCandidates() error
	SetVotes() error
	SetCandidatesByCourse() error
	SetVotesByCourse() error
	GetResultsString() (string, error)
	ComputeResults(ctx context.Context) error
	ComputeGlobalTop(ctx context.Context) error
	SaveResultsToCSV(ctx context.Context) error
}

// Установка списка кандидатов перед голосованием
func (b *Bot) SetCandidates() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	candidates, err := b.voteChain.GetAllCandidates(context.Background())
	if err != nil {
		return fmt.Errorf("SetCandidates: %w", err)
	}
	b.Candidates = make(map[int]models.Candidate)
	b.sortedCandidatesIDs = []int{}
	for _, candidate := range candidates {
		if candidate.IsEligible {
			b.Candidates[candidate.CandidateID] = candidate
		}
	}
	for candidateID := range b.Candidates {
		b.sortedCandidatesIDs = append(b.sortedCandidatesIDs, candidateID)
	}
	sort.Ints(b.sortedCandidatesIDs)

	b.candidatesList = msgVoteCandidatesHeader
	for _, k := range b.sortedCandidatesIDs {
		b.candidatesList += fmt.Sprintf(msgVoteCandidateLineFmt, html.EscapeString(b.Candidates[k].Name), html.EscapeString(b.Candidates[k].Course))
	}

	return nil
}

// logWebhookReject логирует каждый отказ (файл и лог-чат); значение секрета в лог не попадает
func logWebhookReject(r *http.Request, status int, hasSecret bool) {
	log.Warnf("webhook rejected: method=%s status=%d remote=%s x_forwarded_for=%q x_real_ip=%q secret_header=%t",
		r.Method, status, r.RemoteAddr, r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-IP"), hasSecret)
}

// HandleWebhook обрабатывает вебхуки от Telegram; запросы без верного secret_token отвергаются.
func (b *Bot) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	got := r.Header.Get(secretHeader)
	if r.Method != http.MethodPost {
		logWebhookReject(r, http.StatusMethodNotAllowed, got != "")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Без настроенного секрета закрываемся: пустой заголовок не должен совпадать с пустой настройкой
	if config.WebhookSecret == "" {
		logWebhookReject(r, http.StatusServiceUnavailable, got != "")
		http.Error(w, "webhook secret not configured", http.StatusServiceUnavailable)
		return
	}
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(config.WebhookSecret)) != 1 {
		logWebhookReject(r, http.StatusForbidden, got != "")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Создаем контекст с таймаутом для обработки запроса
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Битое или слишком большое (> 1 MiB) тело отбрасываем с 200: на не-2xx Telegram повторяет update бесконечно и блокирует очередь
	var update tgbotapi.Update
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&update); err != nil {
		log.Errorf("Error decoding update: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Паника обработчика не должна оставлять запрос без 200: иначе Telegram повторяет тот же update и блокирует очередь
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("panic при обработке update %d: %v", update.UpdateID, r)
			w.WriteHeader(http.StatusOK)
		}
	}()
	if update.Message != nil || update.CallbackQuery != nil {
		b.HandleUpdate(ctx, update) // TODO добавить выход по таймауту
	}

	w.WriteHeader(http.StatusOK)
}

// HandleUpdate обрабатывает обновления от Telegram; неполные объекты игнорируются
func (b *Bot) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	switch {
	case update.Message != nil:
		if update.Message.Chat == nil {
			return
		}
		// Группа стала супергруппой: chat_id сменился, ADMIN_CHAT_ID/LOG_CHAT_ID в .env устарели
		if m := update.Message; m.MigrateToChatID != 0 && (m.Chat.ID == config.AdminChatID || m.Chat.ID == config.LogChatID) {
			log.Errorf("чат %d мигрировал в %d: обновите ADMIN_CHAT_ID/LOG_CHAT_ID в .env и перезапустите бота", m.Chat.ID, m.MigrateToChatID)
		}
		if update.Message.IsCommand() {
			b.handleCommand(ctx, update.Message)
		} else {
			b.handleText(ctx, update.Message)
		}
	case update.CallbackQuery != nil:
		query := update.CallbackQuery
		// handleCallbackQuery разыменовывает From и Message; паника там оставила бы b.mu захваченным навсегда
		if query.From == nil || query.Message == nil {
			if b.botAPI != nil {
				b.botAPI.Request(tgbotapi.NewCallback(query.ID, ""))
			}
			return
		}
		b.handleCallbackQuery(ctx, query)
	}
}

// HandleCommand обрабатывает команды пользователя
func (b *Bot) handleCommand(ctx context.Context, message *tgbotapi.Message) {
	// Проверка администратора
	if message.Chat.ID == config.AdminChatID {
		switch message.Command() {
		// Изменение базы данных
		case "add_delegate":
			b.handleAddDelegate(ctx, message)
		case "delete_delegate":
			b.handleDeleteDelegate(ctx, message)
		case "add_candidate":
			b.handleAddCandidate(ctx, message)
		case "ban_candidate":
			b.handleBanCandidate(ctx, message)
		case "delete_candidate":
			b.handleDeleteCandidate(ctx, message)
		// Показать инфу
		case "show_delegates":
			b.handleShowDelegates(ctx, message)
		case "show_candidates":
			b.handleShowCandidates(ctx, message)
		case "show_votes":
			b.handleShowVotes(ctx, message)
		case "show_code":
			b.handleShowCode(ctx, message)
		// Управление голосованием
		case "start_voting":
			b.handleStartVoting(ctx, message)
		case "stop_voting":
			b.handleStopVoting(ctx, message)
		case "results":
			b.handleResults(ctx, message)
		case "print":
			b.handlePrint(ctx, message)
		case "csv":
			b.handleCSV(ctx, message)
		// Уровень логирования
		case "log":
			b.handleLog(ctx, message)
		case "send_logs":
			b.handleSendLogs(ctx, message)
		// Показать доступные команды
		case "help":
			b.handleHelpAdmin(ctx, message)
		default:
			msg := tgbotapi.NewMessage(message.Chat.ID, msgAdminUnknownCommand)
			b.botAPI.Send(msg)
		}
		return
	}
	if !message.Chat.IsPrivate() {
		return
	}
	// Команды делегатов
	switch message.Command() {
	case "start":
		b.handleStart(ctx, message)
	case "vote":
		b.handleVote(ctx, message)
	case "help":
		b.handleHelp(ctx, message)
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, msgCmdUnknown)
		b.botAPI.Send(msg)
	}
}

// HandleText обрабатывает текстовые сообщения пользователя
func (b *Bot) handleText(ctx context.Context, message *tgbotapi.Message) {
	// текст ждём только в личке; в группах сюда приходят служебные сообщения и реплаи
	if !message.Chat.IsPrivate() {
		return
	}
	b.mu.RLock()
	state := b.userStates[message.Chat.ID]
	b.mu.RUnlock()

	switch state {
	case StateWaitingForEmail:
		b.handleEmailInput(ctx, message)
	case StateWaitingForCode:
		b.handleCodeInput(ctx, message)
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, msgCmdDefaultHint)
		b.botAPI.Send(msg)
	}
}

// Подсказка по командам
func (b *Bot) handleHelp(_ context.Context, message *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(message.Chat.ID, msgCmdHelp)
	b.botAPI.Send(msg)
}

func (b *Bot) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	_, err := b.botAPI.Send(msg)
	return err
}

func (b *Bot) handleLog(_ context.Context, message *tgbotapi.Message) {
	level := message.CommandArguments()
	var err error
	switch level {
	case "Debug":
		err = log.SetLevel("Debug")
	case "Info":
		err = log.SetLevel("Info")
	case "Warn":
		err = log.SetLevel("Warn")
	case "Error":
		err = log.SetLevel("Error")
	default:
		log.Errorf("%d Неизвестный уровень логирования: %s", message.Chat.ID, level)
		return
	}
	if err != nil {
		log.Errorf("%d Ошибка при изменении уровня логирования: %v", message.Chat.ID, err)
		return
	}
}
