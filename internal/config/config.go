package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// Провайдеры почты (EMAIL_PROVIDER).
const (
	EmailProviderSMTP    = "smtp"
	EmailProviderPostbox = "postbox"
)

// Режимы шифрования SMTP (SMTP_TLS).
const (
	SMTPTLSImplicit = "tls"      // SMTPS, порт 465
	SMTPTLSStartTLS = "starttls" // STARTTLS, порты 25/587/2525
)

// Telegram Bot
var TelegramAPIToken string
var AdminChatID int64
var LogChatID int64
var WebhookSecret string // secret_token из setWebhook

// Database
var DatabaseURL string
var PostgresHost string
var PostgresPort int
var PostgresUser string
var PostgresPassword string
var PostgresDB string
var PostgresSSLMode string

// Email
var EmailProvider string // EmailProviderSMTP или EmailProviderPostbox
var EmailDailyLimit int  // потолок писем в сутки (квота Postbox по умолчанию 200)

// SMTP. SMTPEmail — адрес отправителя для обоих провайдеров.
var SMTPEmail string
var SMTPPassword string
var SMTPUser string // логин, если не равен SMTPEmail
var SMTPHost string // smtp.mail.ru по умолчанию
var SMTPPort string // 465 по умолчанию
var SMTPTLS string  // SMTPTLSImplicit или SMTPTLSStartTLS

// Yandex Cloud Postbox: статический ключ сервисного аккаунта с ролью postbox.sender
var PostboxKeyID string
var PostboxSecretKey string
var PostboxEndpoint string // https://host, без пути
var PostboxRegion string

// App
var AppPort string
var Domain string // необязательно: домен страницы бюллетеней для ссылки делегату

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

	// 0 сделал бы админом любой update без chat.id
	AdminChatIDStr := os.Getenv("ADMIN_CHAT_ID")
	if AdminChatIDStr == "" {
		return fmt.Errorf("ADMIN_CHAT_ID is required")
	}
	{
		var err error
		AdminChatID, err = strconv.ParseInt(AdminChatIDStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ADMIN_CHAT_ID: %v", err)
		}
		if AdminChatID == 0 {
			return fmt.Errorf("ADMIN_CHAT_ID must be a non-zero Telegram chat id")
		}
	}

	WebhookSecret = os.Getenv("WEBHOOK_SECRET")
	if WebhookSecret == "" {
		return fmt.Errorf("WEBHOOK_SECRET is required (32..256 символов из A-Za-z0-9_-, например openssl rand -hex 32; передаётся в setWebhook как secret_token)")
	}
	if len(WebhookSecret) < 32 || len(WebhookSecret) > 256 {
		return fmt.Errorf("WEBHOOK_SECRET must be 32..256 characters long")
	}
	for _, c := range WebhookSecret {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return fmt.Errorf("WEBHOOK_SECRET may contain only A-Z, a-z, 0-9, '_' and '-' (Telegram restriction)")
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
	if PostgresPort < 1 || PostgresPort > 65535 {
		return fmt.Errorf("invalid POSTGRES_PORT %d (expected 1..65535)", PostgresPort)
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

	// Email
	EmailProvider = os.Getenv("EMAIL_PROVIDER")
	if EmailProvider == "" {
		EmailProvider = EmailProviderSMTP
	}
	if EmailProvider != EmailProviderSMTP && EmailProvider != EmailProviderPostbox {
		return fmt.Errorf("invalid EMAIL_PROVIDER: %q (expected %q or %q)", EmailProvider, EmailProviderSMTP, EmailProviderPostbox)
	}

	// Только голый адрес: он уходит в MAIL FROM, логин PLAIN и FromEmailAddress
	SMTPEmail = os.Getenv("SMTP_EMAIL")
	if SMTPEmail == "" {
		return fmt.Errorf("SMTP_EMAIL is required (адрес отправителя)")
	}
	if addr, err := mail.ParseAddress(SMTPEmail); err != nil || addr.Name != "" || addr.Address != SMTPEmail {
		return fmt.Errorf("invalid SMTP_EMAIL %q (expected a bare address like noreply@example.org)", SMTPEmail)
	}

	EmailDailyLimit = 180
	if s := os.Getenv("EMAIL_DAILY_LIMIT"); s != "" {
		EmailDailyLimit, err = strconv.Atoi(s)
		if err != nil || EmailDailyLimit <= 0 {
			return fmt.Errorf("invalid EMAIL_DAILY_LIMIT %q (expected a positive integer)", s)
		}
	}

	SMTPPassword = os.Getenv("SMTP_PASSWORD")
	if SMTPPassword == "" && EmailProvider == EmailProviderSMTP {
		return fmt.Errorf("SMTP_PASSWORD is required")
	}

	SMTPUser = os.Getenv("SMTP_USER") // необязательно

	PostboxKeyID = os.Getenv("POSTBOX_KEY_ID")
	PostboxSecretKey = os.Getenv("POSTBOX_SECRET_KEY")
	if EmailProvider == EmailProviderPostbox && (PostboxKeyID == "" || PostboxSecretKey == "") {
		return fmt.Errorf("POSTBOX_KEY_ID and POSTBOX_SECRET_KEY are required when EMAIL_PROVIDER=postbox")
	}
	PostboxEndpoint = os.Getenv("POSTBOX_ENDPOINT")
	if PostboxEndpoint == "" {
		PostboxEndpoint = "https://postbox.cloud.yandex.net"
	}
	// Подписанный запрос без TLS утёк бы открытым текстом; путь/query сломали бы подпись
	if u, err := url.Parse(PostboxEndpoint); err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid POSTBOX_ENDPOINT %q (expected https://host without path)", PostboxEndpoint)
	} else {
		PostboxEndpoint = "https://" + u.Host
	}
	PostboxRegion = os.Getenv("POSTBOX_REGION")
	if PostboxRegion == "" {
		PostboxRegion = "ru-central1"
	}
	if strings.ContainsAny(PostboxRegion, " /\t\r\n") {
		return fmt.Errorf("invalid POSTBOX_REGION %q", PostboxRegion)
	}

	SMTPHost = os.Getenv("SMTP_HOST")
	if SMTPHost == "" {
		SMTPHost = "smtp.mail.ru"
	}
	if strings.ContainsAny(SMTPHost, " :/\t\r\n") {
		return fmt.Errorf("invalid SMTP_HOST %q (hostname only, port goes to SMTP_PORT)", SMTPHost)
	}

	SMTPPort = os.Getenv("SMTP_PORT")
	if SMTPPort == "" {
		SMTPPort = "465"
	}
	if p, err := strconv.Atoi(SMTPPort); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid SMTP_PORT %q (expected 1..65535)", SMTPPort)
	}

	SMTPTLS = os.Getenv("SMTP_TLS")
	if SMTPTLS == "" {
		SMTPTLS = SMTPTLSImplicit
	}
	if SMTPTLS != SMTPTLSImplicit && SMTPTLS != SMTPTLSStartTLS {
		return fmt.Errorf("invalid SMTP_TLS: %q (expected %q for port 465 or %q for ports 25/587/2525)", SMTPTLS, SMTPTLSImplicit, SMTPTLSStartTLS)
	}
	// Неверная пара режим/порт иначе висит до дедлайна с невнятным i/o timeout
	if SMTPPort == "465" && SMTPTLS != SMTPTLSImplicit ||
		(SMTPPort == "25" || SMTPPort == "587" || SMTPPort == "2525") && SMTPTLS != SMTPTLSStartTLS {
		return fmt.Errorf("SMTP_TLS=%s does not match SMTP_PORT=%s (465 → %s, 25/587/2525 → %s)", SMTPTLS, SMTPPort, SMTPTLSImplicit, SMTPTLSStartTLS)
	}

	// App Port
	AppPort = os.Getenv("APP_PORT")
	if AppPort == "" {
		return fmt.Errorf("APP_PORT is required")
	}
	if p, err := strconv.Atoi(AppPort); err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid APP_PORT %q (expected 1..65535)", AppPort)
	}
	Domain = strings.TrimSpace(os.Getenv("DOMAIN"))
	if strings.ContainsAny(Domain, " /:\t\r\n") {
		return fmt.Errorf("invalid DOMAIN %q (hostname only)", Domain)
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

	// DATABASE_URL: url.URL экранирует userinfo, путь и query по правилам каждой части
	DatabaseURL = (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(PostgresUser, PostgresPassword),
		Host:     net.JoinHostPort(PostgresHost, strconv.Itoa(PostgresPort)),
		Path:     "/" + PostgresDB,
		RawQuery: url.Values{"sslmode": {PostgresSSLMode}}.Encode(),
	}).String()

	// Logging: уровни logrus (panic, fatal, error, warn, info, debug, trace)
	LogLevel = os.Getenv("LOG_LEVEL")
	if LogLevel == "" {
		LogLevel = "info"
	}
	if _, err := logrus.ParseLevel(LogLevel); err != nil {
		return fmt.Errorf("invalid LOG_LEVEL %q: %v", LogLevel, err)
	}

	TelegramLogLevel = os.Getenv("TELEGRAM_LOG_LEVEL")
	if TelegramLogLevel == "" {
		TelegramLogLevel = "info"
	}
	if _, err := logrus.ParseLevel(TelegramLogLevel); err != nil {
		return fmt.Errorf("invalid TELEGRAM_LOG_LEVEL %q: %v", TelegramLogLevel, err)
	}

	return nil
}
