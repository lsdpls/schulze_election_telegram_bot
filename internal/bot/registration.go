package bot

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	emailSender "github.com/lsdpls/schulze_election_telegram_bot/internal/email"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Обработчик команды /start
func (b *Bot) handleStart(ctx context.Context, message *tgbotapi.Message) {

	//Проверка на то, что пользователь уже зарегистрирован
	ok, err := b.voteChain.CheckExistDelegateByTelegramID(ctx, message.Chat.ID)
	if err != nil {
		log.Errorf("%d Ошибка при проверке делегата: %v", message.Chat.ID, err)
		b.SendMessage(message.Chat.ID, "Произошла ошибка при проверке делегата. Пожалуйста, попробуйте снова")
		return
	}
	if ok {
		log.Warn(message.Chat.ID, " Попытка повторной регистрации")
		b.SendMessage(message.Chat.ID, "Вы уже зарегистрированы! Используйте команду /vote для голосования")
		return
	}

	// Отправляем приветственное сообщение
	b.SendMessage(message.Chat.ID, "Добро пожаловать в бот для голосования на выборах в Студенческий совет ПМ-ПУ!\n\n"+
		"Чтобы начать голосование, необходимо пройти регистрацию.\n"+
		"<b>Пожалуйста, введите свою st почту (в формате: stXXXXXX)</b>")

	// Устанавливаем состояние ожидания почты
	b.mu.Lock()
	defer b.mu.Unlock()
	b.userStates[message.Chat.ID] = StateWaitingForEmail
}

// Обработчик ввода почты
func (b *Bot) handleEmailInput(ctx context.Context, message *tgbotapi.Message) {
	email := strings.TrimSpace(message.Text)
	telegramID := message.Chat.ID

	// Проверяем формат email
	if !isValidEmail(email) {
		log.Debug(telegramID, " Неверный формат почты")
		b.SendMessage(telegramID, "Неверный формат почты. Пожалуйста, введите почту в формате stXXXXXX.")
		return
	}
	// Извлекаем ID делегата из почты
	delegateID, err := strconv.Atoi(email[2:])
	if err != nil {
		log.Errorf("%d Ошибка при извлечении ID делегата из почты: %v", telegramID, err)
		b.SendMessage(telegramID, "Невозможно получить ID делегата из почты. Пожалуйста, попробуйте снова")
		return
	}
	// Проверяем, существует ли такой делегат в базе данных
	ok, err := b.voteChain.CheckExistDelegateByDelegateID(ctx, delegateID)
	if err != nil {
		log.Errorf("%d Ошибка проверки существования делегата: %v", telegramID, err)
		b.SendMessage(telegramID, "Произошла ошибка при проверке делегата. Пожалуйста, попробуйте снова")
		return
	}
	if !ok {
		log.Warn(telegramID, " Попытка регистрации несуществующего делегата")
		b.SendMessage(telegramID, "Такой делегат не найден. Убедитесь, что вы ввели правильную почту.")
		return
	}
	// Проверяем, зарегистрирован ли уже делегат с такой почтой
	ok, err = b.voteChain.CheckFerification(ctx, delegateID)
	if err != nil {
		log.Errorf("%d Ошибка проверки уникальности делегата: %v", telegramID, err)
		b.SendMessage(telegramID, "Произошла ошибка при проверке делегата. Пожалуйста, попробуйте снова")
		return
	}
	if ok {
		log.Warn(telegramID, " Попытка регистрации уже зарегистрированного делегата")
		b.SendMessage(telegramID, "Делегат с такой почтой уже зарегистрировался")
		return
	}

	// Генерируем и отправляем код
	code := generateCode()
	email = fmt.Sprintf("%s@student.spbu.ru", email)
	if err := emailSender.SendVerificationCodeToEmail(email, code); err != nil {
		log.Errorf("%d Ошибка отправки кода на почту: %v", telegramID, err)
		b.SendMessage(telegramID, "Произошла ошибка при отправке кода на почту. Пожалуйста, попробуйте снова")
		return
	}
	log.Debugf("%d Код подтверждения %d отправлен на почту", telegramID, code)
	// Сохраняем сгенерированный код в мапу для дальнейшей проверки
	b.mu.Lock()
	defer b.mu.Unlock()
	b.codeStore[telegramID] = code
	if err := b.SendMessage(telegramID, "Код подтверждения отправлен на ваш email. Пожалуйста, введите код.\nЕсли Вы не видите пиьсмо - проверьте Спам или обратитесь к организаторам"); err != nil {
		log.Errorf("%d Ошибка уведомления об отправке кода: %v", telegramID, err)
	}

	// Устанавливаем email в мапу для дальнейшего использования
	b.userEmail[telegramID] = delegateID
	// Устанавливаем состояние ожидания кода
	b.userStates[telegramID] = StateWaitingForCode
}

// Обработчик ввода кода
func (b *Bot) handleCodeInput(ctx context.Context, message *tgbotapi.Message) {
	telegramID := message.Chat.ID

	// Извлекаем введенный код
	code, err := strconv.Atoi(message.Text)
	if err != nil {
		log.Debug(telegramID, " Неверный формат кода")
		b.SendMessage(telegramID, "Неверный формат кода. Пожалуйста, введите числовой код.")
		return
	}
	// Проверяем, есть ли сгенерированный код для этого пользователя
	b.mu.Lock()
	defer b.mu.Unlock()
	expectedCode, ok := b.codeStore[telegramID]
	if !ok {
		log.Error(telegramID, " Не найден код для подтверждения")
		b.SendMessage(telegramID, "Не найден код для подтверждения. Попробуйте начать регистрацию заново.")
		return
	}
	// Сравниваем введенный код с ожидаемым
	if code != expectedCode {
		log.Debug(telegramID, " Неверный код")
		b.SendMessage(telegramID, "Неверный код. Попробуйте еще раз.")
		return
	}

	// Верифицируем делегата
	delegateID := b.userEmail[telegramID]
	if err := b.voteChain.VerificateDelegate(ctx, delegateID, sql.NullInt64{Int64: telegramID, Valid: true}); err != nil {
		log.Errorf("%d Ошибка верификации делегата: %v", telegramID, err)
		b.SendMessage(telegramID, "Произошла ошибка при верификации. Пожалуйста, попробуйте снова")
		return
	}
	// Уведомляем пользователя о завершении регистрации
	if err := b.SendMessage(telegramID, "Регистрация успешно завершена! Используйте команду /vote для голосования"); err != nil {
		log.Errorf("%d Ошибка уведомления о завершении регистрации: %v", telegramID, err)
	}
	log.Info(telegramID, " Регистрация прошла успешно")

	// Сбрасываем состояния пользователя
	delete(b.userStates, telegramID)
	delete(b.codeStore, telegramID)
	delete(b.userEmail, telegramID)
}

// Проверка формата email
func isValidEmail(email string) bool {
	re := regexp.MustCompile(`^st\d{6}$`)
	return re.MatchString(email)
}

// Генерирует шестизначный код подтверждения
func generateCode() int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(900000) + 100000
}
