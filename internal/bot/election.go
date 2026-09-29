package bot

import (
	"context"
	"fmt"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/utils"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Обработчик команды /vote
func (b *Bot) handleVote(ctx context.Context, message *tgbotapi.Message) {
	telegramID := message.Chat.ID
	b.mu.RLock()
	isActive := b.activeVoting
	b.mu.RUnlock()

	if !isActive {
		log.Warn(telegramID, " Попытка начать голосование при закрытом голосовании")
		b.SendMessage(telegramID, msgVoteClosed)
		return
	}
	// Проверяем, зарегистрирован ли пользователь
	ok, err := b.voteChain.CheckExistDelegateByTelegramID(ctx, telegramID)
	if err != nil {
		log.Errorf("%d Ошибка при начале голосования: %v", telegramID, err)
		b.SendMessage(telegramID, msgSysRegistrationCheckError)
		return
	}
	if !ok {
		log.Warn(telegramID, " Незарегистрированный пользователь пытается начать голосование")
		b.SendMessage(telegramID, msgVoteNotRegistered)
		return
	}
	if err := b.SendMessage(telegramID, msgVoteHelp); err != nil {
		log.Errorf("%d Ошибка получения памятки к голосованию: %v", telegramID, err)
	}

	b.mu.RLock()
	candidatesListCopy := b.candidatesList
	b.mu.RUnlock()

	if err := b.SendMessage(telegramID, candidatesListCopy); err != nil {
		log.Errorf("%d Ошибка отправки списка кандидатов: %v", telegramID, err)
	}

	// Создаем бюллетень для делегата
	b.mu.Lock()
	b.rankedList[message.Chat.ID] = []int{}
	b.mu.Unlock()
	b.sendCandidateKeyboard(ctx, message, false)
}

// Отправка бюллетеня
func (b *Bot) sendCandidateKeyboard(_ context.Context, message *tgbotapi.Message, editMsg bool) {
	telegramID := message.Chat.ID
	b.mu.RLock()
	defer b.mu.RUnlock()
	// TODO добавить кнопку отмены последнего голоса

	// Создаем кнопки выбора кандидата
	var keyboard tgbotapi.InlineKeyboardMarkup
	for _, candidateID := range b.sortedCandidatesIDs {
		// Пропускаем уже записанных кандидатов
		if contains(b.rankedList[telegramID], candidateID) {
			continue
		}
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf(msgVoteButtonFmt, b.Candidates[candidateID].Name, b.Candidates[candidateID].Course), // надпись кнопки
			strconv.Itoa(candidateID), // данные кнопки
		)
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []tgbotapi.InlineKeyboardButton{button})
	}
	// Проверяем остались ли кандидаты для выбора
	if len(keyboard.InlineKeyboard) == 0 {
		log.Warn(message.Chat.ID, " Попытка вписать кандидатов, когда все уже вписаны")
		b.spoilBallot(telegramID, message)
		return
	}

	// Отправляем сообщение с клавиатурой
	if editMsg {
		msgText := msgVoteBallotHeader
		for i, candidateID := range b.rankedList[telegramID] {
			msgText += fmt.Sprintf(msgVoteBallotLineFmt, i+1, b.Candidates[candidateID].Name)
		}
		msg := tgbotapi.NewEditMessageTextAndMarkup(
			message.Chat.ID,
			message.MessageID,
			msgText,
			keyboard,
		)
		if _, err := b.botAPI.Send(msg); err != nil {
			log.Errorf("%d ошибка записи бюллетеня: %v", telegramID, err)
		}
	} else {
		msg := tgbotapi.NewMessage(message.Chat.ID, msgVoteBallotHeader)
		msg.ReplyMarkup = keyboard
		if _, err := b.botAPI.Send(msg); err != nil {
			log.Errorf("%d ошибка отправки бюллетеня: %v", telegramID, err)
		}
	}
}

// Получение ответа кнопки
func (b *Bot) handleCallbackQuery(ctx context.Context, query *tgbotapi.CallbackQuery) {
	telegramID := query.From.ID
	b.mu.RLock()
	isActive := b.activeVoting
	b.mu.RUnlock()

	if !isActive {
		log.Warn(telegramID, " Попытка голосования при закрытом голосовании")
		b.SendMessage(telegramID, msgVoteClosed)
		return
	}
	// Извлекаем ID кандидата из данных кнопки
	candidateID, err := strconv.Atoi(query.Data)
	if err != nil {
		log.Errorf("%d Ошибка при обработке кнопки: %v", telegramID, err)
		b.SendMessage(telegramID, msgSysCallbackError)
		return
	}
	// TODO Вариант порчи бюллетеня получше того, что есть. Портит бюллетень одиножды при повторе
	// // Проверяем, есть ли уже такой кандидат в списке ранжирования
	// if contains(b.rankedList[telegramID], candidateID) {
	// 	log.Warn(telegramID, "Попытка добавить уже добавленного кандидата")
	// 	msg := tgbotapi.NewMessage(telegramID, "Этот кандидат уже добавлен в список. Пожалуйста, выберите другого кандидата.")
	// 	b.botAPI.Send(msg)
	// 	return
	// }
	b.mu.Lock()
	// Проверяем не испорчен ли бюллетень (rankedList) делегата
	if len(b.rankedList[telegramID]) >= len(b.Candidates) {
		log.Warn(telegramID, " Попытка вписать кандидатов в заполненный бюллетень")
		b.spoilBallot(telegramID, query.Message)
		b.mu.Unlock()
		return
	}
	// Добавляем ID кандидата в список ранжирования
	b.rankedList[telegramID] = append(b.rankedList[telegramID], candidateID)
	if _, err := b.botAPI.Request(tgbotapi.NewCallback(query.ID, msgVoteCallbackAccepted)); err != nil {
		log.Errorf("%d ошибка ответа на нажатие кнопки: %v", telegramID, err)
	}

	// Проверяем, все ли кандидаты ранжированы
	if len(b.rankedList[telegramID]) == len(b.Candidates) {
		// Проверяем, не испорчен ли бюллетень
		if !isUniqueCandidates(b.rankedList[telegramID]) {
			log.Warn(telegramID, " Испорченный бюллетень (повтор кандидатов)")
			b.spoilBallot(telegramID, query.Message)
			b.mu.Unlock()
			return
		}
		// Отправка бюллетеня
		b.mu.Unlock()
		b.sendRankedList(ctx, query)
		// TODO удаление списка ранжирования (Не имеет смысла при текущей реализации порчи бюллетеня)
		return
	}

	// ждем следующую отмеку в бюллетене
	b.mu.Unlock()
	b.sendCandidateKeyboard(ctx, query.Message, true)
}

// Отправка заполненного бюллетеня
func (b *Bot) sendRankedList(ctx context.Context, query *tgbotapi.CallbackQuery) {
	telegramID := query.From.ID
	b.mu.RLock()
	defer b.mu.RUnlock()
	// Отправляем бюллетень и удаляем клавиатуру
	msgText := msgVoteFinalBallotHeader
	for i, candidateID := range b.rankedList[telegramID] {
		msgText += fmt.Sprintf(msgVoteBallotLineFmt, i+1, b.Candidates[candidateID].Name)
	}
	editMsg := tgbotapi.NewEditMessageText(telegramID, query.Message.MessageID, msgText)
	if _, err := b.botAPI.Send(editMsg); err != nil {
		log.Errorf("%d ошибка отправки заполненного бюллетеня: %v", telegramID, err)
	}
	// Запись голоса в базу данных
	log.Debugf("%d rankedList: %v", telegramID, b.rankedList[telegramID])
	err := b.voteChain.AddVote(ctx, telegramID, b.rankedList[telegramID])
	if err != nil {
		log.Errorf("%d ошибка регистрации голоса: %v", telegramID, err)
		b.SendMessage(telegramID, msgSysVoteSaveError)
		return
	}

	// Генерируем токен из telegramID (детерминированный, каждый раз одинаковый)
	voteToken := utils.GenerateVoteToken(telegramID)

	// Уведомляем, что голос учтен
	successMessage := fmt.Sprintf(msgVoteAcceptedFmt, voteToken)
	if config.Domain != "" {
		successMessage += fmt.Sprintf(msgVoteBallotPageFmt, config.Domain)
	}

	if err := b.SendMessage(telegramID, successMessage); err != nil {
		log.Errorf("%d ошибка ответа о принятии бюллетеня: %v", telegramID, err)
	}
	log.Info(query.From.ID, " Голос учтен")
}

// Порча бюллетеня
func (b *Bot) spoilBallot(telegramID int64, message *tgbotapi.Message) {
	spoiledTxt := fmt.Sprintf(msgVoteSpoiledFmt, message.Text)
	spoiledMsg := tgbotapi.NewEditMessageText(
		telegramID,
		message.MessageID,
		spoiledTxt,
	)
	if _, err := b.botAPI.Send(spoiledMsg); err != nil {
		log.Errorf("%d ошибка при попытке запретить испорченный бюллетень: %v", telegramID, err)
	}
	b.SendMessage(telegramID, msgVoteSpoiledHint)
}

// Проверка уникальности кандидатов в списке
func isUniqueCandidates(rankedList []int) bool {
	seen := make(map[int]bool)
	for _, candidateID := range rankedList {
		if seen[candidateID] {
			return false
		}
		seen[candidateID] = true
	}
	return true
}

// Проверка, есть ли элемент в слайсе
func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false

}
