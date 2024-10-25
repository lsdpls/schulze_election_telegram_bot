package bot

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"vote_system/internal/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Обработчик команды /help
func (b *Bot) handleHelpAdmin(_ context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	msg := tgbotapi.NewMessage(chatID, "Список доступных команд:\n"+
		"/add_delegate <delegate_id> <name> <group> - добавить делегата\n"+
		"/delete_delegate <delegate_id> - удалить делегата\n"+
		"/add_candidate <candidate_id> <name> <course> <description> - добавить кандидата\n"+
		"/ban_candidate <candidate_id> - заблокировать кандидата\n"+
		"/delete_candidate <candidate_id> - удалить кандидата\n"+
		"/show_delegates - показать список делегатов\n"+
		"/show_candidates - показать список кандидатов\n"+
		"/show_votes - показать список голосов\n"+
		"/start_voting - начать голосование\n"+
		"/stop_voting - остановить голосование\n"+
		"/results - вычислить результаты голосования\n"+
		"/print - вывести результаты голосования\n"+
		"/csv - сохранить результаты в CSV файл\n"+
		"/log <level> - установить уровень логирования (Debug, Info, Warn, Error)\n"+
		"/send_logs - отправить файл логов\n"+
		"/help - показать список доступных команд\n"+
		"\nВсегда используйте запятые между аргументами команды, если идет перечисление аргументов")
	b.botAPI.Send(msg)
}

// Обработчик команды /add_delegate
func (b *Bot) handleAddDelegate(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	delegateMsg := message.CommandArguments()

	// Разделяем сообщение на части
	parts := strings.Split(delegateMsg, ",")
	if len(parts) != 3 {
		log.Warn(chatID, " Неверный формат команды. Используйте: /add_delegate [delegate_id], [name], [group]")
		return
	}
	for part := range parts {
		parts[part] = strings.TrimSpace(parts[part])
	}

	// Извлекаем данные из частей сообщения
	delegateID, err := strconv.Atoi(parts[0])
	if err != nil || !isValidID(parts[0]) {
		log.Warn(chatID, " Неверный формат delegate_id. Используйте целое шестизначное число.")
		return
	}
	name := parts[1]
	group := parts[2]
	if !isValidGroup(group) {
		log.Warn(chatID, " Неверный формат group. Используйте: XX.(Б|М)XX-пу")
		return
	}

	// Создаем делегата
	delegate := models.Delegate{
		DelegateID: delegateID,
		Name:       name,
		Group:      group,
		HasVoted:   false,
	}

	// Добавляем делегата в базу данных
	if err := b.voteChain.AddDelegate(ctx, delegate); err != nil {
		log.Errorf("%d Ошибка при добавлении делегата: %v", chatID, err)
		return
	}
	log.Info(chatID, " Делегат успешно добавлен")
}

// Обработчик команды /delete_delegate
func (b *Bot) handleDeleteDelegate(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	delegateMsg := message.CommandArguments()

	// Извлекаем ID делегата из сообщения
	delegateID, err := strconv.Atoi(delegateMsg)
	if err != nil || !isValidID(delegateMsg) {
		log.Warn(chatID, " Неверный формат delegate_id. Используйте целое шестизначное число.")
		return
	}

	// Удаляем делегата из базы данных
	if err := b.voteChain.DeleteDelegate(ctx, delegateID); err != nil {
		log.Errorf("%d Ошибка при удалении делегата: %v", chatID, err)
		return
	}
	log.Info(chatID, " Делегат успешно удален")
}

// Обработчик команды /add_candidate
func (b *Bot) handleAddCandidate(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	candidateMsg := message.CommandArguments()

	// Разделяем сообщение на части
	parts := strings.Split(candidateMsg, ",")
	if len(parts) != 4 {
		log.Warn(chatID, " Неверный формат команды. Используйте: /add_candidate [candidate_id], [name], [course], [description]")
		return
	}
	for part := range parts {
		parts[part] = strings.TrimSpace(parts[part])
	}

	// Извлекаем данные из частей сообщения
	candidateID, err := strconv.Atoi(parts[0])
	if err != nil || !isValidID(parts[0]) {
		log.Warn(chatID, " Неверный формат candidate_id. Используйте целое шестизначное число.")
		return
	}
	name := parts[1]
	course := parts[2]
	if !isValidCourse(course) {
		log.Warn(chatID, " Неверный формат course. Используйте: 1 бакалавриат/магистратура")
		return
	}
	description := parts[3]

	// Создаем кандидата
	candidate := models.Candidate{
		CandidateID: candidateID,
		Name:        name,
		Course:      course,
		Description: description,
		IsEligible:  true,
	}

	// Добавляем кандидата в базу данных
	if err := b.voteChain.AddCandidate(ctx, candidate); err != nil {
		log.Errorf("%d Ошибка при добавлении кандидата: %v", chatID, err)
		return
	}
	log.Info(chatID, " Кандидат успешно добавлен")
}

// Обработчик команды /ban_candidate
func (b *Bot) handleBanCandidate(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	candidateMsg := message.CommandArguments()

	// Извлекаем ID кандидата из сообщения
	candidateID, err := strconv.Atoi(candidateMsg)
	if err != nil || !isValidID(candidateMsg) {
		log.Warn(chatID, " Неверный формат candidate_id. Используйте целое шестизначное число.")
		return
	}

	// Запрещаем кандидата
	if err := b.voteChain.BanCandidate(ctx, candidateID); err != nil {
		log.Errorf("%d Ошибка при запрете кандидата: %v", chatID, err)
		return
	}
	log.Info(chatID, " Кандидат успешно заблокирован")
}

// Обработчик команды /delete_candidate
func (b *Bot) handleDeleteCandidate(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	candidateMsg := message.CommandArguments()

	// Извлекаем ID кандидата из сообщения
	candidateID, err := strconv.Atoi(candidateMsg)
	if err != nil || !isValidID(candidateMsg) {
		log.Warn(chatID, " Неверный формат candidate_id. Используйте целое шестизначное число.")
		return
	}

	// Удаляем кандидата
	if err := b.voteChain.DeleteCandidate(ctx, candidateID); err != nil {
		log.Errorf("%d Ошибка при удалении кандидата: %v", chatID, err)
		return
	}
	log.Info(chatID, " Кандидат успешно удален")
}

// Обработчик команды /show_delegates
func (b *Bot) handleShowDelegates(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	delegates, err := b.voteChain.GetAllDelegates(ctx)
	if err != nil {
		log.Errorf("%d Ошибка при получении списка делегатов: %v", chatID, err)
		return
	}

	msgText := "Список делегатов:\n"
	for _, delegate := range delegates {
		voteStatus := "❌" // Default to cross (not voted)
		if delegate.HasVoted {
			voteStatus = "✅" // Change to checkmark if voted
		}
		registry := "❌"
		if delegate.TelegramID.Valid {
			registry = "✅"
		}
		telegramIDStr := toStrTelegramID(strconv.Itoa(int(delegate.TelegramID.Int64)))
		delegateIDStr := toStrDelegatID(strconv.Itoa(delegate.DelegateID))
		delegateInfo := fmt.Sprintf("• <a href=\"tg://user?id=%s\">st%s</a>, %s, Registry%s, Vote%s\n", telegramIDStr, delegateIDStr, delegate.Group, registry, voteStatus)
		// Check if adding the delegate info exceeds the limit
		if len(msgText)+len(delegateInfo) > 4096 {
			// Send the current message
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = "HTML"
			b.botAPI.Send(msg)

			// Reset the message text for the next message
			msgText = delegateInfo
		} else {
			// Append the delegate info to the current message
			msgText += delegateInfo
		}
	}
	// Send the final message (if any)
	if len(msgText) > 0 {
		msg := tgbotapi.NewMessage(chatID, msgText)
		msg.ParseMode = "HTML"
		b.botAPI.Send(msg)
	}

}

// Обработчик команды /show_candidates
func (b *Bot) handleShowCandidates(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	candidates, err := b.voteChain.GetAllCandidates(ctx)
	if err != nil {
		log.Errorf("%d Ошибка при получении списка кандидатов: %v", chatID, err)
		return
	}

	msgText := "Список кандидатов:\n"
	for _, candidate := range candidates {
		eligibleStatus := "❌" // Default to cross (not eligible)
		if candidate.IsEligible {
			eligibleStatus = "✅" // Change to checkmark if eligible
		}
		delegateIDStr := toStrDelegatID(strconv.Itoa(candidate.CandidateID))
		candidateInfo := fmt.Sprintf("• %s, st%s, %s, %s, Eligible %s\n", candidate.Name, delegateIDStr, candidate.Course, candidate.Description, eligibleStatus)
		// Check if adding the candidate info exceeds the limit
		if len(msgText)+len(candidateInfo) > 4096 {
			// Send the current message
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = "HTML"
			b.botAPI.Send(msg)

			// Reset the message text for the next message
			msgText = candidateInfo
		} else {
			// Append the candidate info to the current message
			msgText += candidateInfo
		}
	}
	// Send the final message (if any)
	if len(msgText) > 0 {
		msg := tgbotapi.NewMessage(chatID, msgText)
		msg.ParseMode = "HTML"
		b.botAPI.Send(msg)
	}

}

// Обработчик команды /show_votes
func (b *Bot) handleShowVotes(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	votes, err := b.voteChain.GetAllVotes(ctx)
	if err != nil {
		log.Errorf("%d Ошибка при получении списка голосов: %v", chatID, err)
		return
	}

	msgText := "Список голосов:\n"

	// msg := tgbotapi.NewMessage(chatID, "Список голосов:\n")
	for _, vote := range votes {
		delegate, err := b.voteChain.GetDelegateByDelegateID(ctx, vote.DelegateID)
		if err != nil {
			log.Errorf("%d Ошибка при получении делегата: %v", chatID, err)
			return
		}
		delegateIDStr := toStrDelegatID(strconv.Itoa(delegate.DelegateID))
		voteInfo := fmt.Sprintf("• st%s, %s: %s\n", delegateIDStr, vote.CreatedAt.Format("15:04:05"), fmt.Sprint(vote.CandidateRankings))

		// Check if adding the vote info exceeds the limit
		if len(msgText)+len(voteInfo) > 4096 {
			// Send the current message
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = "HTML"
			b.botAPI.Send(msg)

			// Reset the message text for the next message
			msgText = voteInfo
		} else {
			// Append the vote info to the current message
			msgText += voteInfo
		}
	}
	// Send the final message (if any)
	if len(msgText) > 0 {
		msg := tgbotapi.NewMessage(chatID, msgText)
		msg.ParseMode = "HTML"
		b.botAPI.Send(msg)
	}

}

// Обработчик команды /start_voting
func (b *Bot) handleStartVoting(_ context.Context, message *tgbotapi.Message) {
	// Обновляем список кандидатов
	if err := b.SetCandidates(); err != nil {
		log.Errorf("%d Ошибка при обновлении списка кандидатов: %v", message.From.ID, err)
		return
	}
	activeVoting = true
	log.Warn(message.From.ID, " Голосование открыто!")
}

// Обработчик команды /stop_voting
func (b *Bot) handleStopVoting(_ context.Context, message *tgbotapi.Message) {
	activeVoting = false
	log.Warn(message.From.ID, " Голосование закрыто!")
}

// Преобразование ID делегата в строку с ведущими нулями
func toStrDelegatID(number string) string {
	re := regexp.MustCompile(`^\d{6}$`)
	if re.MatchString(number) {
		return number
	}
	// Add leading zeros
	for len(number) < 6 {
		number = "0" + number
	}
	return number
}

// Преобразование ID телеграмма в строку с ведущими нулями
func toStrTelegramID(number string) string {
	re := regexp.MustCompile(`^\d{9}$`)
	if re.MatchString(number) {
		return number
	}
	// Add leading zeros
	for len(number) < 9 {
		number = "0" + number
	}
	return number

}

// Проверка формата ID (шестизначное число)
func isValidID(number string) bool {
	re := regexp.MustCompile(`^\d{6}$`)
	return re.MatchString(strings.TrimSpace(number))
}

// Проверка формата курса (1-4 бакалавриат или 1-2 магистратура)
func isValidCourse(course string) bool {
	// курс должен быть 1-4 бакалавариат или 1-2 магистратура
	re := regexp.MustCompile(`^((1|2|3|4) бакалавриат|(1|2) магистратура)$`)
	return re.MatchString(course)
}

// Проверка формата группы (XX.(б|м)XX-пу)
func isValidGroup(group string) bool {
	// курс должен быть XX.(б|м)XX-пу
	re := regexp.MustCompile(`^\d{2}\.(Б|М)\d{2}-пу$`)
	return re.MatchString(group)
}

// Обработчик команды /results
func (b *Bot) handleResults(ctx context.Context, message *tgbotapi.Message) {
	if err := b.schulze.SetCandidates(); err != nil {
		log.Errorf("%d %v", message.Chat.ID, err)
	}
	if err := b.schulze.SetVotes(); err != nil {
		log.Errorf("%d %v", message.Chat.ID, err)
	}
	if err := b.schulze.SetCandidatesByCourse(); err != nil {
		log.Errorf("%d %v", message.Chat.ID, err)
	}
	if err := b.schulze.SetVotesByCourse(); err != nil {
		log.Errorf("%d %v", message.Chat.ID, err)
	}
	if err := b.schulze.ComputeResults(ctx); err != nil {
		log.Errorf("%d %v", message.Chat.ID, err)
	}
	if err := b.schulze.ComputeGlobalTop(ctx); err != nil {
		log.Errorf("%d %v", message.Chat.ID, err)
	}
	log.Info(message.Chat.ID, " Результаты успешно вычислены")
	// if err := b.schulze.SaveResultsToCSV(ctx); err != nil {
	// 	log.Errorf("%d %v", message.Chat.ID, err)
	// }
	b.handleCSV(ctx, message)
}

// Функция для разбивки сообщения на части по заданному размеру
func splitMessage(message string, maxSize int) []string {
	var msgParts []string
	for len(message) > maxSize {
		msgParts = append(msgParts, message[:maxSize])
		message = message[maxSize:]
	}
	if len(message) > 0 {
		msgParts = append(msgParts, message)
	}
	return msgParts
}

// Обработчик команды /print
func (b *Bot) handlePrint(_ context.Context, message *tgbotapi.Message) {
	resultsString, err := b.schulze.GetResultsString()
	if err != nil {
		log.Errorf("%d %v", message.Chat.ID, err)
		return
	}
	// Разбиваем результаты на сообщения по 4096 символов
	msgParts := splitMessage(resultsString, 4096)

	// Отправляем сообщения по очереди
	for _, msgPart := range msgParts {
		msg := tgbotapi.NewMessage(message.Chat.ID, msgPart)
		msg.ParseMode = "HTML"
		b.botAPI.Send(msg)
	}
}

// Обработчик команды /csv
func (b *Bot) handleCSV(ctx context.Context, message *tgbotapi.Message) {
	if err := b.schulze.SaveResultsToCSV(ctx); err != nil {
		log.Errorf("%d Ошибка при записи в CSV: %v", message.Chat.ID, err)
		return
	}
	log.Info(message.Chat.ID, " Результаты успешно записаны в CSV файл")

	// Открываем файл для чтения
	filePath := filepath.Join("logs", "results.csv")
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		log.Errorf("%d Ошибка при открытии файла: %v", message.Chat.ID, err)
		return
	}
	defer file.Close()

	// Read the file contents into a byte slice
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Errorf("%d Ошибка при чтении файла: %v", message.Chat.ID, err)
		return
	}

	// Создаем новое сообщение с документом
	msg := tgbotapi.NewDocument(message.Chat.ID, tgbotapi.FileBytes{
		Name:  "results.csv",
		Bytes: fileBytes,
	})

	// Отправляем сообщение
	_, err = b.botAPI.Send(msg)
	if err != nil {
		log.Errorf("%d Ошибка при отправке файла: %v", message.Chat.ID, err)
		return
	}
}

func (b *Bot) handleSendLogs(_ context.Context, message *tgbotapi.Message) {
	// Открываем файл для чтения
	filePath := filepath.Join("logs", "bot.log")
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		log.Errorf("%d Ошибка при открытии файла: %v", message.Chat.ID, err)
		return
	}
	defer file.Close()

	// Read the file contents into a byte slice
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Errorf("%d Ошибка при чтении файла: %v", message.Chat.ID, err)
		return
	}

	// Создаем новое сообщение с документом
	msg := tgbotapi.NewDocument(message.Chat.ID, tgbotapi.FileBytes{
		Name:  "bot.log",
		Bytes: fileBytes,
	})

	// Отправляем сообщение
	_, err = b.botAPI.Send(msg)
	if err != nil {
		log.Errorf("%d Ошибка при отправке файла логов: %v", message.Chat.ID, err)
	}
}
