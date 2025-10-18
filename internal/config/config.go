package config

import (
	"fmt"
	"os"
	"strconv"
)

// Telegram Bot
var TelegramAPIToken string
var AdminChatID int64
var LogChatID int64

// Database
var DatabaseURL string
var PostgresHost string
var PostgresPort int
var PostgresUser string
var PostgresPassword string
var PostgresDB string
var PostgresSSLMode string

// SMTP
var SMTPEmail string
var SMTPPassword string

// App
var AppPort string

// Vote Token Security
var VoteTokenSecret string

// Election
var TotalPlaces int

// Logging
var LogLevel string
var TelegramLogLevel string

// LoadConfig загружает и валидирует конфигурацию из переменных окружения
func LoadConfig() error {
	// Telegram Bot
	TelegramAPIToken = os.Getenv("TELEGRAM_APITOKEN")
	if TelegramAPIToken == "" {
		return fmt.Errorf("TELEGRAM_APITOKEN is required")
	}

	AdminChatIDStr := os.Getenv("ADMIN_CHAT_ID")
	if AdminChatIDStr != "" {
		var err error
		AdminChatID, err = strconv.ParseInt(AdminChatIDStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ADMIN_CHAT_ID: %v", err)
		}
	}

	LogChatIDStr := os.Getenv("LOG_CHAT_ID")
	if LogChatIDStr != "" {
		var err error
		LogChatID, err = strconv.ParseInt(LogChatIDStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid LOG_CHAT_ID: %v", err)
		}
	}

	// Database
	PostgresHost = os.Getenv("POSTGRES_HOST")
	if PostgresHost == "" {
		return fmt.Errorf("POSTGRES_HOST is required")
	}

	PostgresPortStr := os.Getenv("POSTGRES_PORT")
	if PostgresPortStr == "" {
		return fmt.Errorf("POSTGRES_PORT is required")
	}
	var err error
	PostgresPort, err = strconv.Atoi(PostgresPortStr)
	if err != nil {
		return fmt.Errorf("invalid POSTGRES_PORT: %v", err)
	}

	PostgresUser = os.Getenv("POSTGRES_USER")
	if PostgresUser == "" {
		return fmt.Errorf("POSTGRES_USER is required")
	}

	PostgresPassword = os.Getenv("POSTGRES_PASSWORD")
	if PostgresPassword == "" {
		return fmt.Errorf("POSTGRES_PASSWORD is required")
	}

	PostgresDB = os.Getenv("POSTGRES_DB")
	if PostgresDB == "" {
		return fmt.Errorf("POSTGRES_DB is required")
	}

	PostgresSSLMode = os.Getenv("POSTGRES_SSLMODE")
	if PostgresSSLMode == "" {
		return fmt.Errorf("POSTGRES_SSLMODE is required")
	}

	// SMTP
	SMTPEmail = os.Getenv("SMTP_EMAIL")
	if SMTPEmail == "" {
		return fmt.Errorf("SMTP_EMAIL is required")
	}

	SMTPPassword = os.Getenv("SMTP_PASSWORD")
	if SMTPPassword == "" {
		return fmt.Errorf("SMTP_PASSWORD is required")
	}

	// App Port
	AppPort = os.Getenv("APP_PORT")
	if AppPort == "" {
		return fmt.Errorf("APP_PORT is required")
	}

	// Vote Token Secret
	VoteTokenSecret = os.Getenv("VOTE_TOKEN_SECRET")
	if VoteTokenSecret == "" {
		return fmt.Errorf("VOTE_TOKEN_SECRET is required")
	}
	if len(VoteTokenSecret) < 32 {
		return fmt.Errorf("VOTE_TOKEN_SECRET must be at least 32 characters long")
	}

	// Election
	TotalPlacesStr := os.Getenv("TOTAL_PLACES")
	if TotalPlacesStr == "" {
		return fmt.Errorf("TOTAL_PLACES is required")
	}
	TotalPlaces, err = strconv.Atoi(TotalPlacesStr)
	if err != nil {
		return fmt.Errorf("invalid TOTAL_PLACES: %v", err)
	}
	if TotalPlaces <= 0 {
		return fmt.Errorf("TOTAL_PLACES must be greater than 0")
	}

	// Собираем DATABASE_URL из отдельных компонентов
	DatabaseURL = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		PostgresUser,
		PostgresPassword,
		PostgresHost,
		PostgresPort,
		PostgresDB,
		PostgresSSLMode,
	)

	// Logging
	LogLevel = os.Getenv("LOG_LEVEL")
	if LogLevel == "" {
		LogLevel = "debug" // Значение по умолчанию
	}

	TelegramLogLevel = os.Getenv("TELEGRAM_LOG_LEVEL")
	if TelegramLogLevel == "" {
		TelegramLogLevel = "info" // Значение по умолчанию
	}

	return nil
}
