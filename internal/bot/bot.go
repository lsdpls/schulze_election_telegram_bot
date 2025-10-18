package bot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/logger"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

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
}

// NewBot создает новый экземпляр бота
func NewBot(botAPI *tgbotapi.BotAPI, voteChain voteChain, schulze schulze) *Bot {
	log = logger.NewLogger(botAPI, "Info")
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

	b.candidatesList = "Cписок кандидатов:\n\n"
	for _, k := range b.sortedCandidatesIDs {
		b.candidatesList += fmt.Sprintf("• %s, %s\n", b.Candidates[k].Name, b.Candidates[k].Course)
	}

	return nil
}

// HandleWebhook обрабатывает вебхуки от Telegram
func (b *Bot) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	// Создаем контекст с таймаутом для обработки запроса
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var update tgbotapi.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		log.Errorf("Error decoding update: %v", err)
		http.Error(w, "Error decoding update", http.StatusBadRequest)
		return
	}

	if update.Message != nil || update.CallbackQuery != nil {
		b.HandleUpdate(ctx, update) // TODO добавить выход по таймауту
	}

	w.WriteHeader(http.StatusOK)
}

// HandleUpdate обрабатывает обновления от Telegram
func (b *Bot) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.Message != nil {
		if update.Message.IsCommand() {
			b.handleCommand(ctx, update.Message)
		} else {
			b.handleText(ctx, update.Message)
		}
	} else if update.CallbackQuery != nil {
		b.handleCallbackQuery(ctx, update.CallbackQuery)
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
			msg := tgbotapi.NewMessage(message.Chat.ID, "Неизвестная Администрирующая команда")
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
		msg := tgbotapi.NewMessage(message.Chat.ID, "Неизвестная команда")
		b.botAPI.Send(msg)
	}
}

// HandleText обрабатывает текстовые сообщения пользователя
func (b *Bot) handleText(ctx context.Context, message *tgbotapi.Message) {
	b.mu.RLock()
	state := b.userStates[message.Chat.ID]
	b.mu.RUnlock()

	switch state {
	case StateWaitingForEmail:
		b.handleEmailInput(ctx, message)
	case StateWaitingForCode:
		b.handleCodeInput(ctx, message)
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, "Используйте /start для регистрации и /vote для голосования")
		b.botAPI.Send(msg)
	}
}

// Подсказка по командам
func (b *Bot) handleHelp(_ context.Context, message *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(message.Chat.ID, "Список доступных команд:\n"+
		"/start - начать регистрацию\n"+
		"/vote - начать голосование\n"+
		"/help - показать список доступных команд")
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
