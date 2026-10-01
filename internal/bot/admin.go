package bot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/chain"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"
	schulzepkg "github.com/lsdpls/schulze_election_telegram_bot/internal/schulze"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Обработчик команды /help
func (b *Bot) handleHelpAdmin(_ context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	msg := tgbotapi.NewMessage(chatID, msgAdminHelp)
	if _, err := b.botAPI.Send(msg); err != nil {
		log.Errorf("%d Ошибка отправки списка: %v", chatID, err)
	}
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

	// Удаляем делегата из базы данных; проголосовавшего chain не удаляет (ErrDelegateHasVoted)
	if err := b.voteChain.DeleteDelegate(ctx, delegateID); err != nil {
		if errors.Is(err, chain.ErrDelegateHasVoted) {
			log.Warnf("%d Отказ в удалении проголосовавшего делегата st%06d", chatID, delegateID)
			if err := b.SendMessage(chatID, fmt.Sprintf(msgAdminDelegateHasVotedFmt, delegateID)); err != nil {
				log.Errorf("%d Ошибка ответа об отказе в удалении делегата: %v", chatID, err)
			}
			return
		}
		log.Errorf("%d Ошибка при удалении делегата: %v", chatID, err)
		return
	}
	log.Info(chatID, " Делегат успешно удален")
}

// refuseIfVotingActive: пока голосование открыто, состав кандидатов менять нельзя (добавлять, снимать, удалять).
// Возвращает true и отвечает в админ-чат, если голосование открыто. Вызывать под b.votingMu
func (b *Bot) refuseIfVotingActive(message *tgbotapi.Message) bool {
	b.mu.RLock()
	active := b.activeVoting
	b.mu.RUnlock()
	if !active {
		return false
	}
	log.Warnf("%d Попытка изменить состав кандидатов при открытом голосовании: /%s", message.Chat.ID, message.Command())
	if err := b.SendMessage(message.Chat.ID, msgAdminCandidatesFrozen); err != nil {
		log.Errorf("%d Ошибка ответа о запрете изменения кандидатов: %v", message.Chat.ID, err)
	}
	return true
}

// Обработчик команды /add_candidate
func (b *Bot) handleAddCandidate(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	b.votingMu.Lock()
	defer b.votingMu.Unlock()
	if b.refuseIfVotingActive(message) {
		return
	}
	// Голосование уже начиналось (есть бюллетени, в том числе после /stop_voting или перезапуска бота):
	// нового кандидата нет ни в одном поданном бюллетене, и подсчёт дал бы неразрешимую ничью
	votes, err := b.voteChain.GetAllVotes(ctx)
	if err != nil {
		log.Errorf("%d Ошибка проверки бюллетеней перед добавлением кандидата: %v", chatID, err)
		return
	}
	if len(votes) > 0 {
		log.Warnf("%d Попытка добавить кандидата, когда уже есть бюллетени (%d)", chatID, len(votes))
		if err := b.SendMessage(chatID, fmt.Sprintf(msgAdminCandidateAddAfterVotesFmt, len(votes))); err != nil {
			log.Errorf("%d Ошибка ответа о запрете добавления кандидата: %v", chatID, err)
		}
		return
	}
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

// Обработчик команды /ban_candidate: снять кандидата с выборов (is_eligible = false), только при закрытом голосовании
func (b *Bot) handleBanCandidate(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	b.votingMu.Lock()
	defer b.votingMu.Unlock()
	if b.refuseIfVotingActive(message) {
		return
	}
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

// Обработчик команды /delete_candidate: кандидата из БД не удаляем никогда — снимаем мягко (is_eligible = false),
// как /ban_candidate. Бюллетени с его ID остаются, при подсчёте он не учитывается. Только при закрытом голосовании
func (b *Bot) handleDeleteCandidate(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	b.votingMu.Lock()
	defer b.votingMu.Unlock()
	if b.refuseIfVotingActive(message) {
		return
	}
	candidateMsg := message.CommandArguments()

	// Извлекаем ID кандидата из сообщения
	candidateID, err := strconv.Atoi(candidateMsg)
	if err != nil || !isValidID(candidateMsg) {
		log.Warn(chatID, " Неверный формат candidate_id. Используйте целое шестизначное число.")
		return
	}

	// Снимаем кандидата (мягкое удаление)
	if err := b.voteChain.BanCandidate(ctx, candidateID); err != nil {
		log.Errorf("%d Ошибка при удалении кандидата: %v", chatID, err)
		return
	}
	log.Info(chatID, " Кандидат успешно удален (снят с выборов, запись в базе сохранена)")
}

// Обработчик команды /show_delegates
func (b *Bot) handleShowDelegates(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	delegates, err := b.voteChain.GetAllDelegates(ctx)
	if err != nil {
		log.Errorf("%d Ошибка при получении списка делегатов: %v", chatID, err)
		return
	}

	msgText := msgAdminDelegatesHeader
	for _, delegate := range delegates {
		voteStatus := msgAdminMarkNo // Default to cross (not voted)
		if delegate.HasVoted {
			voteStatus = msgAdminMarkYes // Change to checkmark if voted
		}
		registry := msgAdminMarkNo
		if delegate.TelegramID.Valid {
			registry = msgAdminMarkYes
		}
		telegramIDStr := toStrTelegramID(strconv.Itoa(int(delegate.TelegramID.Int64)))
		delegateIDStr := toStrDelegatID(strconv.Itoa(delegate.DelegateID))
		delegateInfo := fmt.Sprintf(msgAdminDelegateLineFmt, telegramIDStr, delegateIDStr, html.EscapeString(delegate.Group), registry, voteStatus)
		// Check if adding the delegate info exceeds the limit
		if len(msgText)+len(delegateInfo) > 4096 {
			// Send the current message
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = "HTML"
			if _, err := b.botAPI.Send(msg); err != nil {
				log.Errorf("%d Ошибка отправки списка: %v", chatID, err)
			}

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
		if _, err := b.botAPI.Send(msg); err != nil {
			log.Errorf("%d Ошибка отправки списка: %v", chatID, err)
		}
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

	msgText := msgAdminCandidatesHeader
	for _, candidate := range candidates {
		eligibleStatus := msgAdminMarkNo // Default to cross (not eligible)
		if candidate.IsEligible {
			eligibleStatus = msgAdminMarkYes // Change to checkmark if eligible
		}
		delegateIDStr := toStrDelegatID(strconv.Itoa(candidate.CandidateID))
		candidateInfo := fmt.Sprintf(msgAdminCandidateLineFmt, html.EscapeString(candidate.Name), delegateIDStr, html.EscapeString(candidate.Course), html.EscapeString(candidate.Description), eligibleStatus)
		// Check if adding the candidate info exceeds the limit
		if len(msgText)+len(candidateInfo) > 4096 {
			// Send the current message
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = "HTML"
			if _, err := b.botAPI.Send(msg); err != nil {
				log.Errorf("%d Ошибка отправки списка: %v", chatID, err)
			}

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
		if _, err := b.botAPI.Send(msg); err != nil {
			log.Errorf("%d Ошибка отправки списка: %v", chatID, err)
		}
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

	msgText := msgAdminVotesHeader

	for _, vote := range votes {
		// delegate_id есть в самом голосе; лукап делегата давал nil-deref при гонке с /delete_delegate
		delegateIDStr := toStrDelegatID(strconv.Itoa(vote.DelegateID))
		voteInfo := fmt.Sprintf(msgAdminVoteLineFmt, delegateIDStr, vote.CreatedAt.Format("15:04:05"), fmt.Sprint(vote.CandidateRankings))

		// Check if adding the vote info exceeds the limit
		if len(msgText)+len(voteInfo) > 4096 {
			// Send the current message
			msg := tgbotapi.NewMessage(chatID, msgText)
			msg.ParseMode = "HTML"
			if _, err := b.botAPI.Send(msg); err != nil {
				log.Errorf("%d Ошибка отправки списка: %v", chatID, err)
			}

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
		if _, err := b.botAPI.Send(msg); err != nil {
			log.Errorf("%d Ошибка отправки списка: %v", chatID, err)
		}
	}

}

// Обработчик команды /start_voting
func (b *Bot) handleStartVoting(_ context.Context, message *tgbotapi.Message) {
	b.votingMu.Lock()
	defer b.votingMu.Unlock()
	// Обновляем список кандидатов
	if err := b.SetCandidates(); err != nil {
		log.Errorf("%d Ошибка при обновлении списка кандидатов: %v", message.From.ID, err)
		return
	}
	b.mu.Lock()
	b.activeVoting = true
	b.mu.Unlock()
	noteVotingState(true)
	log.Warn(message.From.ID, " Голосование открыто!")
}

// Обработчик команды /stop_voting
func (b *Bot) handleStopVoting(_ context.Context, message *tgbotapi.Message) {
	b.votingMu.Lock()
	defer b.votingMu.Unlock()
	b.mu.Lock()
	b.activeVoting = false
	b.mu.Unlock()
	noteVotingState(false)
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

// Deprecated: Проверка формата группы (XX.(б|м)XX-пу)
func isValidGroup(group string) bool {
	// курс должен быть XX.(б|м)XX-пу
	re := regexp.MustCompile(`^\d{2}\.(Б|М)\d{2}-пу$`)
	return re.MatchString(group)
}

// Обработчик команды /results. «Результаты успешно вычислены» — только если посчитано всё. Об ошибке и о ничьей,
// которую тай-брейк не разрешил, бот пишет в админ-чат (и в лог). CSV отправляется при успехе и при неразрешённой
// ничьей (матрицы курсов нужны для ручного подсчёта), при прочих ошибках — нет
func (b *Bot) handleResults(ctx context.Context, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	// Загрузка данных: если она не удалась, считать нечего
	for _, load := range []func() error{
		b.schulze.SetCandidates,
		b.schulze.SetVotes,
		b.schulze.SetCandidatesByCourse,
		b.schulze.SetVotesByCourse,
	} {
		if err := load(); err != nil {
			b.reportResultsErrors(chatID, err)
			return
		}
	}
	// Общие места считаются только после победителей всех курсов: от них зависят число мест и состав пула
	if err := b.schulze.ComputeResults(ctx); err != nil {
		if onlyTies := b.reportResultsErrors(chatID, err); onlyTies {
			b.handleCSV(ctx, message)
		}
		return
	}
	if err := b.schulze.ComputeGlobalTop(ctx); err != nil {
		if onlyTies := b.reportResultsErrors(chatID, err); onlyTies {
			b.handleCSV(ctx, message)
		}
		return
	}
	log.Info(message.Chat.ID, " Результаты успешно вычислены")
	// if err := b.schulze.SaveResultsToCSV(ctx); err != nil {
	// 	log.Errorf("%d %v", message.Chat.ID, err)
	// }
	b.handleCSV(ctx, message)
}

// reportResultsErrors пишет каждую ошибку подсчёта в лог и отдельным сообщением в админ-чат.
// Возвращает true, если все ошибки — неразрешённые ничьи (*schulze.TieError)
func (b *Bot) reportResultsErrors(chatID int64, err error) (onlyTies bool) {
	onlyTies = true
	for _, e := range flattenErrors(err) {
		var tie *schulzepkg.TieError
		var text string
		switch {
		case errors.As(e, &tie):
			log.Warnf("%d %v", chatID, e)
			text = formatTieMessage(tie)
		case errors.Is(e, schulzepkg.ErrNoVotes):
			onlyTies = false
			log.Errorf("%d %v", chatID, e)
			text = msgAdminResultsNoVotes
		default:
			onlyTies = false
			log.Errorf("%d %v", chatID, e)
			text = fmt.Sprintf(msgAdminResultsErrorFmt, html.EscapeString(e.Error()))
		}
		if err := b.SendMessage(chatID, text); err != nil {
			log.Errorf("%d Ошибка отправки сообщения о результатах подсчёта: %v", chatID, err)
		}
	}
	return onlyTies
}

// flattenErrors раскрывает errors.Join (в том числе вложенные) в плоский список
func flattenErrors(err error) []error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, e := range joined.Unwrap() {
			out = append(out, flattenErrors(e)...)
		}
		return out
	}
	return []error{err}
}

// formatTieMessage — текст уведомления о неразрешённой ничьей для админ-чата
func formatTieMessage(tie *schulzepkg.TieError) string {
	details := formatCandidates(tie.Unresolved)
	if len(tie.Tied) > len(tie.Unresolved) {
		details = fmt.Sprintf(msgAdminResultsTieNarrowedFmt, formatCandidates(tie.Tied), formatCandidates(tie.Unresolved))
	}
	if tie.Course != "" {
		return fmt.Sprintf(msgAdminResultsTieCourseFmt, html.EscapeString(tie.Course), details)
	}
	decided := msgAdminResultsNobody
	if len(tie.Decided) > 0 {
		decided = formatCandidates(tie.Decided)
	}
	return fmt.Sprintf(msgAdminResultsTieCommonFmt, tie.Place, tie.Places, details, decided, tie.Places-tie.Place+1)
}

// formatCandidates: «st123456 Иванов И.И., st654321 Петров П.П.» (имена экранированы для HTML)
func formatCandidates(candidates []models.Candidate) string {
	parts := make([]string, len(candidates))
	for i, c := range candidates {
		parts[i] = fmt.Sprintf("st%06d %s", c.CandidateID, html.EscapeString(c.Name))
	}
	return strings.Join(parts, ", ")
}

// Строка, которой GetResultsString завершает блок каждого курса
const printBlockSeparator = "—————\n"

// packBlocks собирает блоки в сообщения не длиннее maxSize байт, не разрывая блок; блок длиннее maxSize
// делится по строкам (splitMessage)
func packBlocks(blocks []string, maxSize int) []string {
	var msgParts []string
	var cur strings.Builder
	for _, block := range blocks {
		if block == "" {
			continue
		}
		if cur.Len()+len(block) > maxSize && cur.Len() > 0 {
			msgParts = append(msgParts, cur.String())
			cur.Reset()
		}
		if len(block) > maxSize {
			msgParts = append(msgParts, splitMessage(block, maxSize)...)
			continue
		}
		cur.WriteString(block)
	}
	if cur.Len() > 0 {
		msgParts = append(msgParts, cur.String())
	}
	return msgParts
}

// TODO: move to utils
// splitMessage делит текст на части не длиннее maxSize байт по границам строк: HTML-теги /print стоят
// внутри одной строки, поэтому каждая часть остаётся с закрытыми тегами и целыми буквами.
// Строку длиннее maxSize (в /print таких нет) режет по границе символа
func splitMessage(message string, maxSize int) []string {
	var msgParts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			msgParts = append(msgParts, cur.String())
			cur.Reset()
		}
	}
	for _, line := range strings.SplitAfter(message, "\n") {
		if line == "" {
			continue
		}
		if cur.Len()+len(line) > maxSize {
			flush()
		}
		for len(line) > maxSize {
			cut := maxSize
			for cut > 0 && !utf8.RuneStart(line[cut]) {
				cut--
			}
			msgParts = append(msgParts, line[:cut])
			line = line[cut:]
		}
		cur.WriteString(line)
	}
	flush()
	return msgParts
}

// Обработчик команды /print
func (b *Bot) handlePrint(_ context.Context, message *tgbotapi.Message) {
	resultsString, err := b.schulze.GetResultsString()
	if err != nil {
		log.Errorf("%d %v", message.Chat.ID, err)
		return
	}
	// Разбиваем результаты на сообщения до 4096 байт: целыми блоками курсов, а блок длиннее — по строкам
	msgParts := packBlocks(strings.SplitAfter(resultsString, printBlockSeparator), 4096)

	// Отправляем сообщения по очереди
	for _, msgPart := range msgParts {
		msg := tgbotapi.NewMessage(message.Chat.ID, msgPart)
		msg.ParseMode = "HTML"
		if _, err := b.botAPI.Send(msg); err != nil {
			log.Errorf("%d Ошибка отправки списка: %v", message.Chat.ID, err)
		}
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
