package config

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Все переменные, которые читает LoadConfig: t.Setenv гарантирует чистое окружение.
var allVars = []string{
	"TELEGRAM_APITOKEN", "ADMIN_CHAT_ID", "LOG_CHAT_ID", "WEBHOOK_SECRET",
	"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "POSTGRES_SSLMODE",
	"EMAIL_PROVIDER", "EMAIL_DAILY_LIMIT", "SMTP_EMAIL", "SMTP_PASSWORD", "SMTP_USER", "SMTP_HOST", "SMTP_PORT", "SMTP_TLS",
	"POSTBOX_KEY_ID", "POSTBOX_SECRET_KEY", "POSTBOX_ENDPOINT", "POSTBOX_REGION",
	"APP_PORT", "VOTE_TOKEN_SECRET", "TOTAL_PLACES", "LOG_LEVEL", "TELEGRAM_LOG_LEVEL",
}

func baseline() map[string]string {
	return map[string]string{
		"TELEGRAM_APITOKEN":  "123456:ABC-DEF",
		"ADMIN_CHAT_ID":      "-1001",
		"LOG_CHAT_ID":        "-1002",
		"WEBHOOK_SECRET":     strings.Repeat("ab", 32),
		"POSTGRES_HOST":      "localhost",
		"POSTGRES_PORT":      "5432",
		"POSTGRES_USER":      "bot",
		"POSTGRES_PASSWORD":  "secret",
		"POSTGRES_DB":        "election",
		"POSTGRES_SSLMODE":   "disable",
		"EMAIL_PROVIDER":     EmailProviderPostbox,
		"SMTP_EMAIL":         "noreply@example.org",
		"POSTBOX_KEY_ID":     "keyid",
		"POSTBOX_SECRET_KEY": "secretkey",
		"APP_PORT":           "8080",
		"VOTE_TOKEN_SECRET":  strings.Repeat("s", 64),
		"TOTAL_PLACES":       "10",
	}
}

// setEnv выставляет baseline с переопределениями; пустое значение = переменная не задана.
func setEnv(t *testing.T, overrides map[string]string) {
	t.Helper()
	env := baseline()
	for k, v := range overrides {
		env[k] = v
	}
	for _, k := range allVars {
		t.Setenv(k, env[k])
	}
}

func TestLoadConfig_Baseline(t *testing.T) {
	setEnv(t, nil)
	require.NoError(t, LoadConfig())

	assert.Equal(t, "123456:ABC-DEF", TelegramAPIToken)
	assert.Equal(t, int64(-1001), AdminChatID)
	assert.Equal(t, int64(-1002), LogChatID)
	assert.Equal(t, strings.Repeat("ab", 32), WebhookSecret)
	assert.Equal(t, EmailProviderPostbox, EmailProvider)
	assert.Equal(t, "noreply@example.org", SMTPEmail)
	assert.Equal(t, "8080", AppPort)
	assert.Equal(t, 10, TotalPlaces)

	// Значения по умолчанию
	assert.Equal(t, "https://postbox.cloud.yandex.net", PostboxEndpoint)
	assert.Equal(t, "ru-central1", PostboxRegion)
	assert.Equal(t, "smtp.mail.ru", SMTPHost)
	assert.Equal(t, "465", SMTPPort)
	assert.Equal(t, SMTPTLSImplicit, SMTPTLS)
	assert.Equal(t, "info", LogLevel)
	assert.Equal(t, "info", TelegramLogLevel)
	assert.Equal(t, 180, EmailDailyLimit)
}

func TestLoadConfig_RequiredVars(t *testing.T) {
	required := []string{
		"TELEGRAM_APITOKEN", "ADMIN_CHAT_ID", "WEBHOOK_SECRET",
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "POSTGRES_SSLMODE",
		"SMTP_EMAIL", "APP_PORT", "VOTE_TOKEN_SECRET", "TOTAL_PLACES",
	}
	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			setEnv(t, map[string]string{name: ""})
			err := LoadConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

func TestLoadConfig_Validation(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr string // подстрока ошибки; "" = успех
		check   func(t *testing.T)
	}{
		{name: "admin chat zero", env: map[string]string{"ADMIN_CHAT_ID": "0"}, wantErr: "ADMIN_CHAT_ID"},
		{name: "admin chat non-numeric", env: map[string]string{"ADMIN_CHAT_ID": "abc"}, wantErr: "ADMIN_CHAT_ID"},
		{name: "log chat non-numeric", env: map[string]string{"LOG_CHAT_ID": "abc"}, wantErr: "LOG_CHAT_ID"},
		{name: "log chat empty ok", env: map[string]string{"LOG_CHAT_ID": ""}, check: func(t *testing.T) {
			assert.Equal(t, int64(0), LogChatID)
		}},

		{name: "webhook 31", env: map[string]string{"WEBHOOK_SECRET": strings.Repeat("a", 31)}, wantErr: "WEBHOOK_SECRET"},
		{name: "webhook 32 ok", env: map[string]string{"WEBHOOK_SECRET": strings.Repeat("a", 32)}},
		{name: "webhook 256 ok", env: map[string]string{"WEBHOOK_SECRET": strings.Repeat("a", 256)}},
		{name: "webhook 257", env: map[string]string{"WEBHOOK_SECRET": strings.Repeat("a", 257)}, wantErr: "WEBHOOK_SECRET"},
		{name: "webhook dot", env: map[string]string{"WEBHOOK_SECRET": strings.Repeat("a", 31) + "."}, wantErr: "WEBHOOK_SECRET"},
		{name: "webhook underscore dash ok", env: map[string]string{"WEBHOOK_SECRET": strings.Repeat("a", 30) + "_-"}},

		{name: "postgres port non-numeric", env: map[string]string{"POSTGRES_PORT": "abc"}, wantErr: "POSTGRES_PORT"},
		{name: "postgres port 0", env: map[string]string{"POSTGRES_PORT": "0"}, wantErr: "POSTGRES_PORT"},

		{name: "provider foo", env: map[string]string{"EMAIL_PROVIDER": "foo"}, wantErr: "EMAIL_PROVIDER"},
		{name: "provider empty defaults to smtp", env: map[string]string{"EMAIL_PROVIDER": "", "SMTP_PASSWORD": "pw"}, check: func(t *testing.T) {
			assert.Equal(t, EmailProviderSMTP, EmailProvider)
		}},
		{name: "postbox without key id", env: map[string]string{"POSTBOX_KEY_ID": ""}, wantErr: "POSTBOX_KEY_ID"},
		{name: "postbox without secret", env: map[string]string{"POSTBOX_SECRET_KEY": ""}, wantErr: "POSTBOX_SECRET_KEY"},
		{name: "smtp without password", env: map[string]string{"EMAIL_PROVIDER": EmailProviderSMTP}, wantErr: "SMTP_PASSWORD"},
		{name: "smtp with password ok", env: map[string]string{"EMAIL_PROVIDER": EmailProviderSMTP, "SMTP_PASSWORD": "pw", "POSTBOX_KEY_ID": "", "POSTBOX_SECRET_KEY": ""}, check: func(t *testing.T) {
			assert.Equal(t, "pw", SMTPPassword)
		}},
		{name: "smtp user optional", env: map[string]string{"SMTP_USER": "login"}, check: func(t *testing.T) {
			assert.Equal(t, "login", SMTPUser)
		}},

		{name: "smtp email with name", env: map[string]string{"SMTP_EMAIL": "Name <a@b.org>"}, wantErr: "SMTP_EMAIL"},
		{name: "smtp email angle only", env: map[string]string{"SMTP_EMAIL": "<a@b.org>"}, wantErr: "SMTP_EMAIL"},
		{name: "smtp email garbage", env: map[string]string{"SMTP_EMAIL": "not-an-address"}, wantErr: "SMTP_EMAIL"},

		{name: "endpoint http", env: map[string]string{"POSTBOX_ENDPOINT": "http://x"}, wantErr: "POSTBOX_ENDPOINT"},
		{name: "endpoint with path", env: map[string]string{"POSTBOX_ENDPOINT": "https://x/v2/email"}, wantErr: "POSTBOX_ENDPOINT"},
		{name: "endpoint with query", env: map[string]string{"POSTBOX_ENDPOINT": "https://x?a=b"}, wantErr: "POSTBOX_ENDPOINT"},
		{name: "endpoint with userinfo", env: map[string]string{"POSTBOX_ENDPOINT": "https://user@x"}, wantErr: "POSTBOX_ENDPOINT"},
		{name: "endpoint trailing slash normalised", env: map[string]string{"POSTBOX_ENDPOINT": "https://x/"}, check: func(t *testing.T) {
			assert.Equal(t, "https://x", PostboxEndpoint)
		}},
		{name: "endpoint with port ok", env: map[string]string{"POSTBOX_ENDPOINT": "https://x:8443"}, check: func(t *testing.T) {
			assert.Equal(t, "https://x:8443", PostboxEndpoint)
		}},
		{name: "region with space", env: map[string]string{"POSTBOX_REGION": "ru central1"}, wantErr: "POSTBOX_REGION"},
		{name: "region custom ok", env: map[string]string{"POSTBOX_REGION": "ru-central2"}, check: func(t *testing.T) {
			assert.Equal(t, "ru-central2", PostboxRegion)
		}},

		{name: "smtp host with port", env: map[string]string{"SMTP_HOST": "smtp:465"}, wantErr: "SMTP_HOST"},
		{name: "smtp port 0", env: map[string]string{"SMTP_PORT": "0"}, wantErr: "SMTP_PORT"},
		{name: "smtp port 70000", env: map[string]string{"SMTP_PORT": "70000"}, wantErr: "SMTP_PORT"},
		{name: "smtp port abc", env: map[string]string{"SMTP_PORT": "abc"}, wantErr: "SMTP_PORT"},
		{name: "smtp tls unknown", env: map[string]string{"SMTP_TLS": "ssl"}, wantErr: "SMTP_TLS"},
		{name: "465 + starttls", env: map[string]string{"SMTP_PORT": "465", "SMTP_TLS": SMTPTLSStartTLS}, wantErr: "SMTP_TLS"},
		{name: "587 + tls", env: map[string]string{"SMTP_PORT": "587", "SMTP_TLS": SMTPTLSImplicit}, wantErr: "SMTP_TLS"},
		{name: "587 default tls", env: map[string]string{"SMTP_PORT": "587"}, wantErr: "SMTP_TLS"},
		{name: "587 + starttls ok", env: map[string]string{"SMTP_PORT": "587", "SMTP_TLS": SMTPTLSStartTLS}, check: func(t *testing.T) {
			assert.Equal(t, "587", SMTPPort)
			assert.Equal(t, SMTPTLSStartTLS, SMTPTLS)
		}},
		{name: "2525 + starttls ok", env: map[string]string{"SMTP_PORT": "2525", "SMTP_TLS": SMTPTLSStartTLS}},
		{name: "8465 + tls ok", env: map[string]string{"SMTP_PORT": "8465", "SMTP_TLS": SMTPTLSImplicit}},
		{name: "8465 + starttls ok", env: map[string]string{"SMTP_PORT": "8465", "SMTP_TLS": SMTPTLSStartTLS}},

		{name: "log level fatal", env: map[string]string{"LOG_LEVEL": "fatal"}, check: func(t *testing.T) {
			assert.Equal(t, "fatal", LogLevel)
		}},
		{name: "log level warning", env: map[string]string{"LOG_LEVEL": "warning"}},
		{name: "log level TRACE", env: map[string]string{"LOG_LEVEL": "TRACE"}},
		{name: "log level loud", env: map[string]string{"LOG_LEVEL": "loud"}, wantErr: "LOG_LEVEL"},
		{name: "tg log level fatal", env: map[string]string{"TELEGRAM_LOG_LEVEL": "fatal"}, check: func(t *testing.T) {
			assert.Equal(t, "fatal", TelegramLogLevel)
		}},
		{name: "tg log level warning", env: map[string]string{"TELEGRAM_LOG_LEVEL": "warning"}},
		{name: "tg log level TRACE", env: map[string]string{"TELEGRAM_LOG_LEVEL": "TRACE"}},
		{name: "tg log level loud", env: map[string]string{"TELEGRAM_LOG_LEVEL": "loud"}, wantErr: "TELEGRAM_LOG_LEVEL"},

		{name: "daily limit 0", env: map[string]string{"EMAIL_DAILY_LIMIT": "0"}, wantErr: "EMAIL_DAILY_LIMIT"},
		{name: "daily limit -1", env: map[string]string{"EMAIL_DAILY_LIMIT": "-1"}, wantErr: "EMAIL_DAILY_LIMIT"},
		{name: "daily limit x", env: map[string]string{"EMAIL_DAILY_LIMIT": "x"}, wantErr: "EMAIL_DAILY_LIMIT"},
		{name: "daily limit 500 ok", env: map[string]string{"EMAIL_DAILY_LIMIT": "500"}, check: func(t *testing.T) {
			assert.Equal(t, 500, EmailDailyLimit)
		}},

		{name: "app port abc", env: map[string]string{"APP_PORT": "abc"}, wantErr: "APP_PORT"},
		{name: "app port 0", env: map[string]string{"APP_PORT": "0"}, wantErr: "APP_PORT"},
		{name: "vote secret short", env: map[string]string{"VOTE_TOKEN_SECRET": strings.Repeat("s", 31)}, wantErr: "VOTE_TOKEN_SECRET"},
		{name: "total places 0", env: map[string]string{"TOTAL_PLACES": "0"}, wantErr: "TOTAL_PLACES"},
		{name: "total places abc", env: map[string]string{"TOTAL_PLACES": "abc"}, wantErr: "TOTAL_PLACES"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, tc.env)
			err := LoadConfig()
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t)
			}
		})
	}
}

func TestLoadConfig_DatabaseURL(t *testing.T) {
	// Разбираем DSN тем же парсером, что и pgxpool, — проверка экранирования
	parse := func(t *testing.T) *pgconn.Config {
		t.Helper()
		require.NoError(t, LoadConfig())
		cfg, err := pgconn.ParseConfig(DatabaseURL)
		require.NoError(t, err, "DSN: %s", DatabaseURL)
		return cfg
	}

	passwords := []string{"pa ss", "p+q", "p@q/r?s#t", "p%40q", "pässwörd", "a b+c", "it's", "x:y"}
	users := []string{"us er", "u:x"}

	for _, pw := range passwords {
		t.Run("password "+pw, func(t *testing.T) {
			setEnv(t, map[string]string{"POSTGRES_PASSWORD": pw})
			cfg := parse(t)
			assert.Equal(t, "bot", cfg.User)
			assert.Equal(t, pw, cfg.Password)
			assert.Equal(t, "localhost", cfg.Host)
			assert.Equal(t, uint16(5432), cfg.Port)
			assert.Equal(t, "election", cfg.Database)
			assert.Nil(t, cfg.TLSConfig)
		})
	}

	for _, u := range users {
		t.Run("user "+u, func(t *testing.T) {
			setEnv(t, map[string]string{"POSTGRES_USER": u})
			cfg := parse(t)
			assert.Equal(t, u, cfg.User)
			assert.Equal(t, "secret", cfg.Password)
			assert.Equal(t, "localhost", cfg.Host)
			assert.Equal(t, uint16(5432), cfg.Port)
			assert.Equal(t, "election", cfg.Database)
			assert.Nil(t, cfg.TLSConfig)
		})
	}

	t.Run("db with question mark", func(t *testing.T) {
		setEnv(t, map[string]string{"POSTGRES_DB": "db?x"})
		cfg := parse(t)
		assert.Equal(t, "db?x", cfg.Database)
		assert.Nil(t, cfg.TLSConfig)
	})

	t.Run("ipv6 host", func(t *testing.T) {
		setEnv(t, map[string]string{"POSTGRES_HOST": "::1"})
		cfg := parse(t)
		assert.Equal(t, "::1", cfg.Host)
		assert.Equal(t, uint16(5432), cfg.Port)
	})

	t.Run("custom port", func(t *testing.T) {
		setEnv(t, map[string]string{"POSTGRES_PORT": "6543"})
		cfg := parse(t)
		assert.Equal(t, uint16(6543), cfg.Port)
	})
}
