package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
)

func SendVerificationCodeToEmail(email string, code int) error {
	// Данные SMTP-сервера Mail.ru
	smtpHost := "smtp.mail.ru"
	smtpPort := "465"

	// Данные отправителя (логин и пароль для приложения)
	senderEmail := os.Getenv("SMTP_EMAIL")       // Логин (полный адрес почты)
	senderPassword := os.Getenv("SMTP_PASSWORD") // Пароль для почтового клиента

	if senderEmail == "" || senderPassword == "" {
		return fmt.Errorf("email.Sender: переменные SMTP_EMAIL или SMTP_PASSWORD не установлены")
	}

	// Получатель
	to := []string{email}

	// Тема и тело письма
	subject := "Ваш код подтверждения"
	body := fmt.Sprintf("Ваш код подтверждения: %d", code)
	message := []byte(fmt.Sprintf("Subject: %s\r\n\r\n%s\r\n", subject, body))

	// Аутентификация
	auth := smtp.PlainAuth("", senderEmail, senderPassword, smtpHost)

	// Настройка TLS-соединения
	tlsconfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         smtpHost,
	}

	// Установка соединения через TLS
	conn, err := tls.Dial("tcp", smtpHost+":"+smtpPort, tlsconfig)
	if err != nil {
		return fmt.Errorf("email.Sender: ошибка TLS-соединения: %w", err)
	}

	// Создание SMTP клиента
	client, err := smtp.NewClient(conn, smtpHost)
	if err != nil {
		return fmt.Errorf("email.Sender: ошибка создания SMTP-клиента: %w", err)
	}

	// Аутентификация
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("email.Sender: ошибка аутентификации SMTP: %w", err)
	}

	// Устанавливаем отправителя
	if err = client.Mail(senderEmail); err != nil {
		return fmt.Errorf("email.Sender: ошибка установки отправителя: %w", err)
	}

	// Устанавливаем получателя
	for _, addr := range to {
		if err = client.Rcpt(addr); err != nil {
			return fmt.Errorf("email.Sender: ошибка установки получателя: %w", err)
		}
	}

	// Передача данных
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email.Sender: ошибка передачи данных: %w", err)
	}

	_, err = w.Write(message)
	if err != nil {
		return fmt.Errorf("email.Sender: ошибка записи данных: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("email.Sender: ошибка закрытия записи: %w", err)
	}

	// Завершаем SMTP сессию
	err = client.Quit()
	if err != nil {
		return fmt.Errorf("email.Sender: ошибка завершения SMTP сессии: %w", err)
	}

	return nil
}
