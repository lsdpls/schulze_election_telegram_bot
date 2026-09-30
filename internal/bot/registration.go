package bot

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/chain"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	emailSender "github.com/lsdpls/schulze_election_telegram_bot/internal/email"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Повторный код одному пользователю или на одного делегата — не чаще раза в codeCooldown
const codeCooldown = 60 * time.Second

// Неверных вводов кода подряд, после которых код аннулируется (6 цифр нельзя перебирать)
const maxCodeAttempts = 10

// Писем в сутки на один Telegram-аккаунт и на одного делегата (с любых аккаунтов). Код при этом
// генерируется и хранится всегда — админ выдаёт его вручную через /show_code, если письмо не дошло
const (
	perTGDailyLimit       = 2
	perDelegateDailyLimit = 10
)

var (
	errDailyLimit = errors.New("суточный лимит писем исчерпан")
	errUserLimit  = errors.New("лимит запросов кода на сегодня исчерпан")
)

// Обработчик команды /start
func (b *Bot) handleStart(ctx context.Context, message *tgbotapi.Message) {

	//Проверка на то, что пользователь уже зарегистрирован
	ok, err := b.voteChain.CheckExistDelegateByTelegramID(ctx, message.Chat.ID)
	if err != nil {
		log.Errorf("%d Ошибка при проверке делегата: %v", message.Chat.ID, err)
		b.SendMessage(message.Chat.ID, msgSysDelegateCheckError)
		return
	}
	if ok {
		log.Warn(message.Chat.ID, " Попытка повторной регистрации")
		b.SendMessage(message.Chat.ID, msgRegAlreadyRegistered)
		return
	}

	// Код уже ждёт ввода (повторный /start после письма): остаёмся в ожидании кода, а не просим почту заново.
	// Другую почту в этом состоянии ввести можно: handleCodeInput передаёт её в handleEmailInput
	if delegateID, ok := b.resumePendingCode(message.Chat.ID); ok {
		log.Debugf("%d Повторный /start при ожидающем коде st%06d", message.Chat.ID, delegateID)
		b.SendMessage(message.Chat.ID, fmt.Sprintf(msgRegStartCodePendingFmt, delegateID))
		return
	}

	// Отправляем приветственное сообщение
	b.SendMessage(message.Chat.ID, msgRegWelcome)

	// Устанавливаем состояние ожидания почты
	b.mu.Lock()
	defer b.mu.Unlock()
	b.userStates[message.Chat.ID] = StateWaitingForEmail
}

// resumePendingCode: если у аккаунта есть ожидающий код, переводит его в ожидание кода и возвращает делегата. Сам берёт b.mu.
func (b *Bot) resumePendingCode(telegramID int64) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.codeStore[telegramID]; !ok {
		return 0, false
	}
	b.userStates[telegramID] = StateWaitingForCode
	return b.userEmail[telegramID], true
}

// Обработчик ввода почты
func (b *Bot) handleEmailInput(ctx context.Context, message *tgbotapi.Message) {
	email := normalizeEmail(strings.TrimSpace(message.Text))
	telegramID := message.Chat.ID

	// Проверяем формат email
	if !isValidEmail(email) {
		log.Debug(telegramID, " Неверный формат почты")
		b.SendMessage(telegramID, msgRegInvalidEmail)
		return
	}
	// Извлекаем ID делегата из почты
	delegateID, err := strconv.Atoi(email[2:])
	if err != nil {
		log.Errorf("%d Ошибка при извлечении ID делегата из почты: %v", telegramID, err)
		b.SendMessage(telegramID, msgRegEmailParseError)
		return
	}
	// Проверяем, существует ли такой делегат в базе данных
	ok, err := b.voteChain.CheckExistDelegateByDelegateID(ctx, delegateID)
	if err != nil {
		log.Errorf("%d Ошибка проверки существования делегата: %v", telegramID, err)
		b.SendMessage(telegramID, msgSysDelegateCheckError)
		return
	}
	if !ok {
		log.Warn(telegramID, " Попытка регистрации несуществующего делегата")
		b.SendMessage(telegramID, msgRegDelegateNotFound)
		return
	}
	// Проверяем, зарегистрирован ли уже делегат с такой почтой
	ok, err = b.voteChain.CheckFerification(ctx, delegateID)
	if err != nil {
		log.Errorf("%d Ошибка проверки уникальности делегата: %v", telegramID, err)
		b.SendMessage(telegramID, msgSysDelegateCheckError)
		return
	}
	if ok {
		log.Warn(telegramID, " Попытка регистрации уже зарегистрированного делегата")
		b.SendMessage(telegramID, msgRegDelegateTaken)
		return
	}

	// Резервируем отправку до письма: перебор /start и дубли одного update не должны жечь квоту.
	// Код существует всегда: при лимите или сбое почты админ выдаёт его через /show_code
	now := time.Now()
	wait, err := b.reserveCodeSend(telegramID, delegateID, now)
	switch {
	case wait > 0:
		log.Debug(telegramID, " Повторный запрос кода раньше времени")
		secs := int(math.Ceil(wait.Seconds()))
		// Код уже есть (например, повторный /start после письма): ждём его ввода, а не новую почту
		if b.switchToPendingCode(telegramID, delegateID) {
			b.SendMessage(telegramID, fmt.Sprintf(msgRegCodeAlreadySentFmt, delegateID, secs))
			return
		}
		if other, ok := b.pendingDelegate(telegramID); ok {
			b.SendMessage(telegramID, fmt.Sprintf(msgRegOtherCodePendingFmt, other, delegateID, secs))
			return
		}
		b.SendMessage(telegramID, fmt.Sprintf(msgRegCooldownFmt, secs))
		return
	case err != nil:
		// Письмо с действующим кодом на эту почту уже ушло: не отправляем к организаторам, а просим ввести код
		alreadyEmailed := b.pendingCodeEmailed(telegramID, delegateID)
		if _, genErr := b.ensurePendingCode(telegramID, delegateID); genErr != nil {
			log.Errorf("%d Ошибка генерации кода: %v", telegramID, genErr)
			b.SendMessage(telegramID, msgSysGenericError)
			return
		}
		if errors.Is(err, errUserLimit) {
			log.Warnf("%d Лимит писем на сегодня исчерпан (%d на аккаунт, %d на делегата); код st%06d выдать вручную: /show_code %06d", telegramID, perTGDailyLimit, perDelegateDailyLimit, delegateID, delegateID)
		} else {
			log.Errorf("%d Суточный лимит писем (%d) исчерпан, письма не отправляются; коды выдавать вручную: /show_code <delegate_id>", telegramID, config.EmailDailyLimit)
		}
		if alreadyEmailed {
			b.SendMessage(telegramID, fmt.Sprintf(msgRegEmailLimitCodeSentFmt, delegateID))
			return
		}
		b.SendMessage(telegramID, msgRegEmailLimit)
		return
	}

	// Код сохраняем ДО отправки: если письмо не дошло, админ выдаст его через /show_code
	code, err := b.ensurePendingCode(telegramID, delegateID)
	if err != nil {
		b.releaseCodeSend(telegramID, delegateID, now, true)
		log.Errorf("%d Ошибка генерации кода: %v", telegramID, err)
		b.SendMessage(telegramID, msgSysGenericError)
		return
	}
	email = fmt.Sprintf("%s@student.spbu.ru", email)
	if err := emailSender.SendVerificationCodeToEmail(email, code); err != nil {
		// ErrMaybeSent: ответа нет, письмо могло уйти — квоту не возвращаем
		b.releaseCodeSend(telegramID, delegateID, now, !errors.Is(err, emailSender.ErrMaybeSent))
		log.Errorf("%d Ошибка отправки кода на почту st%06d: %v (выдать вручную: /show_code %06d)", telegramID, delegateID, err, delegateID)
		b.SendMessage(telegramID, msgRegEmailSendFailed)
		return
	}
	b.markCodeEmailed(telegramID, delegateID)
	log.Debugf("%d Код подтверждения %d отправлен на почту", telegramID, code)
	if err := b.SendMessage(telegramID, msgRegCodeSent); err != nil {
		log.Errorf("%d Ошибка уведомления об отправке кода: %v", telegramID, err)
	}
}

// switchToPendingCode переводит пользователя в ожидание кода, если код для пары (telegramID, delegateID)
// уже есть. Сам берёт b.mu.
func (b *Bot) switchToPendingCode(telegramID int64, delegateID int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.codeStore[telegramID]; !ok || b.userEmail[telegramID] != delegateID {
		return false
	}
	b.userStates[telegramID] = StateWaitingForCode
	return true
}

// pendingCodeEmailed — у аккаунта ждёт ввода код именно для этого делегата, и письмо с ним точно ушло. Сам берёт b.mu.
func (b *Bot) pendingCodeEmailed(telegramID int64, delegateID int) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.codeStore[telegramID]
	return ok && b.userEmail[telegramID] == delegateID && b.codeEmailed[telegramID]
}

// markCodeEmailed отмечает, что письмо с ожидающим кодом для пары (telegramID, delegateID) ушло. Сам берёт b.mu.
func (b *Bot) markCodeEmailed(telegramID int64, delegateID int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.codeStore[telegramID]; ok && b.userEmail[telegramID] == delegateID {
		b.codeEmailed[telegramID] = true
	}
}

// pendingDelegate — делегат, код для которого ждёт ввода у этого аккаунта. Сам берёт b.mu.
func (b *Bot) pendingDelegate(telegramID int64) (int, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if _, ok := b.codeStore[telegramID]; !ok {
		return 0, false
	}
	return b.userEmail[telegramID], true
}

// dropPendingCodes удаляет ожидающие коды делегата у всех аккаунтов, кроме except. Сам берёт b.mu.
func (b *Bot) dropPendingCodes(delegateID int, except int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for tg, del := range b.userEmail {
		if del == delegateID && tg != except {
			delete(b.codeStore, tg)
			delete(b.userEmail, tg)
			delete(b.userStates, tg)
			delete(b.codeAttempts, tg)
			delete(b.codeEmailed, tg)
		}
	}
}

// ensurePendingCode возвращает ожидающий код для пары (telegramID, delegateID) или генерирует новый;
// состояние переводится в ожидание кода. Сам берёт b.mu.
func (b *Bot) ensurePendingCode(telegramID int64, delegateID int) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if code, ok := b.codeStore[telegramID]; ok && b.userEmail[telegramID] == delegateID {
		b.userStates[telegramID] = StateWaitingForCode
		return code, nil
	}
	code, err := generateCode()
	if err != nil {
		return 0, err
	}
	b.codeStore[telegramID] = code
	b.userEmail[telegramID] = delegateID
	b.userStates[telegramID] = StateWaitingForCode
	delete(b.codeAttempts, telegramID)
	delete(b.codeEmailed, telegramID) // новый код ещё никому не отправлен
	return code, nil
}

// Обработчик админ-команды /show_code <delegate_id>: ожидающие ввода коды делегата (для ручной выдачи)
func (b *Bot) handleShowCode(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	var adminID int64
	if message.From != nil {
		adminID = message.From.ID
	}
	arg := strings.TrimPrefix(strings.TrimSpace(message.CommandArguments()), "st")
	delegateID, err := strconv.Atoi(arg)
	if err != nil || delegateID <= 0 {
		if err := b.SendMessage(chatID, msgAdminShowCodeUsage); err != nil {
			log.Errorf("%d Ошибка ответа на /show_code: %v", adminID, err)
		}
		return
	}
	exists, err := b.voteChain.CheckExistDelegateByDelegateID(ctx, delegateID)
	if err != nil {
		log.Errorf("%d Ошибка проверки делегата st%06d: %v", adminID, delegateID, err)
		if err := b.SendMessage(chatID, msgAdminShowCodeCheckError); err != nil {
			log.Errorf("%d Ошибка ответа на /show_code: %v", adminID, err)
		}
		return
	}
	if !exists {
		if err := b.SendMessage(chatID, fmt.Sprintf(msgAdminShowCodeNotFoundFmt, delegateID)); err != nil {
			log.Errorf("%d Ошибка ответа на /show_code: %v", adminID, err)
		}
		return
	}
	// Уже зарегистрированному делегату код выдавать нельзя: он перепривязал бы аккаунт
	registered, err := b.voteChain.CheckFerification(ctx, delegateID)
	if err != nil {
		log.Errorf("%d Ошибка проверки регистрации st%06d: %v", adminID, delegateID, err)
		if err := b.SendMessage(chatID, msgAdminShowCodeRegCheckError); err != nil {
			log.Errorf("%d Ошибка ответа на /show_code: %v", adminID, err)
		}
		return
	}
	if registered {
		b.dropPendingCodes(delegateID, 0)
		if err := b.SendMessage(chatID, fmt.Sprintf(msgAdminShowCodeRegisteredFmt, delegateID)); err != nil {
			log.Errorf("%d Ошибка ответа на /show_code: %v", adminID, err)
		}
		return
	}
	type pending struct {
		tg   int64
		code int
	}
	var found []pending
	b.mu.RLock()
	for tg, del := range b.userEmail {
		if code, ok := b.codeStore[tg]; ok && del == delegateID {
			found = append(found, pending{tg, code})
		}
	}
	b.mu.RUnlock()
	if len(found) == 0 {
		if err := b.SendMessage(chatID, fmt.Sprintf(msgAdminShowCodeNonePendingFmt, delegateID)); err != nil {
			log.Errorf("%d Ошибка ответа на /show_code: %v", adminID, err)
		}
		return
	}
	sort.Slice(found, func(i, j int) bool { return found[i].tg < found[j].tg })
	var sb strings.Builder
	fmt.Fprintf(&sb, msgAdminShowCodeHeaderFmt, delegateID)
	for _, p := range found {
		fmt.Fprintf(&sb, msgAdminShowCodeLineFmt, p.tg, p.tg, p.code)
	}
	if err := b.SendMessage(chatID, sb.String()); err != nil {
		log.Errorf("%d Ошибка ответа на /show_code: %v", adminID, err)
	}
	log.Warnf("%d Админ запросил код st%06d (ожидающих: %d)", adminID, delegateID, len(found))
}

// reserveCodeSend резервирует отправку кода; вызывается ДО письма, чтобы дубли одного update не ушли дважды;
// если письмо не ушло, резерв снимает releaseCodeSend. Сам берёт b.mu.
// wait > 0 — код недавно уже слали этому пользователю или на этого делегата, повторить через wait;
// errUserLimit — исчерпан суточный лимит аккаунта или делегата (perTGDailyLimit / perDelegateDailyLimit);
// errDailyLimit — исчерпан общий суточный лимит config.EmailDailyLimit. Сутки по UTC.
func (b *Bot) reserveCodeSend(telegramID int64, delegateID int, now time.Time) (wait time.Duration, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if last, ok := b.lastCodeByTG[telegramID]; ok && now.Sub(last) < codeCooldown {
		return codeCooldown - now.Sub(last), nil
	}
	if last, ok := b.lastCodeByDelegate[delegateID]; ok && now.Sub(last) < codeCooldown {
		return codeCooldown - now.Sub(last), nil
	}
	if day := now.UTC().Format("2006-01-02"); day != b.emailsDay {
		b.emailsDay, b.emailsSent = day, 0
		b.codesByTGDay = make(map[int64]int)
		b.codesByDelegateDay = make(map[int]int)
	}
	if b.codesByTGDay[telegramID] >= perTGDailyLimit || b.codesByDelegateDay[delegateID] >= perDelegateDailyLimit {
		return 0, errUserLimit
	}
	if b.emailsSent >= config.EmailDailyLimit {
		return 0, errDailyLimit
	}
	b.lastCodeByTG[telegramID] = now
	b.lastCodeByDelegate[delegateID] = now
	b.emailsSent++
	b.codesByTGDay[telegramID]++
	b.codesByDelegateDay[delegateID]++
	return 0, nil
}

// releaseCodeSend снимает резерв reserveCodeSend, если письмо не ушло: кулдауны сбрасываются, при refund
// возвращается и квота. Кулдаун снимается только если его поставил этот же резерв (reservedAt), более
// поздний резерв того же ключа остаётся. Сам берёт b.mu.
func (b *Bot) releaseCodeSend(telegramID int64, delegateID int, reservedAt time.Time, refund bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if last, ok := b.lastCodeByTG[telegramID]; ok && last.Equal(reservedAt) {
		delete(b.lastCodeByTG, telegramID)
	}
	if last, ok := b.lastCodeByDelegate[delegateID]; ok && last.Equal(reservedAt) {
		delete(b.lastCodeByDelegate, delegateID)
	}
	// квота возвращается только в те же сутки: после смены суток счётчики уже чужие
	if !refund || reservedAt.UTC().Format("2006-01-02") != b.emailsDay {
		return
	}
	if b.emailsSent > 0 {
		b.emailsSent--
	}
	if b.codesByTGDay[telegramID] > 0 {
		b.codesByTGDay[telegramID]--
	}
	if b.codesByDelegateDay[delegateID] > 0 {
		b.codesByDelegateDay[delegateID]--
	}
}

// Обработчик ввода кода
func (b *Bot) handleCodeInput(ctx context.Context, message *tgbotapi.Message) {
	telegramID := message.Chat.ID

	// Снова прислали почту (хотят письмо заново): обычный путь запроса кода
	if isValidEmail(normalizeEmail(strings.TrimSpace(message.Text))) {
		b.handleEmailInput(ctx, message)
		return
	}
	// Извлекаем введенный код
	code, err := strconv.Atoi(message.Text)
	if err != nil {
		log.Debug(telegramID, " Неверный формат кода")
		b.SendMessage(telegramID, msgRegInvalidCode)
		return
	}
	// Код и делегата читаем под мьютексом, БД и Telegram — без него
	b.mu.RLock()
	expectedCode, ok := b.codeStore[telegramID]
	delegateID := b.userEmail[telegramID]
	b.mu.RUnlock()
	if !ok {
		log.Error(telegramID, " Не найден код для подтверждения")
		b.SendMessage(telegramID, msgRegCodeNotFound)
		return
	}
	// Сравниваем введенный код с ожидаемым; после maxCodeAttempts неверных код аннулируется
	if code != expectedCode {
		b.mu.Lock()
		b.codeAttempts[telegramID]++
		exhausted := b.codeAttempts[telegramID] >= maxCodeAttempts
		if exhausted {
			delete(b.codeStore, telegramID)
			delete(b.userEmail, telegramID)
			delete(b.userStates, telegramID)
			delete(b.codeAttempts, telegramID)
			delete(b.codeEmailed, telegramID)
		}
		b.mu.Unlock()
		if exhausted {
			log.Warnf("%d %d неверных кодов подряд для st%06d, код аннулирован", telegramID, maxCodeAttempts, delegateID)
			b.SendMessage(telegramID, msgRegTooManyAttempts)
			return
		}
		log.Debug(telegramID, " Неверный код")
		b.SendMessage(telegramID, msgRegWrongCode)
		return
	}

	// Верифицируем делегата; chain отказывает, если делегат уже привязан к другому аккаунту
	if err := b.voteChain.VerificateDelegate(ctx, delegateID, sql.NullInt64{Int64: telegramID, Valid: true}); err != nil {
		if errors.Is(err, chain.ErrAlreadyVerified) {
			log.Warnf("%d Код st%06d введён после регистрации делегата с другого аккаунта", telegramID, delegateID)
			b.dropPendingCodes(delegateID, 0)
			b.SendMessage(telegramID, msgRegTakenByOtherAccount)
			return
		}
		log.Errorf("%d Ошибка верификации делегата: %v", telegramID, err)
		b.SendMessage(telegramID, msgSysVerifyError)
		return
	}
	// Уведомляем пользователя о завершении регистрации
	if err := b.SendMessage(telegramID, msgRegDone); err != nil {
		log.Errorf("%d Ошибка уведомления о завершении регистрации: %v", telegramID, err)
	}
	log.Info(telegramID, " Регистрация прошла успешно")

	// Сбрасываем состояние: своё и ожидающие коды того же делегата у других аккаунтов
	b.dropPendingCodes(delegateID, 0)
}

// normalizeEmail приводит ввод почты к виду stXXXXXX: нижний регистр и без домена @student.spbu.ru
// («St117795», «ST117795@student.spbu.ru» → «st117795»). Пробелы внутри не убираются
func normalizeEmail(email string) string {
	return strings.TrimSuffix(strings.ToLower(email), "@student.spbu.ru")
}

// Проверка формата email
func isValidEmail(email string) bool {
	re := regexp.MustCompile(`^st\d{6}$`)
	return re.MatchString(email)
}

// Генерирует шестизначный код подтверждения криптографическим ГПСЧ
func generateCode() (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()) + 100000, nil
}
