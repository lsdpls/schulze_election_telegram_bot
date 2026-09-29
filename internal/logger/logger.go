package logger

import (
	"fmt"
	"html"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

// Текст в лог-чат режется до telegramMaxBody байт: лимит Telegram — 4096 на сообщение вместе с разметкой
const telegramMaxBody = 3500

type Logger struct {
	loggerBotAPI     *tgbotapi.BotAPI
	entry            *logrus.Entry
	logFile          *os.File
	telegramLogLevel logrus.Level
}

// Create a new logger instance
func NewLogger(loggerBotAPI *tgbotapi.BotAPI, level string, telegramLevel string) *Logger {
	// Configure logrus
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "15:04:05",
		ForceColors:     true,
	})

	// Открываем файл для записи логов
	logFile, err := os.OpenFile("./logs/bot.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		logrus.Fatalf("Ошибка при открытии файла логов: %v", err)
	}
	// Создаем MultiWriter для записи в консоль и файл
	logrus.SetOutput(io.MultiWriter(os.Stdout, logFile))

	// Set general log level
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logrus.Fatalf("Invalid log level: %s", level)
	}
	logrus.SetLevel(logLevel)

	// Parse telegram log level
	telegramLogLevel, err := logrus.ParseLevel(telegramLevel)
	if err != nil {
		logrus.Fatalf("Invalid telegram log level: %s", telegramLevel)
	}

	return &Logger{
		loggerBotAPI:     loggerBotAPI,
		entry:            logrus.WithFields(logrus.Fields{}),
		logFile:          logFile,
		telegramLogLevel: telegramLogLevel,
	}
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// Log methods: пишут через logrus и дублируют строку в лог-чат, если уровень проходит порог
func (l *Logger) Debug(args ...interface{}) {
	l.entry.Debug(args...)
	l.notify(logrus.DebugLevel, fmt.Sprint(args...), args)
}

func (l *Logger) Info(args ...interface{}) {
	l.entry.Info(args...)
	l.notify(logrus.InfoLevel, fmt.Sprint(args...), args)
}

func (l *Logger) Warn(args ...interface{}) {
	l.entry.Warn(args...)
	l.notify(logrus.WarnLevel, fmt.Sprint(args...), args)
}

func (l *Logger) Error(args ...interface{}) {
	l.entry.Error(args...)
	l.notify(logrus.ErrorLevel, fmt.Sprint(args...), args)
}

// Fatal/Panic: уведомление до entry.Fatal/Panic, иначе до него не дойдёт
func (l *Logger) Fatal(args ...interface{}) {
	l.notify(logrus.FatalLevel, fmt.Sprint(args...), args)
	l.entry.Fatal(args...)
}

func (l *Logger) Panic(args ...interface{}) {
	l.notify(logrus.PanicLevel, fmt.Sprint(args...), args)
	l.entry.Panic(args...)
}

// Logf methods for logrus
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.entry.Debugf(format, args...)
	l.notify(logrus.DebugLevel, fmt.Sprintf(format, args...), args)
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.entry.Infof(format, args...)
	l.notify(logrus.InfoLevel, fmt.Sprintf(format, args...), args)
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.entry.Warnf(format, args...)
	l.notify(logrus.WarnLevel, fmt.Sprintf(format, args...), args)
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.entry.Errorf(format, args...)
	l.notify(logrus.ErrorLevel, fmt.Sprintf(format, args...), args)
}

func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.notify(logrus.FatalLevel, fmt.Sprintf(format, args...), args)
	l.entry.Fatalf(format, args...)
}

func (l *Logger) Panicf(format string, args ...interface{}) {
	l.notify(logrus.PanicLevel, fmt.Sprintf(format, args...), args)
	l.entry.Panicf(format, args...)
}

// emoji и подпись уровня в лог-чате
var levelTags = map[logrus.Level][2]string{
	logrus.DebugLevel: {"🐛", "DEBUG"},
	logrus.InfoLevel:  {"🔎", "INFO"},
	logrus.WarnLevel:  {"⚠️", "WARN"},
	logrus.ErrorLevel: {"📛", "ERROR"},
	logrus.FatalLevel: {"☠️", "FATAL"},
	logrus.PanicLevel: {"😱", "PANIC"},
}

// notify шлёт строку в лог-чат, если уровень проходит порог; из args нужен только первый — ID пользователя
func (l *Logger) notify(level logrus.Level, text string, args []interface{}) {
	if l.telegramLogLevel < level {
		return
	}
	tag := levelTags[level]
	l.sendNotification(formatNotification(tag[1], tag[0], text, args...))
}

// formatNotification собирает HTML для лог-чата: emoji + LEVEL (ссылкой tg://user, если первый аргумент — int/int64 ID)
// + экранированный текст, обрезанный до telegramMaxBody. Битый UTF-8 (например, из SMTP-ответа) Telegram
// отвергает целиком — заменяем на U+FFFD
func formatNotification(level, emoji, text string, args ...interface{}) string {
	tag := level
	if len(args) > 0 {
		switch id := args[0].(type) {
		case int:
			tag = fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, id, level)
		case int64:
			tag = fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, id, level)
		}
	}
	text = strings.ToValidUTF8(text, "�")
	return emoji + tag + ": " + truncate(html.EscapeString(text), telegramMaxBody)
}

// truncate обрезает s до max байт по границе руны, не разрывая HTML-сущность (&amp;), и добавляет "…"
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if amp := strings.LastIndexByte(s[:cut], '&'); amp >= 0 && !strings.Contains(s[amp:cut], ";") {
		cut = amp
	}
	return s[:cut] + "…"
}

// sendNotification шлёт сообщение в лог-чат; ошибка отправки пишется только в файл/stdout, без рекурсии
func (l *Logger) sendNotification(message string) {
	if config.LogChatID == 0 || l.loggerBotAPI == nil {
		return // Не отправляем уведомления, если LogChatID не настроен
	}
	msg := tgbotapi.NewMessage(config.LogChatID, message)
	msg.ParseMode = "HTML"
	if _, err := l.loggerBotAPI.Send(msg); err != nil {
		l.entry.Warnf("Ошибка при отправке уведомления в лог-чат: %v", err)
	}
}

func (l *Logger) SetLevel(level string) error {
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	l.entry.Logger.SetLevel(logLevel)
	// logrus.SetLevel(logLevel)
	return nil
}

func (l *Logger) SetTelegramLevel(level string) error {
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	l.telegramLogLevel = logLevel
	return nil
}
