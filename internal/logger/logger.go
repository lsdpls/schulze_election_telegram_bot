package logger

import (
	"fmt"
	"io"
	"os"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

type Logger struct {
	loggerBotAPI *tgbotapi.BotAPI
	entry        *logrus.Entry
	logFile      *os.File
}

// Create a new logger instance
func NewLogger(loggerBotAPI *tgbotapi.BotAPI, level string) *Logger {
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

	// Set log level
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		logrus.Fatalf("Invalid log level: %s", level)
	}
	logrus.SetLevel(logLevel)

	return &Logger{
		loggerBotAPI: loggerBotAPI,
		entry:        logrus.WithFields(logrus.Fields{}),
		logFile:      logFile,
	}
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// Log methods for logrus
func (l *Logger) Debug(args ...interface{}) {
	l.entry.Debug(args...)
	if l.entry.Logger.Level >= logrus.DebugLevel {
		l.sendNotification(fmt.Sprintf("🐛%sDEBUG</a>: %s", itos(args[0]), fmt.Sprint(args...)))
	}
}

func (l *Logger) Info(args ...interface{}) {
	l.entry.Info(args...)
	if l.entry.Logger.Level >= logrus.InfoLevel {
		l.sendNotification(fmt.Sprintf("🔎%sINFO</a>: %s", itos(args[0]), fmt.Sprint(args...)))
	}
}

func (l *Logger) Warn(args ...interface{}) {
	l.entry.Warn(args...)
	if l.entry.Logger.Level >= logrus.WarnLevel {
		l.sendNotification(fmt.Sprintf("⚠️%sWARN</a>: %s", itos(args[0]), fmt.Sprint(args...)))
	}
}

func (l *Logger) Error(args ...interface{}) {
	l.entry.Error(args...)
	if l.entry.Logger.Level >= logrus.ErrorLevel {
		l.sendNotification(fmt.Sprintf("📛%sERROR</a>: %s", itos(args[0]), fmt.Sprint(args...)))
	}
}

func (l *Logger) Fatal(args ...interface{}) {
	l.entry.Fatal(args...)
	if l.entry.Logger.Level >= logrus.FatalLevel {
		l.sendNotification(fmt.Sprintf("☠️%sFATAL</a>: %s", itos(args[0]), fmt.Sprint(args...)))
	}
}

func (l *Logger) Panic(args ...interface{}) {
	l.entry.Panic(args...)
	if l.entry.Logger.Level >= logrus.PanicLevel {
		l.sendNotification(fmt.Sprintf("😱%sPANIC</a>: %s", itos(args[0]), fmt.Sprint(args...)))
	}
}

// Logf methods for logrus
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.entry.Debugf(format, args...)
	if l.entry.Logger.Level >= logrus.DebugLevel {
		l.sendNotification(fmt.Sprintf("🐛%sDEBUG</a>: %s", itos(args[0]), fmt.Sprintf(format, args...)))
	}
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.entry.Infof(format, args...)
	if l.entry.Logger.Level >= logrus.InfoLevel {
		l.sendNotification(fmt.Sprintf("🔎%sINFO</a>: %s", itos(args[0]), fmt.Sprintf(format, args...)))
	}
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.entry.Warnf(format, args...)
	if l.entry.Logger.Level >= logrus.WarnLevel {
		l.sendNotification(fmt.Sprintf("⚠️%sWARN</a>: %s", itos(args[0]), fmt.Sprintf(format, args...)))
	}
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.entry.Errorf(format, args...)
	if l.entry.Logger.Level >= logrus.ErrorLevel {
		l.sendNotification(fmt.Sprintf("📛%sERROR</a>: %s", itos(args[0]), fmt.Sprintf(format, args...)))
	}
}

func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.entry.Fatalf(format, args...)
	if l.entry.Logger.Level >= logrus.FatalLevel {
		l.sendNotification(fmt.Sprintf("☠️%sFATAL</a>: %s", itos(args[0]), fmt.Sprintf(format, args...)))
	}
}
func (l *Logger) Panicf(format string, args ...interface{}) {
	l.entry.Panicf(format, args...)
	if l.entry.Logger.Level >= logrus.PanicLevel {
		l.sendNotification(fmt.Sprintf("😱%sPANIC</a>: %s", itos(args[0]), fmt.Sprintf(format, args...)))
	}
}

// Send notification to the log chat
func (l *Logger) sendNotification(message string) {
	if config.LogChatID == 0 {
		return // Не отправляем уведомления, если LogChatID не настроен
	}
	msg := tgbotapi.NewMessage(config.LogChatID, message)
	msg.ParseMode = "HTML"
	if _, err := l.loggerBotAPI.Send(msg); err != nil {
		l.entry.Debugf("Ошибка при отправке уведомления: %v", err)
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

func itos(i interface{}) string {
	switch i.(type) {
	case int:
		return fmt.Sprintf("<a href=\"tg://user?id=%d\">", i)
	case int64:
		return fmt.Sprintf("<a href=\"tg://user?id=%d\">", i)
	case string:
		return fmt.Sprintf("<a href=\"tg://user?id=%s\">", i)
	default:
		return ""
	}
}
