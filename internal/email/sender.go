package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
)

// emailRegex - регулярное выражение для базовой валидации email
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

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

func SendVerificationCodeToEmail(email string, code int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return SendVerificationCodeToEmailWithContext(ctx, email, code)
}

// SendVerificationCodeToEmailWithContext отправляет код верификации с поддержкой контекста
func SendVerificationCodeToEmailWithContext(ctx context.Context, email string, code int) error {
	// Валидация входных данных
	if err := validateEmail(email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	if err := validateVerificationCode(code); err != nil {
		return fmt.Errorf("invalid verification code: %w", err)
	}

	// Данные SMTP-сервера Mail.ru (используем порт 2525, так как 587 заблокирован)
	smtpHost := "smtp.mail.ru"
	smtpPort := "2525"

	// Данные отправителя (логин и пароль для приложения)
	senderEmail := config.SMTPEmail
	senderPassword := config.SMTPPassword

	if senderEmail == "" || senderPassword == "" {
		return fmt.Errorf("SMTP credentials not configured")
	}

	// Получатель
	to := []string{email}

	// Тема и тело письма с правильными заголовками MIME
	subject := "Ваш код подтверждения"
	body := fmt.Sprintf("Ваш код подтверждения: %d", code)

	// Формируем правильное SMTP сообщение с заголовками
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		senderEmail, strings.Join(to, ","), subject, body)

	// Аутентификация
	auth := smtp.PlainAuth("", senderEmail, senderPassword, smtpHost)

	// Создание соединения с таймаутом из контекста
	deadline, ok := ctx.Deadline()
	timeout := 15 * time.Second
	if ok {
		timeout = time.Until(deadline)
	}

	dialer := &net.Dialer{
		Timeout: timeout,
	}

	netConn, err := dialer.DialContext(ctx, "tcp", smtpHost+":"+smtpPort)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}

	conn, err := smtp.NewClient(netConn, smtpHost)
	if err != nil {
		netConn.Close()
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer conn.Close()

	// Настройка TLS с минимальной версией TLS 1.2
	tlsconfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         smtpHost,
		MinVersion:         tls.VersionTLS12,
	}

	// STARTTLS - обновляем незашифрованное соединение до TLS
	if err = conn.StartTLS(tlsconfig); err != nil {
		return fmt.Errorf("failed to start TLS: %w", err)
	}

	// Аутентификация (после установки TLS для безопасности)
	if err = conn.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Устанавливаем отправителя
	if err = conn.Mail(senderEmail); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Устанавливаем получателя
	for _, addr := range to {
		if err = conn.Rcpt(addr); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", addr, err)
		}
	}

	// Передача данных
	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("failed to start data transfer: %w", err)
	}

	_, err = w.Write([]byte(message))
	if err != nil {
		w.Close()
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	// Завершаем SMTP сессию
	err = conn.Quit()
	if err != nil {
		// Не возвращаем ошибку, так как письмо уже отправлено
		// Ошибка будет игнорирована
	}

	return nil
}
