package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
)

// defaultTimeout — лимит сессии, если в контексте нет дедлайна.
const defaultTimeout = 20 * time.Second

// emailRegex - регулярное выражение для базовой валидации email
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// smtpTLSConfig переопределяется в тестах (свой CA).
var smtpTLSConfig = func(host string) *tls.Config {
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
}

// validateEmail проверяет корректность email адреса
func validateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}
	if len(email) > 254 {
		return fmt.Errorf("email is too long")
	}
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// validateVerificationCode проверяет корректность кода верификации
func validateVerificationCode(code int) error {
	if code < 100000 || code > 999999 {
		return fmt.Errorf("verification code must be 6 digits")
	}
	return nil
}

// SendVerificationCodeToEmail отправляет код с таймаутом по умолчанию.
func SendVerificationCodeToEmail(email string, code int) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	return SendVerificationCodeToEmailWithContext(ctx, email, code)
}

// SendVerificationCodeToEmailWithContext отправляет код через провайдер из EMAIL_PROVIDER.
func SendVerificationCodeToEmailWithContext(ctx context.Context, email string, code int) error {
	if err := validateEmail(email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}
	if err := validateVerificationCode(code); err != nil {
		return fmt.Errorf("invalid verification code: %w", err)
	}

	subject := "Ваш код подтверждения"
	body := fmt.Sprintf("Ваш код подтверждения: %d", code)

	switch config.EmailProvider {
	case config.EmailProviderPostbox:
		return sendViaPostbox(ctx, config.SMTPEmail, email, subject, body)
	case config.EmailProviderSMTP:
		return sendViaSMTP(ctx, email, subject, body)
	default:
		return fmt.Errorf("unsupported EMAIL_PROVIDER %q (expected %q or %q)", config.EmailProvider, config.EmailProviderSMTP, config.EmailProviderPostbox)
	}
}

// sendViaSMTP отправляет письмо по SMTP; дедлайн контекста ограничивает всю сессию.
func sendViaSMTP(ctx context.Context, email, subject, body string) error {
	senderEmail := config.SMTPEmail
	host := config.SMTPHost
	mode := config.SMTPTLS
	switch mode {
	case config.SMTPTLSImplicit, config.SMTPTLSStartTLS:
	default:
		return fmt.Errorf("unsupported SMTP_TLS mode %q", mode)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(defaultTimeout)
	}

	domain := senderDomain(senderEmail)
	message := buildMessage(senderEmail, email, subject, body, domain)
	tlsConfig := smtpTLSConfig(host)

	dialer := &net.Dialer{Timeout: time.Until(deadline)}
	rawConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, config.SMTPPort))
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	if err := rawConn.SetDeadline(deadline); err != nil {
		rawConn.Close()
		return fmt.Errorf("failed to set SMTP deadline: %w", err)
	}
	// отмена контекста после dial: срываем дедлайн сокета, иначе сессия живёт до deadline
	stop := context.AfterFunc(ctx, func() { rawConn.SetDeadline(time.Now()) })
	defer stop()

	var client *smtp.Client
	switch mode {
	case config.SMTPTLSImplicit:
		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return fmt.Errorf("TLS handshake failed: %w", err)
		}
		client, err = smtp.NewClient(tlsConn, host)
		if err != nil {
			tlsConn.Close()
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
		if err := client.Hello(domain); err != nil {
			client.Close()
			return fmt.Errorf("EHLO failed: %w", err)
		}
	case config.SMTPTLSStartTLS:
		client, err = smtp.NewClient(rawConn, host)
		if err != nil {
			rawConn.Close()
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				return fmt.Errorf("failed to create SMTP client: %w (сервер молчит: возможно, порт ожидает implicit TLS — проверьте SMTP_TLS/SMTP_PORT)", err)
			}
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
		if err := client.Hello(domain); err != nil {
			client.Close()
			return fmt.Errorf("EHLO failed: %w", err)
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			client.Close()
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}
	defer client.Close()

	// PLAIN разрешён net/smtp только поверх TLS; логин может отличаться от адреса (релеи)
	smtpUser := config.SMTPUser
	if smtpUser == "" {
		smtpUser = senderEmail
	}
	if err := client.Auth(smtp.PlainAuth("", smtpUser, config.SMTPPassword, host)); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}
	if err := client.Mail(senderEmail); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}
	if err := client.Rcpt(email); err != nil {
		return fmt.Errorf("failed to set recipient %s: %w", email, err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to start data transfer: %w", err)
	}
	if _, err := w.Write([]byte(message)); err != nil {
		w.Close()
		return fmt.Errorf("failed to write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}
	_ = client.Quit() // письмо уже принято
	return nil
}

// senderDomain — домен отправителя для EHLO и Message-ID.
func senderDomain(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 && i+1 < len(addr) {
		return addr[i+1:]
	}
	return "localhost"
}

// buildMessage собирает письмо; тело в quoted-printable (кириллица без расчёта на 8BITMIME).
func buildMessage(from, to, subject, body, domain string) string {
	var qp strings.Builder
	w := quotedprintable.NewWriter(&qp)
	w.Write([]byte(body))
	w.Close()
	return strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"Message-ID: <" + randomID() + "@" + domain + ">",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		qp.String(),
		"",
	}, "\r\n")
}

func randomID() string {
	b := make([]byte, 12)
	rand.Read(b) // с Go 1.24 не возвращает ошибку
	return hex.EncodeToString(b)
}
