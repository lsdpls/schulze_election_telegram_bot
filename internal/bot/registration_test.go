package bot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mocks "github.com/lsdpls/schulze_election_telegram_bot/internal/bot/mocks"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/chain"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/golang/mock/gomock"
)

// fakeTelegram — минимальный Bot API: getMe и sendMessage; тексты отправленных сообщений копятся в sent
type fakeTelegram struct {
	*httptest.Server
	mu   sync.Mutex
	sent []string
}

func newFakeTelegram(t *testing.T) (*fakeTelegram, *tgbotapi.BotAPI) {
	t.Helper()
	f := &fakeTelegram{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var result any
		switch path.Base(r.URL.Path) {
		case "getMe":
			result = map[string]any{"id": 1, "is_bot": true, "first_name": "test", "username": "test_bot"}
		case "sendMessage":
			text := r.FormValue("text")
			f.mu.Lock()
			f.sent = append(f.sent, text)
			n := len(f.sent)
			f.mu.Unlock()
			chatID, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
			result = map[string]any{"message_id": n, "chat": map[string]any{"id": chatID, "type": "private"}, "text": text}
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": 404, "description": "Not Found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(f.Close)
	api, err := tgbotapi.NewBotAPIWithClient("123:TEST", f.URL+"/bot%s/%s", f.Client())
	if err != nil {
		t.Fatalf("NewBotAPIWithClient: %v", err)
	}
	return f, api
}

func (f *fakeTelegram) texts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

func (f *fakeTelegram) last() string {
	sent := f.texts()
	if len(sent) == 0 {
		return ""
	}
	return sent[len(sent)-1]
}

// newRegistrationBot — бот с фейковым Telegram и моком БД: делегат 123456 существует; CheckFerification задаёт тест
func newRegistrationBot(t *testing.T) (*Bot, *fakeTelegram, *mocks.MockvoteChain) {
	t.Helper()
	oldProvider := config.EmailProvider
	config.EmailProvider = "none" // SendVerificationCodeToEmail отказывает сразу, без сети
	t.Cleanup(func() { config.EmailProvider = oldProvider })

	tg, api := newFakeTelegram(t)
	db := mocks.NewMockvoteChain(gomock.NewController(t))
	db.EXPECT().CheckExistDelegateByDelegateID(gomock.Any(), 123456).Return(true, nil).AnyTimes()

	b := newTestBot()
	b.botAPI, b.voteChain = api, db
	return b, tg, db
}

func setAdminChat(t *testing.T, id int64) {
	t.Helper()
	old := config.AdminChatID
	config.AdminChatID = id
	t.Cleanup(func() { config.AdminChatID = old })
}

func privateMsg(chatID int64, text string) *tgbotapi.Message {
	return &tgbotapi.Message{Text: text, Chat: &tgbotapi.Chat{ID: chatID, Type: "private"}}
}

func emailMsg(chatID int64) *tgbotapi.Message { return privateMsg(chatID, "st123456") }

func commandMsg(chatID int64, chatType, text string) *tgbotapi.Message {
	return &tgbotapi.Message{Text: text, Chat: &tgbotapi.Chat{ID: chatID, Type: chatType}, From: &tgbotapi.User{ID: 77},
		Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(strings.Fields(text)[0])}}}
}

func adminCommand(chatID int64, text string) *tgbotapi.Message {
	return commandMsg(chatID, "supergroup", text)
}

func pending(t *testing.T, b *Bot, chatID int64) (code int, ok bool, state string) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	code, ok = b.codeStore[chatID]
	return code, ok, b.userStates[chatID]
}

// Письмо не ушло → код всё равно сохранён (ждём ввода), резерв снят: повтор через /start сразу
// получает ту же ошибку отправки (не кулдаун) и тот же код, квота не тратится
func TestHandleEmailInputReleasesReservationOnSendError(t *testing.T) {
	setDailyLimit(t, 10)
	b, tg, db := newRegistrationBot(t)
	db.EXPECT().CheckFerification(gomock.Any(), 123456).Return(false, nil).Times(2)
	const chatID int64 = 424242
	b.userStates[chatID] = StateWaitingForEmail

	const wantErr = "Не удалось отправить письмо"
	b.handleText(context.Background(), emailMsg(chatID))
	if got := tg.last(); !strings.Contains(got, wantErr) {
		t.Fatalf("1-й ответ %q, want %q", got, wantErr)
	}
	code, ok, state := pending(t, b, chatID)
	if !ok || state != StateWaitingForCode {
		t.Fatalf("код не сохранён (%v) или состояние %q, want %q", ok, state, StateWaitingForCode)
	}

	b.mu.Lock()
	b.userStates[chatID] = StateWaitingForEmail // как после повторного /start
	b.mu.Unlock()
	b.handleText(context.Background(), emailMsg(chatID))
	got := tg.last()
	if strings.Contains(got, "Повторный запрос кода") {
		t.Fatalf("2-й ответ — кулдаун вместо ошибки отправки: %q", got)
	}
	if !strings.Contains(got, wantErr) {
		t.Fatalf("2-й ответ %q, want %q", got, wantErr)
	}
	if n := len(tg.texts()); n != 2 {
		t.Fatalf("отправлено %d сообщений, want 2: %q", n, tg.texts())
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.codeStore[chatID] != code {
		t.Fatalf("код сменился: %d → %d", code, b.codeStore[chatID])
	}
	if b.emailsSent != 0 || b.codesByTGDay[chatID] != 0 || b.codesByDelegateDay[123456] != 0 {
		t.Fatalf("квота потрачена: emailsSent=%d byTG=%d byDelegate=%d", b.emailsSent, b.codesByTGDay[chatID], b.codesByDelegateDay[123456])
	}
	if len(b.lastCodeByTG) != 0 || len(b.lastCodeByDelegate) != 0 {
		t.Fatalf("кулдауны не сняты: %v %v", b.lastCodeByTG, b.lastCodeByDelegate)
	}
	if b.userStates[chatID] != StateWaitingForCode {
		t.Fatalf("состояние %q, want %q", b.userStates[chatID], StateWaitingForCode)
	}
}

// Лимит писем исчерпан → письмо не шлётся, но код сгенерирован и ждёт ввода; админ видит его через /show_code
func TestHandleEmailInputLimitKeepsCodeForAdmin(t *testing.T) {
	setDailyLimit(t, 10)
	setAdminChat(t, -1001234)
	b, tg, db := newRegistrationBot(t)
	db.EXPECT().CheckFerification(gomock.Any(), 123456).Return(false, nil).AnyTimes()
	db.EXPECT().CheckExistDelegateByDelegateID(gomock.Any(), 654321).Return(false, nil)
	const chatID int64 = 424242
	b.userStates[chatID] = StateWaitingForEmail
	b.codesByTGDay[chatID] = perTGDailyLimit
	b.emailsDay = time.Now().UTC().Format("2006-01-02") // текущие сутки, чтобы rollover не обнулил счётчик

	b.handleText(context.Background(), emailMsg(chatID))
	if got := tg.last(); !strings.Contains(got, "Лимит писем") {
		t.Fatalf("ответ %q, want лимит", got)
	}
	code, ok, state := pending(t, b, chatID)
	if !ok || state != StateWaitingForCode {
		t.Fatalf("код не сохранён (%v) или состояние %q", ok, state)
	}

	b.handleCommand(context.Background(), adminCommand(config.AdminChatID, "/show_code st123456"))
	if got := tg.last(); !strings.Contains(got, strconv.Itoa(code)) || !strings.Contains(got, strconv.FormatInt(chatID, 10)) {
		t.Fatalf("/show_code не показал код %d для %d: %q", code, chatID, got)
	}
	b.handleCommand(context.Background(), adminCommand(config.AdminChatID, "/show_code 654321"))
	if got := tg.last(); !strings.Contains(got, "не найден") {
		t.Fatalf("/show_code для несуществующего делегата: %q", got)
	}
	b.handleCommand(context.Background(), adminCommand(config.AdminChatID, "/show_code"))
	if got := tg.last(); !strings.Contains(got, "Использование") {
		t.Fatalf("/show_code без аргумента: %q", got)
	}

	// Введённый код (выданный админом) завершает регистрацию
	db.EXPECT().VerificateDelegate(gomock.Any(), 123456, sql.NullInt64{Int64: chatID, Valid: true}).Return(nil)
	b.handleText(context.Background(), privateMsg(chatID, strconv.Itoa(code)))
	if got := tg.last(); !strings.Contains(got, "Регистрация успешно завершена") {
		t.Fatalf("ввод кода: %q", got)
	}
	if _, ok, _ := pending(t, b, chatID); ok {
		t.Fatal("код не удалён после регистрации")
	}
}

// /show_code работает только в админ-чате: в личке — «Неизвестная команда», в чужой группе — тишина
func TestShowCodeOnlyInAdminChat(t *testing.T) {
	setAdminChat(t, -1001234)
	b, tg, _ := newRegistrationBot(t)
	b.codeStore[1], b.userEmail[1], b.userStates[1] = 777777, 123456, StateWaitingForCode

	b.handleCommand(context.Background(), commandMsg(5, "private", "/show_code 123456"))
	if got := tg.last(); strings.Contains(got, "777777") || !strings.Contains(got, "Неизвестная команда") {
		t.Fatalf("личка: %q", got)
	}
	n := len(tg.texts())
	b.handleCommand(context.Background(), commandMsg(-1009999, "supergroup", "/show_code 123456"))
	if len(tg.texts()) != n {
		t.Fatalf("чужая группа получила ответ: %q", tg.last())
	}
}

// Делегат уже зарегистрирован: /show_code не выдаёт код и чистит ожидающие записи
func TestShowCodeRegisteredDelegate(t *testing.T) {
	setAdminChat(t, -1001234)
	b, tg, db := newRegistrationBot(t)
	db.EXPECT().CheckFerification(gomock.Any(), 123456).Return(true, nil)
	b.codeStore[1], b.userEmail[1], b.userStates[1] = 777777, 123456, StateWaitingForCode

	b.handleCommand(context.Background(), adminCommand(config.AdminChatID, "/show_code 123456"))
	if got := tg.last(); strings.Contains(got, "777777") || !strings.Contains(got, "уже зарегистрирован") {
		t.Fatalf("ответ: %q", got)
	}
	if _, ok, _ := pending(t, b, 1); ok {
		t.Fatal("устаревший код не удалён")
	}
}

// Устаревший код другого аккаунта: после регистрации tg2 записи tg1 удаляются, а если всё же введён —
// chain отказывает (ErrAlreadyVerified) и бот не перепривязывает делегата
func TestCodeInputRejectsAlreadyRegistered(t *testing.T) {
	setDailyLimit(t, 10)
	b, tg, db := newRegistrationBot(t)
	const tg1, tg2 int64 = 1001, 1002
	b.codeStore[tg1], b.userEmail[tg1], b.userStates[tg1] = 111111, 123456, StateWaitingForCode
	b.codeStore[tg2], b.userEmail[tg2], b.userStates[tg2] = 222222, 123456, StateWaitingForCode

	db.EXPECT().VerificateDelegate(gomock.Any(), 123456, sql.NullInt64{Int64: tg2, Valid: true}).Return(nil)
	b.handleText(context.Background(), privateMsg(tg2, "222222"))
	if got := tg.last(); !strings.Contains(got, "Регистрация успешно завершена") {
		t.Fatalf("tg2: %q", got)
	}
	if _, ok, _ := pending(t, b, tg1); ok {
		t.Fatal("код tg1 не удалён после регистрации tg2")
	}

	// как после рестарта: запись tg1 снова есть, но БД уже привязала делегата к tg2
	b.codeStore[tg1], b.userEmail[tg1], b.userStates[tg1] = 111111, 123456, StateWaitingForCode
	db.EXPECT().VerificateDelegate(gomock.Any(), 123456, sql.NullInt64{Int64: tg1, Valid: true}).Return(fmt.Errorf("chain: %w", chain.ErrAlreadyVerified))
	b.handleText(context.Background(), privateMsg(tg1, "111111"))
	if got := tg.last(); !strings.Contains(got, "уже зарегистрирован с другого аккаунта") {
		t.Fatalf("tg1: %q", got)
	}
	if _, ok, state := pending(t, b, tg1); ok || state != "" {
		t.Fatalf("состояние tg1 не сброшено: ok=%v state=%q", ok, state)
	}
}

// Письмо ушло, пользователь снова нажал /start и ввёл почту: его переводят к вводу кода без второго письма
func TestEmailInputDuringCooldownSwitchesToCode(t *testing.T) {
	setDailyLimit(t, 10)
	b, tg, db := newRegistrationBot(t)
	db.EXPECT().CheckFerification(gomock.Any(), 123456).Return(false, nil).Times(2)
	const chatID int64 = 424242
	mustReserve(t, b, chatID, 123456, time.Now()) // письмо только что ушло
	b.codeStore[chatID], b.userEmail[chatID] = 555555, 123456
	b.userStates[chatID] = StateWaitingForEmail // после повторного /start

	b.handleText(context.Background(), emailMsg(chatID))
	if got := tg.last(); !strings.Contains(got, "Код уже отправлен") {
		t.Fatalf("ответ: %q", got)
	}
	if code, ok, state := pending(t, b, chatID); !ok || code != 555555 || state != StateWaitingForCode {
		t.Fatalf("code=%d ok=%v state=%q", code, ok, state)
	}
	if b.emailsSent != 1 {
		t.Fatalf("второе письмо посчитано: emailsSent=%d", b.emailsSent)
	}

	// В ожидании кода снова прислали почту — это обычный запрос письма (упрётся в кулдаун)
	b.handleText(context.Background(), emailMsg(chatID))
	if got := tg.last(); !strings.Contains(got, "Код уже отправлен") {
		t.Fatalf("повторная почта в ожидании кода: %q", got)
	}
	db.EXPECT().VerificateDelegate(gomock.Any(), 123456, sql.NullInt64{Int64: chatID, Valid: true}).Return(nil)
	b.handleText(context.Background(), privateMsg(chatID, "555555"))
	if got := tg.last(); !strings.Contains(got, "Регистрация успешно завершена") {
		t.Fatalf("ввод кода: %q", got)
	}
}

// Кулдаун, но ожидающий код у аккаунта для другого делегата: об этом говорят явно, состояние не меняется
func TestEmailInCooldownMentionsOtherPendingDelegate(t *testing.T) {
	setDailyLimit(t, 10)
	b, tg, db := newRegistrationBot(t)
	db.EXPECT().CheckFerification(gomock.Any(), 123456).Return(false, nil)
	const chatID int64 = 424242
	mustReserve(t, b, chatID, 111111, time.Now()) // письмо для st111111 только что ушло
	b.codeStore[chatID], b.userEmail[chatID], b.userStates[chatID] = 555555, 111111, StateWaitingForCode

	b.handleText(context.Background(), emailMsg(chatID)) // опечатка исправлена: теперь st123456
	got := tg.last()
	if !strings.Contains(got, "st111111") || !strings.Contains(got, "st123456") {
		t.Fatalf("ответ не называет оба адреса: %q", got)
	}
	if code, ok, state := pending(t, b, chatID); !ok || code != 555555 || b.userEmail[chatID] != 111111 || state != StateWaitingForCode {
		t.Fatalf("состояние изменилось: code=%d ok=%v delegate=%d state=%q", code, ok, b.userEmail[chatID], state)
	}
}

// 5 неверных кодов подряд аннулируют код: перебор 6-значного кода невозможен
func TestCodeInputAttemptLimit(t *testing.T) {
	setDailyLimit(t, 10)
	b, tg, _ := newRegistrationBot(t)
	const chatID int64 = 424242
	b.codeStore[chatID], b.userEmail[chatID], b.userStates[chatID] = 555555, 123456, StateWaitingForCode

	for i := 1; i < maxCodeAttempts; i++ {
		b.handleText(context.Background(), privateMsg(chatID, "000000"))
		if got := tg.last(); !strings.Contains(got, "Неверный код") {
			t.Fatalf("попытка %d: %q", i, got)
		}
	}
	b.handleText(context.Background(), privateMsg(chatID, "000000"))
	if got := tg.last(); !strings.Contains(got, "Слишком много неверных попыток") {
		t.Fatalf("после %d попыток: %q", maxCodeAttempts, got)
	}
	if _, ok, state := pending(t, b, chatID); ok || state != "" {
		t.Fatalf("код не аннулирован: ok=%v state=%q", ok, state)
	}
	// верный код уже не принимается
	b.handleText(context.Background(), privateMsg(chatID, "555555"))
	if got := tg.last(); strings.Contains(got, "Регистрация успешно завершена") {
		t.Fatalf("аннулированный код принят: %q", got)
	}
}
