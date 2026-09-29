package bot

// Тексты сообщений бота, видимые пользователям (делегатам и админам).
//
// Соглашение: все пользовательские тексты пакета bot объявлены здесь как константы msg<Область><Что>.
// Константы с суффиксом Fmt содержат плейсхолдеры и подставляются через fmt.Sprintf (порядок аргументов
// в комментарии). Перед каждой константой — где она используется (обработчик / ситуация).
// Сообщения отправляются через b.SendMessage с ParseMode HTML: символы < > & в тексте должны быть либо
// экранированы (&lt; &gt; &amp;), либо быть намеренной HTML-разметкой (<b>, <code>, <a href>).
// Сообщения, отправляемые напрямую через tgbotapi.NewMessage без ParseMode (handleHelp, handleHelpAdmin,
// «Неизвестная команда», подсказка default), а также все тексты бюллетеня и кнопок (msgVoteBallot*,
// msgVoteFinalBallotHeader, msgVoteSpoiledFmt, msgVoteCallbackAccepted, msgVoteButtonFmt) идут как обычный
// текст: HTML-разметка в них не работает.
// Тексты логов (log.*) сюда не входят — они не показываются пользователям.

// Регистрация: /start, ввод почты, коды подтверждения, лимиты писем
const (
	// handleStart: пользователь уже зарегистрирован
	msgRegAlreadyRegistered = "Вы уже зарегистрированы! Используйте команду /vote для голосования"
	// handleStart: приветствие и запрос st-почты
	msgRegWelcome = "Добро пожаловать в бот для голосования на выборах в Студенческий совет ПМ-ПУ!\n\n" +
		"Чтобы начать голосование, необходимо пройти регистрацию.\n" +
		"<b>Пожалуйста, введите свою st почту (в формате: stXXXXXX)</b>"
	// handleEmailInput: текст не похож на stXXXXXX
	msgRegInvalidEmail = "Неверный формат почты. Пожалуйста, введите почту в формате stXXXXXX."
	// handleEmailInput: из почты не удалось получить число (после проверки формата практически недостижимо)
	msgRegEmailParseError = "Невозможно получить ID делегата из почты. Пожалуйста, попробуйте снова"
	// handleEmailInput: делегата с таким st-номером нет в списке
	msgRegDelegateNotFound = "Такой делегат не найден. Убедитесь, что вы ввели правильную почту."
	// handleEmailInput: делегат уже привязан к Telegram-аккаунту
	msgRegDelegateTaken = "Делегат с такой почтой уже зарегистрировался"
	// handleEmailInput: кулдаун, код на эту почту уже ждёт ввода; %06d — delegateID, %d — секунды
	msgRegCodeAlreadySentFmt = "Код уже отправлен на st%06d@student.spbu.ru, введите его. Повторное письмо возможно через %d сек."
	// handleEmailInput: кулдаун, у аккаунта ждёт ввода код другого делегата; %06d — ожидающий delegateID, %06d — запрошенный delegateID, %d — секунды
	msgRegOtherCodePendingFmt = "Сейчас ожидается код, отправленный на st%06d@student.spbu.ru. Код для st%06d можно запросить через %d сек."
	// handleEmailInput: кулдаун без ожидающего кода; %d — секунды
	msgRegCooldownFmt = "Повторный запрос кода возможен через %d сек."
	// handleEmailInput: исчерпан суточный лимит писем (на аккаунт, на делегата или общий); код сохранён для /show_code
	msgRegEmailLimit = "Лимит писем с кодом на сегодня исчерпан. Обратитесь к организаторам: они выдадут код вручную, затем введите его здесь"
	// handleEmailInput: SMTP/API отказал; код сохранён для /show_code
	msgRegEmailSendFailed = "Не удалось отправить письмо с кодом. Обратитесь к организаторам: они выдадут код вручную, затем введите его здесь. Либо начните заново: /start"
	// handleEmailInput: письмо ушло, ждём код
	msgRegCodeSent = "Код подтверждения отправлен на ваш email. Пожалуйста, введите код.\nЕсли Вы не видите письмо - проверьте Спам или обратитесь к организаторам"
	// handleCodeInput: введено не число и не почта
	msgRegInvalidCode = "Неверный формат кода. Введите числовой код из письма или почту stXXXXXX, чтобы запросить письмо заново."
	// handleCodeInput: для аккаунта нет ожидающего кода (бот перезапускался или код удалён)
	msgRegCodeNotFound = "Не найден код для подтверждения. Попробуйте начать регистрацию заново."
	// handleCodeInput: код не совпал
	msgRegWrongCode = "Неверный код. Попробуйте еще раз."
	// handleCodeInput: maxCodeAttempts неверных кодов подряд — код аннулирован
	msgRegTooManyAttempts = "Слишком много неверных попыток, код аннулирован. Начните заново: /start"
	// handleCodeInput: делегат успел зарегистрироваться с другого аккаунта (chain.ErrAlreadyVerified)
	msgRegTakenByOtherAccount = "Этот делегат уже зарегистрирован с другого аккаунта. Если это ошибка, обратитесь к организаторам"
	// handleCodeInput: регистрация завершена
	msgRegDone = "Регистрация успешно завершена! Используйте команду /vote для голосования"
)

// Голосование: /vote, памятка, бюллетень, принятие голоса
const (
	// handleVote, handleCallbackQuery: голосование не открыто (/start_voting не выполнялся или уже /stop_voting)
	msgVoteClosed = "Голосование уже завершилось или еще не началось"
	// handleVote: аккаунт не привязан к делегату
	msgVoteNotRegistered = "Вы не зарегистрированы! Используйте команду /start для регистрации"
	// handleVote: памятка о методе Шульце перед бюллетенем
	msgVoteHelp = "Принцип голосования по методу Шульце заключается в формировании ранжированного списка кандидатов, " +
		"в котором <b>каждый кандидат должен быть ранжирован</b> по отношению к другим.\n" +
		"Например, если вы считаете, что кандидат А лучше кандидата Б, то вы должны поставить кандидата А выше в списке.\n\n" +
		"В контексте использования данного бота Вы должны последовательно выбрать кандидатов, от наиболее предпочитаемого к наименее предпочитаемому.\n" +
		"Для этого используйте кнопки меню в сообщении-бюллетени, последовательно выбирая нужного кандидата.\n\n" +
		"Важно:\n" +
		"• Вы должны ранжировать <b>всех кандидатов</b>.\n" +
		"• Удостоверьтесь, что Ваш бюллетень принят, <b>получив соответствующее сообщение</b>.\n" +
		"• Вы сможете изменить свой бюллетень ранжирования в любое время до окончания голосования.\n" +
		"• Не выбирайте следующего кандидата, пока не увидите изменение в теле сообщения-бюллетеня.\n"
	// SetCandidates: заголовок списка кандидатов, который handleVote шлёт перед бюллетенем
	msgVoteCandidatesHeader = "Список кандидатов:\n\n"
	// SetCandidates: строка списка кандидатов; %s — имя, %s — курс
	msgVoteCandidateLineFmt = "• %s, %s\n"
	// sendCandidateKeyboard: надпись кнопки бюллетеня; %s — имя, %s — курс
	msgVoteButtonFmt = "%s, %s"
	// sendCandidateKeyboard: заголовок бюллетеня (новое сообщение и редактирование)
	msgVoteBallotHeader = "Выберите всех кандидатов от наиболее к наименее предпочтительному:\n\n"
	// sendCandidateKeyboard, sendRankedList: строка уже выбранного кандидата; %d — позиция, %s — имя
	msgVoteBallotLineFmt = "%d. %s\n"
	// handleCallbackQuery: всплывающий ответ на нажатие кнопки
	msgVoteCallbackAccepted = "Кандидат учтен"
	// sendRankedList: заголовок заполненного бюллетеня (клавиатура убирается)
	msgVoteFinalBallotHeader = "Ваш итоговый бюллетень:\n\n"
	// sendRankedList: голос записан; %s — токен для поиска бюллетеня на веб-странице
	msgVoteAcceptedFmt = "<b>Ваш бюллетень принят✅</b>\n\n" +
		"Вы можете изменить свой бюллетень до окончания голосования, проголосовав заново, отправив для этого команду /vote\n\n" +
		"🔑 <code>%s</code>"
	// sendRankedList: ссылка на страницу бюллетеней, добавляется к msgVoteAcceptedFmt, если задан DOMAIN (%s — домен)
	msgVoteBallotPageFmt = "\n\nНайти свой бюллетень по токену: https://%s/votes/"

	// spoilBallot: пометка испорченного бюллетеня; %s — прежний текст сообщения-бюллетеня
	msgVoteSpoiledFmt = "%s\n\n❌Бюллетень испорчен❌"
	// spoilBallot: пояснение после порчи бюллетеня (нажатие в старом/заполненном бюллетене)
	msgVoteSpoiledHint = "Пожалуйста, не используйте несколько бюллетеней одновременно. Используйте команду /vote для получения нового бюллетеня."
)

// Команды делегата: /help, неизвестная команда, текст вне регистрации
const (
	// handleHelp: /help в личке
	msgCmdHelp = "Список доступных команд:\n" +
		"/start - начать регистрацию\n" +
		"/vote - начать голосование\n" +
		"/help - показать список доступных команд"
	// handleCommand: неизвестная команда в личке
	msgCmdUnknown = "Неизвестная команда"
	// handleText: текст в личке без активного состояния регистрации
	msgCmdDefaultHint = "Используйте /start для регистрации и /vote для голосования"
)

// Админ-команды (только в чате ADMIN_CHAT_ID)
const (
	// handleHelpAdmin: /help в админ-чате
	msgAdminHelp = "Список доступных команд:\n" +
		"/add_delegate <delegate_id> <name> <group> - добавить делегата\n" +
		"/delete_delegate <delegate_id> - удалить делегата\n" +
		"/add_candidate <candidate_id> <name> <course> <description> - добавить кандидата\n" +
		"/ban_candidate <candidate_id> - заблокировать кандидата\n" +
		"/delete_candidate <candidate_id> - удалить кандидата\n" +
		"/show_delegates - показать список делегатов\n" +
		"/show_candidates - показать список кандидатов\n" +
		"/show_votes - показать список голосов\n" +
		"/show_code <delegate_id> - показать код подтверждения делегата, если письмо не дошло\n" +
		"/start_voting - начать голосование\n" +
		"/stop_voting - остановить голосование\n" +
		"/results - вычислить результаты голосования\n" +
		"/print - вывести результаты голосования\n" +
		"/csv - сохранить результаты в CSV файл\n" +
		"/log <level> - установить уровень логирования (Debug, Info, Warn, Error)\n" +
		"/send_logs - отправить файл логов\n" +
		"/help - показать список доступных команд\n" +
		"\nВсегда используйте запятые между аргументами команды, если идет перечисление аргументов"
	// handleCommand: неизвестная команда в админ-чате
	msgAdminUnknownCommand = "Неизвестная Администрирующая команда"
	// handleShowDelegates, handleShowCandidates: отметки «да»/«нет» (зарегистрирован, проголосовал, допущен)
	msgAdminMarkYes = "✅"
	msgAdminMarkNo  = "❌"
	// handleShowDelegates: заголовок списка
	msgAdminDelegatesHeader = "Список делегатов:\n"
	// handleShowDelegates: строка делегата; %s — telegramID, %s — delegateID, %s — группа, %s — отметка регистрации, %s — отметка голоса
	msgAdminDelegateLineFmt = "• <a href=\"tg://user?id=%s\">st%s</a>, %s, Registry%s, Vote%s\n"
	// handleShowCandidates: заголовок списка
	msgAdminCandidatesHeader = "Список кандидатов:\n"
	// handleShowCandidates: строка кандидата; %s — имя, %s — candidateID, %s — курс, %s — описание, %s — отметка допуска
	msgAdminCandidateLineFmt = "• %s, st%s, %s, %s, Eligible %s\n"
	// handleShowVotes: заголовок списка
	msgAdminVotesHeader = "Список голосов:\n"
	// handleShowVotes: строка голоса; %s — delegateID, %s — время, %s — ранжирование
	msgAdminVoteLineFmt = "• st%s, %s: %s\n"
	// handleShowCode: аргумент отсутствует или не число
	msgAdminShowCodeUsage = "Использование: /show_code &lt;delegate_id&gt;, например /show_code 123456"
	// handleShowCode: ошибка БД при проверке существования делегата
	msgAdminShowCodeCheckError = "Ошибка проверки делегата, попробуйте снова"
	// handleShowCode: делегата нет в списке; %06d — delegateID
	msgAdminShowCodeNotFoundFmt = "Делегат st%06d не найден"
	// handleShowCode: ошибка БД при проверке регистрации делегата
	msgAdminShowCodeRegCheckError = "Ошибка проверки регистрации делегата, попробуйте снова"
	// handleShowCode: делегат уже зарегистрирован, код не выдаётся; %06d — delegateID
	msgAdminShowCodeRegisteredFmt = "st%06d уже зарегистрирован, код ему не нужен (ожидающие коды удалены)"
	// handleShowCode: нет ожидающего кода; %06d — delegateID
	msgAdminShowCodeNonePendingFmt = "Для st%06d нет ожидающего кода: делегат ещё не вводил почту, уже зарегистрирован или бот перезапускался"
	// handleShowCode: заголовок списка кодов; %06d — delegateID
	msgAdminShowCodeHeaderFmt = "Коды подтверждения для st%06d (выдавать только владельцу аккаунта):\n"
	// handleShowCode: строка кода; %d — telegramID (ссылка), %d — telegramID (текст), %d — код
	msgAdminShowCodeLineFmt = "• <a href=\"tg://user?id=%d\">%d</a>: <code>%d</code>\n"
)

// Системные: ошибки БД и обработки, «попробуйте снова»
const (
	// handleEmailInput: не удалось сгенерировать код (crypto/rand)
	msgSysGenericError = "Произошла ошибка. Пожалуйста, попробуйте снова"
	// handleStart, handleEmailInput: ошибка БД при проверке делегата
	msgSysDelegateCheckError = "Произошла ошибка при проверке делегата. Пожалуйста, попробуйте снова"
	// handleVote: ошибка БД при проверке регистрации
	msgSysRegistrationCheckError = "Произошла ошибка при проверке регистрации делегата. Пожалуйста, попробуйте снова"
	// handleCodeInput: ошибка БД при привязке аккаунта к делегату
	msgSysVerifyError = "Произошла ошибка при верификации. Пожалуйста, попробуйте снова"
	// handleCallbackQuery: данные кнопки не число
	msgSysCallbackError = "Произошла ошибка при обработке кнопки. Пожалуйста, попробуйте снова"
	// sendRankedList: ошибка БД при записи голоса
	msgSysVoteSaveError = "Произошла ошибка при регистрации голоса. Пожалуйста, попробуйте снова"
)
