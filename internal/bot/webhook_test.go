package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

const testSecret = "test-secret-token-1234567890abcdef"

// logHook собирает записи logrus для проверок в тестах
var logHook *logtest.Hook

// Логгер пакета пишет в ./logs/bot.log, поэтому тесты идут из временного каталога.
// botAPI == nil безопасен: sendNotification выходит при LogChatID == 0 или nil API.
func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	dir, err := os.MkdirTemp("", "bot-test-")
	if err != nil {
		panic(err)
	}
	if err := os.Mkdir(dir+"/logs", 0o755); err != nil {
		panic(err)
	}
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	config.LogLevel, config.TelegramLogLevel = "debug", "panic"
	log = logger.NewLogger(nil, config.LogLevel, config.TelegramLogLevel)
	logHook = logtest.NewGlobal()

	code := m.Run()
	log.Close()
	// Каталог не удалить, пока он текущий (macOS)
	if err := os.Chdir(wd); err != nil {
		panic(err)
	}
	os.RemoveAll(dir)
	os.Exit(code)
}

func webhookRequest(method, secret, body string) *http.Request {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	if secret != "" {
		req.Header.Set(secretHeader, secret)
	}
	return req
}

// Запросы без верного secret_token отвергаются до разбора тела.
func TestHandleWebhookRejects(t *testing.T) {
	config.WebhookSecret = testSecret
	b := &Bot{} // поля бота до проверки секрета не нужны
	forged := `{"update_id":1,"message":{"message_id":1,"chat":{"id":424242,"type":"private"},"text":"/stop_voting","entities":[{"type":"bot_command","offset":0,"length":12}]}}`

	cases := []struct {
		name   string
		method string
		secret string
		want   int
	}{
		{"GET", http.MethodGet, testSecret, http.StatusMethodNotAllowed},
		{"missing header", http.MethodPost, "", http.StatusForbidden},
		{"wrong secret", http.MethodPost, "wrong", http.StatusForbidden},
		{"prefix of secret", http.MethodPost, testSecret[:len(testSecret)-1], http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			b.HandleWebhook(rec, webhookRequest(tc.method, tc.secret, forged))
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// Без настроенного секрета вебхук закрыт для всех, включая пустой заголовок.
func TestHandleWebhookFailsClosedWithoutSecret(t *testing.T) {
	config.WebhookSecret = ""
	t.Cleanup(func() { config.WebhookSecret = testSecret })
	b := &Bot{}
	for _, secret := range []string{"", "anything"} {
		rec := httptest.NewRecorder()
		b.HandleWebhook(rec, webhookRequest(http.MethodPost, secret, `{"update_id":1}`))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("secret %q: got %d, want 503", secret, rec.Code)
		}
	}
}

// С верным секретом отвечаем 200 и на битое/огромное тело, и на update, который бот игнорирует.
func TestHandleWebhookAccepts(t *testing.T) {
	config.WebhookSecret = testSecret
	b := &Bot{} // botAPI == nil: любой вызов Telegram API уронил бы тест
	huge := `{"update_id":1,"message":{"message_id":1,"chat":{"id":1,"type":"private"},"text":"` + strings.Repeat("a", 2<<20) + `"}}`

	cases := []struct {
		name    string
		body    string
		wantLog string // подстрока в логе, если ожидается
	}{
		{"empty update", `{"update_id":1}`, ""},
		{"bad json", `{not json`, "Error decoding update"},
		{"2 MiB body", huge, "request body too large"},
		{"group text", `{"update_id":1,"message":{"message_id":1,"chat":{"id":-100123,"type":"group"},"text":"hello"}}`, ""},
		{"callback without message", `{"update_id":1,"callback_query":{"id":"1","from":{"id":5},"data":"7"}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logHook.Reset()
			rec := httptest.NewRecorder()
			b.HandleWebhook(rec, webhookRequest(http.MethodPost, testSecret, tc.body))
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}
			if tc.wantLog != "" {
				if e := logHook.LastEntry(); e == nil || !strings.Contains(e.Message, tc.wantLog) {
					t.Fatalf("log %v lacks %q", e, tc.wantLog)
				}
			}
			if !b.mu.TryLock() {
				t.Fatal("b.mu остался захваченным")
			}
			b.mu.Unlock()
		})
	}
}

// Неполные объекты update пропускаются без паники и без утечки мьютекса.
func TestHandleUpdateIgnoresIncomplete(t *testing.T) {
	b := &Bot{}
	updates := []tgbotapi.Update{
		{CallbackQuery: &tgbotapi.CallbackQuery{ID: "1", From: &tgbotapi.User{ID: 5}, Data: "7"}},  // нет Message
		{CallbackQuery: &tgbotapi.CallbackQuery{ID: "2", Message: &tgbotapi.Message{}, Data: "7"}}, // нет From
		{Message: &tgbotapi.Message{Text: "/start"}},                                               // нет Chat
	}
	for i, u := range updates {
		b.HandleUpdate(context.Background(), u)
		if !b.mu.TryLock() {
			t.Fatalf("update %d: b.mu остался захваченным", i)
		}
		b.mu.Unlock()
	}
}

// Каждый отказ вебхука логируется; значение заголовка с секретом в лог не попадает
func TestLogWebhookRejectEveryTime(t *testing.T) {
	logHook.Reset()
	const leak = "header-value-must-not-leak"
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(secretHeader, leak)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	for i := 0; i < 3; i++ {
		logWebhookReject(req, http.StatusForbidden, true)
	}
	entries := logHook.AllEntries()
	if len(entries) != 3 {
		t.Fatalf("got %d log lines, want 3", len(entries))
	}
	msg := entries[0].Message
	for _, want := range []string{"status=403", "method=POST", "203.0.113.7", "secret_header=true"} {
		if !strings.Contains(msg, want) {
			t.Errorf("log %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, leak) {
		t.Errorf("log %q leaks the secret header", msg)
	}
	if entries[0].Level != logrus.WarnLevel {
		t.Errorf("level %v, want warn", entries[0].Level)
	}
}

// Паника обработчика (здесь: команда в личке при botAPI == nil) не оставляет Telegram без ответа 200
func TestHandleWebhookRecoversFromPanic(t *testing.T) {
	config.WebhookSecret = testSecret
	b := &Bot{}
	body := `{"update_id":9,"message":{"message_id":1,"chat":{"id":5,"type":"private"},"text":"/zzz","entities":[{"type":"bot_command","offset":0,"length":4}]}}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(secretHeader, testSecret)
	rec := httptest.NewRecorder()
	logHook.Reset()
	b.HandleWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if e := logHook.LastEntry(); e == nil || !strings.Contains(e.Message, "panic") {
		t.Fatalf("паника не залогирована: %v", e)
	}
}
