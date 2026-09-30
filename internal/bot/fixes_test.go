package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mocks "github.com/lsdpls/schulze_election_telegram_bot/internal/bot/mocks"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/chain"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"
	schulzepkg "github.com/lsdpls/schulze_election_telegram_bot/internal/schulze"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/golang/mock/gomock"
)

// tgCall — один запрос к поддельному Bot API
type tgCall struct {
	method    string
	chatID    int64
	text      string
	hasMarkup bool
}

// recTelegram — поддельный Bot API: sendMessage, editMessageText, answerCallbackQuery, sendDocument.
// delay задерживает ответы для отдельных чатов (answerCallbackQuery — по префиксу "<chat>:" в callback_query_id);
// inflight считает одновременные запросы по чату
type recTelegram struct {
	*httptest.Server
	mu          sync.Mutex
	calls       []tgCall
	delay       map[int64]time.Duration
	inflight    map[int64]int
	maxInflight map[int64]int
	onCall      func(tgCall)
}

func newRecTelegram(t *testing.T) (*recTelegram, *tgbotapi.BotAPI) {
	t.Helper()
	f := &recTelegram{delay: map[int64]time.Duration{}, inflight: map[int64]int{}, maxInflight: map[int64]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method := path.Base(r.URL.Path)
		chatID, _ := strconv.ParseInt(r.FormValue("chat_id"), 10, 64)
		if method == "answerCallbackQuery" {
			chatID, _ = strconv.ParseInt(strings.SplitN(r.FormValue("callback_query_id"), ":", 2)[0], 10, 64)
		}
		call := tgCall{method: method, chatID: chatID, text: r.FormValue("text"), hasMarkup: r.FormValue("reply_markup") != ""}
		f.mu.Lock()
		f.inflight[chatID]++
		if f.inflight[chatID] > f.maxInflight[chatID] {
			f.maxInflight[chatID] = f.inflight[chatID]
		}
		d := f.delay[chatID]
		onCall := f.onCall
		f.mu.Unlock()
		if onCall != nil {
			onCall(call)
		}
		time.Sleep(d)
		f.mu.Lock()
		f.inflight[chatID]--
		f.calls = append(f.calls, call)
		n := len(f.calls)
		f.mu.Unlock()

		var result any = map[string]any{"message_id": n, "date": 0, "chat": map[string]any{"id": chatID, "type": "private"}, "text": call.text}
		switch method {
		case "getMe":
			result = map[string]any{"id": 1, "is_bot": true, "first_name": "test", "username": "test_bot"}
		case "answerCallbackQuery":
			result = true
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(f.Close)
	api, err := tgbotapi.NewBotAPIWithClient("123:TEST", f.URL+"/bot%s/%s", f.Client())
	if err != nil {
		t.Fatalf("NewBotAPIWithClient: %v", err)
	}
	return f, api
}

func (f *recTelegram) callsFor(chatID int64) []tgCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []tgCall
	for _, c := range f.calls {
		if c.chatID == chatID {
			out = append(out, c)
		}
	}
	return out
}

func (f *recTelegram) texts(chatID int64) string {
	var sb strings.Builder
	for _, c := range f.callsFor(chatID) {
		sb.WriteString(c.method + ": " + c.text + "\n")
	}
	return sb.String()
}

// votingBot — бот с открытым голосованием и тремя допущенными кандидатами
func votingBot(t *testing.T) (*Bot, *recTelegram, *mocks.MockvoteChain) {
	t.Helper()
	tg, api := newRecTelegram(t)
	db := mocks.NewMockvoteChain(gomock.NewController(t))
	b := newTestBot()
	b.botAPI, b.voteChain = api, db
	b.Candidates = map[int]models.Candidate{
		900001: {CandidateID: 900001, Name: "Кандидат 1", Course: "1 бакалавриат", IsEligible: true},
		900002: {CandidateID: 900002, Name: "Кандидат 2", Course: "2 бакалавриат", IsEligible: true},
		900003: {CandidateID: 900003, Name: "Кандидат 3", Course: "3 бакалавриат", IsEligible: true},
	}
	b.sortedCandidatesIDs = []int{900001, 900002, 900003}
	b.rankedList = make(map[int64][]int)
	b.activeVoting = true
	return b, tg, db
}

var cbSeq int64

func press(user int64, candidateID int) tgbotapi.Update {
	n := atomic.AddInt64(&cbSeq, 1)
	return tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:      fmt.Sprintf("%d:%d", user, n),
		From:    &tgbotapi.User{ID: user},
		Message: &tgbotapi.Message{MessageID: 10, Text: msgVoteBallotHeader, Chat: &tgbotapi.Chat{ID: user, Type: "private"}},
		Data:    strconv.Itoa(candidateID),
	}}
}

// Медленный Telegram у одного делегата не задерживает нажатия других: общая блокировка больше не держится
// во время запросов к Telegram. Раньше B ждал, пока A получит ответ на answerCallbackQuery
func TestSlowTelegramForOneUserDoesNotBlockOthers(t *testing.T) {
	b, tg, _ := votingBot(t)
	const userA, userB = 1001, 1002
	tg.mu.Lock()
	tg.delay[userA] = 800 * time.Millisecond
	tg.mu.Unlock()

	done := make(chan struct{})
	go func() { b.HandleUpdate(context.Background(), press(userA, 900001)); close(done) }()
	time.Sleep(100 * time.Millisecond) // A уже ждёт ответа Telegram
	start := time.Now()
	b.HandleUpdate(context.Background(), press(userB, 900002))
	if el := time.Since(start); el > 400*time.Millisecond {
		t.Fatalf("нажатие B обработано за %v: ждало медленного A", el)
	}
	<-done
}

// Апдейты одного пользователя (например, повторная доставка) выполняются строго по очереди
func TestSameUserUpdatesAreSerialized(t *testing.T) {
	b, tg, _ := votingBot(t)
	const user = 1001
	tg.mu.Lock()
	tg.delay[user] = 150 * time.Millisecond
	tg.mu.Unlock()
	var wg sync.WaitGroup
	for _, c := range []int{900001, 900002} {
		wg.Add(1)
		go func(c int) { defer wg.Done(); b.HandleUpdate(context.Background(), press(user, c)) }(c)
	}
	wg.Wait()
	tg.mu.Lock()
	maxIn := tg.maxInflight[user]
	tg.mu.Unlock()
	if maxIn != 1 {
		t.Fatalf("одновременных запросов по одному пользователю: %d, ожидался 1", maxIn)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.rankedList[user]) != 2 {
		t.Fatalf("rankedList=%v", b.rankedList[user])
	}
}

// Голос сначала записывается, и только потом показывается «Ваш итоговый бюллетень»; запись идёт со своим
// контекстом, даже если контекст вебхука уже отменён
func TestFinalBallotShownOnlyAfterVoteSaved(t *testing.T) {
	b, tg, db := votingBot(t)
	const user = 1001
	b.rankedList[user] = []int{900002, 900001}
	var saved atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tg.onCall = func(c tgCall) {
		if c.method == "answerCallbackQuery" {
			cancel() // соединение вебхука оборвалось посреди обработки
		}
		if strings.HasPrefix(c.text, msgVoteFinalBallotHeader) && !saved.Load() {
			t.Errorf("«итоговый бюллетень» показан до записи голоса")
		}
	}
	db.EXPECT().AddVote(gomock.Any(), int64(user), []int{900002, 900001, 900003}).DoAndReturn(func(ctx context.Context, _ int64, _ []int) error {
		if ctx.Err() != nil {
			t.Errorf("контекст записи голоса уже отменён: %v", ctx.Err())
		}
		saved.Store(true)
		return nil
	})
	b.HandleUpdate(ctx, press(user, 900003))
	out := tg.texts(user)
	if !strings.Contains(out, msgVoteFinalBallotHeader) || !strings.Contains(out, "Ваш бюллетень принят") {
		t.Fatalf("нет итогового бюллетеня или подтверждения:\n%s", out)
	}
}

// Запись голоса не удалась: «итоговый бюллетень» не показывается, клавиатура снимается, приходит ошибка
func TestVoteSaveFailureDoesNotShowFinalBallot(t *testing.T) {
	b, tg, db := votingBot(t)
	const user = 1001
	b.rankedList[user] = []int{900002, 900001}
	db.EXPECT().AddVote(gomock.Any(), int64(user), gomock.Any()).Return(errors.New("db down"))
	b.HandleUpdate(context.Background(), press(user, 900003))
	calls := tg.callsFor(user)
	var edited, errSent bool
	for _, c := range calls {
		if strings.Contains(c.text, msgVoteFinalBallotHeader) || strings.Contains(c.text, "Ваш бюллетень принят") {
			t.Fatalf("после ошибки записи показан итог: %q", c.text)
		}
		if c.method == "editMessageText" && !c.hasMarkup && strings.Contains(c.text, "3. Кандидат 3") {
			edited = true
		}
		if c.method == "sendMessage" && c.text == msgSysVoteSaveError {
			errSent = true
		}
	}
	if !edited || !errSent {
		t.Fatalf("edited=%v errSent=%v\n%s", edited, errSent, tg.texts(user))
	}
}

const adminChat = -100500

func adminBot(t *testing.T) (*Bot, *recTelegram, *mocks.MockvoteChain, *mocks.Mockschulze) {
	t.Helper()
	setAdminChat(t, adminChat)
	tg, api := newRecTelegram(t)
	ctrl := gomock.NewController(t)
	db := mocks.NewMockvoteChain(ctrl)
	sz := mocks.NewMockschulze(ctrl)
	b := newTestBot()
	b.botAPI, b.voteChain, b.schulze = api, db, sz
	return b, tg, db, sz
}

func adminUpdate(text string) tgbotapi.Update {
	return tgbotapi.Update{Message: adminCommand(adminChat, text)}
}

// Пока голосование открыто, добавлять, снимать и удалять кандидатов нельзя: ответ в админ-чат, в БД ничего
func TestCandidateChangesRefusedWhileVotingActive(t *testing.T) {
	for _, cmd := range []string{
		"/add_candidate 123456, Петров Пётр, 2 бакалавриат, о себе",
		"/ban_candidate 123456",
		"/delete_candidate 123456",
	} {
		t.Run(strings.Fields(cmd)[0], func(t *testing.T) {
			b, tg, _, _ := adminBot(t) // mock без EXPECT: любой вызов БД провалит тест
			b.activeVoting = true
			b.HandleUpdate(context.Background(), adminUpdate(cmd))
			if out := tg.texts(adminChat); !strings.Contains(out, msgAdminCandidatesFrozen) {
				t.Fatalf("нет ответа о запрете:\n%s", out)
			}
		})
	}
}

// При закрытом голосовании /delete_candidate снимает кандидата мягко (BanCandidate), строку не удаляет
func TestDeleteCandidateIsSoftWhenVotingClosed(t *testing.T) {
	b, tg, db, _ := adminBot(t)
	db.EXPECT().BanCandidate(gomock.Any(), 123456).Return(nil)
	b.HandleUpdate(context.Background(), adminUpdate("/delete_candidate 123456"))
	if out := tg.texts(adminChat); strings.Contains(out, msgAdminCandidatesFrozen) {
		t.Fatalf("при закрытом голосовании запрет не нужен:\n%s", out)
	}
}

// При закрытом голосовании добавление и бан работают как раньше
func TestAddAndBanCandidateWhenVotingClosed(t *testing.T) {
	b, _, db, _ := adminBot(t)
	db.EXPECT().GetAllVotes(gomock.Any()).Return(nil, nil)
	db.EXPECT().AddCandidate(gomock.Any(), models.Candidate{CandidateID: 123456, Name: "Петров Пётр", Course: "2 бакалавриат", Description: "о себе", IsEligible: true}).Return(nil)
	db.EXPECT().BanCandidate(gomock.Any(), 123456).Return(nil)
	b.HandleUpdate(context.Background(), adminUpdate("/add_candidate 123456, Петров Пётр, 2 бакалавриат, о себе"))
	b.HandleUpdate(context.Background(), adminUpdate("/ban_candidate 123456"))
}

// Голосование закрыто, но бюллетени уже есть: добавить кандидата нельзя (его нет ни в одном бюллетене),
// а снять — можно
func TestAddCandidateRefusedWhenBallotsExist(t *testing.T) {
	b, tg, db, _ := adminBot(t)
	db.EXPECT().GetAllVotes(gomock.Any()).Return([]models.Vote{{DelegateID: 1}, {DelegateID: 2}}, nil)
	// AddCandidate не ожидается: вызов провалит тест
	b.HandleUpdate(context.Background(), adminUpdate("/add_candidate 123456, Петров Пётр, 2 бакалавриат, о себе"))
	if out := tg.texts(adminChat); !strings.Contains(out, fmt.Sprintf(msgAdminCandidateAddAfterVotesFmt, 2)) {
		t.Fatalf("нет отказа:\n%s", out)
	}
	db.EXPECT().BanCandidate(gomock.Any(), 123456).Return(nil)
	b.HandleUpdate(context.Background(), adminUpdate("/delete_candidate 123456"))
}

// Таймаут обработки апдейта отсчитывается после блокировки пользователя: апдейт, ждавший предыдущий апдейт
// того же чата, получает полные updateTimeout, а не остаток
func TestUpdateTimeoutStartsAfterUserLock(t *testing.T) {
	b, _, db := votingBot(t)
	const user = 1001
	unlock := b.lockUser(user) // предыдущий апдейт этого чата ещё обрабатывается
	var remaining time.Duration
	db.EXPECT().CheckExistDelegateByTelegramID(gomock.Any(), int64(user)).DoAndReturn(func(ctx context.Context, _ int64) (bool, error) {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Error("у контекста нет дедлайна")
		}
		remaining = time.Until(dl)
		return false, nil
	})
	done := make(chan struct{})
	go func() {
		b.HandleUpdate(context.Background(), tgbotapi.Update{Message: commandMsg(user, "private", "/vote")})
		close(done)
	}()
	time.Sleep(700 * time.Millisecond)
	unlock()
	<-done
	if remaining < updateTimeout-300*time.Millisecond {
		t.Fatalf("после ожидания блокировки осталось %v из %v", remaining, updateTimeout)
	}
}

// Соединение закрыто, пока апдейт ждал блокировку пользователя: апдейт не обрабатывается (Telegram доставит его снова),
// ни Telegram, ни БД не вызываются
func TestAbandonedUpdateIsSkipped(t *testing.T) {
	b, tg, _ := votingBot(t) // mock БД без EXPECT: любой вызов провалит тест
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.HandleUpdate(ctx, press(1001, 900001))
	b.HandleUpdate(ctx, tgbotapi.Update{Message: commandMsg(1001, "private", "/vote")})
	if calls := tg.callsFor(1001); len(calls) != 0 {
		t.Fatalf("брошенный апдейт обработан: %v", calls)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.rankedList[1001]) != 0 {
		t.Fatalf("rankedList=%v", b.rankedList[1001])
	}
}

func expectLoads(sz *mocks.Mockschulze) {
	sz.EXPECT().SetCandidates().Return(nil)
	sz.EXPECT().SetVotes().Return(nil)
	sz.EXPECT().SetCandidatesByCourse().Return(nil)
	sz.EXPECT().SetVotesByCourse().Return(nil)
}

func successLogged() bool {
	for _, e := range logHook.AllEntries() {
		if strings.Contains(e.Message, "Результаты успешно вычислены") {
			return true
		}
	}
	return false
}

// Всё посчитано: «успешно» в логе и CSV
func TestResultsSuccess(t *testing.T) {
	b, _, _, sz := adminBot(t)
	logHook.Reset()
	expectLoads(sz)
	sz.EXPECT().ComputeResults(gomock.Any()).Return(nil)
	sz.EXPECT().ComputeGlobalTop(gomock.Any()).Return(nil)
	sz.EXPECT().SaveResultsToCSV(gomock.Any()).Return(nil)
	b.HandleUpdate(context.Background(), adminUpdate("/results"))
	if !successLogged() {
		t.Fatal("нет «Результаты успешно вычислены»")
	}
}

// Неразрешённая ничья на курсе: уведомление в админ-чат с именами, общие места не считаются, «успешно» нет, CSV есть
func TestResultsCourseTieNotifiesAdmin(t *testing.T) {
	b, tg, _, sz := adminBot(t)
	logHook.Reset()
	expectLoads(sz)
	tie := &schulzepkg.TieError{Course: "2 бакалавриат",
		Tied:       []models.Candidate{{CandidateID: 900003, Name: "Кандидат <3>"}, {CandidateID: 900005, Name: "Кандидат 5"}},
		Unresolved: []models.Candidate{{CandidateID: 900003, Name: "Кандидат <3>"}, {CandidateID: 900005, Name: "Кандидат 5"}}}
	sz.EXPECT().ComputeResults(gomock.Any()).Return(errors.Join(tie))
	sz.EXPECT().SaveResultsToCSV(gomock.Any()).Return(nil) // CSV нужен для ручного подсчёта
	b.HandleUpdate(context.Background(), adminUpdate("/results"))
	out := tg.texts(adminChat)
	for _, want := range []string{"Неразрешимая ничья на курсе «2 бакалавриат»", "st900003 Кандидат &lt;3&gt;", "st900005 Кандидат 5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("нет %q в сообщении:\n%s", want, out)
		}
	}
	if successLogged() {
		t.Fatal("при ничьей записано «успешно»")
	}
}

// Неразрешённая ничья в общих местах: место, число мест, уже избранные
func TestResultsCommonTieNotifiesAdmin(t *testing.T) {
	b, tg, _, sz := adminBot(t)
	logHook.Reset()
	expectLoads(sz)
	sz.EXPECT().ComputeResults(gomock.Any()).Return(nil)
	tie := &schulzepkg.TieError{Place: 3, Places: 6,
		Decided:    []models.Candidate{{CandidateID: 900001, Name: "А"}, {CandidateID: 900002, Name: "Б"}},
		Tied:       []models.Candidate{{CandidateID: 900007, Name: "В"}, {CandidateID: 900008, Name: "Г"}, {CandidateID: 900009, Name: "Д"}},
		Unresolved: []models.Candidate{{CandidateID: 900007, Name: "В"}, {CandidateID: 900008, Name: "Г"}}}
	sz.EXPECT().ComputeGlobalTop(gomock.Any()).Return(fmt.Errorf("ComputeGlobalTop: %w", tie))
	sz.EXPECT().SaveResultsToCSV(gomock.Any()).Return(nil)
	b.HandleUpdate(context.Background(), adminUpdate("/results"))
	out := tg.texts(adminChat)
	for _, want := range []string{"за место 3 из 6", "Уже избраны на общие места: st900001 А, st900002 Б", "Осталось мест: 4", "сузил круг до st900007 В, st900008 Г"} {
		if !strings.Contains(out, want) {
			t.Fatalf("нет %q в сообщении:\n%s", want, out)
		}
	}
	if successLogged() {
		t.Fatal("при ничьей записано «успешно»")
	}
}

// Ошибка подсчёта (не ничья): сообщение об ошибке, без «успешно» и без CSV
func TestResultsErrorNoSuccessNoCSV(t *testing.T) {
	b, tg, _, sz := adminBot(t)
	logHook.Reset()
	expectLoads(sz)
	sz.EXPECT().ComputeResults(gomock.Any()).Return(errors.Join(errors.New("ComputeResults: cant AddResult for 1 бакалавриат: db <down>")))
	// SaveResultsToCSV и ComputeGlobalTop не ожидаются: вызов провалит тест
	b.HandleUpdate(context.Background(), adminUpdate("/results"))
	out := tg.texts(adminChat)
	if !strings.Contains(out, "Подсчёт не завершён из-за ошибки") || !strings.Contains(out, "db &lt;down&gt;") {
		t.Fatalf("нет сообщения об ошибке:\n%s", out)
	}
	if successLogged() {
		t.Fatal("при ошибке записано «успешно»")
	}
}

// Нет бюллетеней: отдельное сообщение, подсчёт не запускается
func TestResultsNoVotes(t *testing.T) {
	b, tg, _, sz := adminBot(t)
	logHook.Reset()
	sz.EXPECT().SetCandidates().Return(nil)
	sz.EXPECT().SetVotes().Return(fmt.Errorf("SetVotes: %w", schulzepkg.ErrNoVotes))
	b.HandleUpdate(context.Background(), adminUpdate("/results"))
	if out := tg.texts(adminChat); !strings.Contains(out, msgAdminResultsNoVotes) {
		t.Fatalf("нет сообщения о пустой урне:\n%s", out)
	}
	if successLogged() {
		t.Fatal("без бюллетеней записано «успешно»")
	}
}

// Почта: регистр и домен @student.spbu.ru нормализуются, пробелы внутри — нет
func TestEmailInputNormalization(t *testing.T) {
	cases := []struct {
		in      string
		invalid bool
	}{
		{"St123456", false},
		{"ST123456", false},
		{"st123456@student.spbu.ru", false},
		{"ST123456@STUDENT.SPBU.RU", false},
		{"st 123456", true},
		{"st123456@gmail.com", true},
		{"123456", true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			b, tg, db := newRegistrationBot(t)
			db.EXPECT().CheckFerification(gomock.Any(), 123456).Return(false, nil).AnyTimes()
			b.userStates[1] = StateWaitingForEmail
			b.handleText(context.Background(), privateMsg(1, c.in))
			got := tg.last()
			if (got == msgRegInvalidEmail) != c.invalid {
				t.Fatalf("%q: ответ %q", c.in, got)
			}
		})
	}
}

// В ожидании кода почта в любом регистре/с доменом распознаётся как запрос письма; код с пробелами — неверный формат
func TestCodeStateNormalization(t *testing.T) {
	b, tg, db := newRegistrationBot(t)
	db.EXPECT().CheckFerification(gomock.Any(), 123456).Return(false, nil).AnyTimes()
	b.userStates[1] = StateWaitingForCode
	b.handleText(context.Background(), privateMsg(1, "ST123456@student.spbu.ru"))
	if tg.last() == msgRegInvalidCode {
		t.Fatal("почта в ожидании кода принята за неверный код")
	}
	b.userStates[1] = StateWaitingForCode
	b.handleText(context.Background(), privateMsg(1, "128 165"))
	if tg.last() != msgRegInvalidCode {
		t.Fatalf("код с пробелом: ответ %q", tg.last())
	}
}

// /delete_delegate: проголосовавшего не удаляем, в админ-чат приходит отказ; не голосовавший удаляется молча, как раньше
func TestDeleteDelegateVotedRefused(t *testing.T) {
	b, tg, db, _ := adminBot(t)
	db.EXPECT().DeleteDelegate(gomock.Any(), 123456).Return(fmt.Errorf("chain.DeleteDelegate: delegate 123456: %w", chain.ErrDelegateHasVoted))
	b.HandleUpdate(context.Background(), adminUpdate("/delete_delegate 123456"))
	if out := tg.texts(adminChat); !strings.Contains(out, fmt.Sprintf(msgAdminDelegateHasVotedFmt, 123456)) {
		t.Fatalf("нет отказа:\n%s", out)
	}
	db.EXPECT().DeleteDelegate(gomock.Any(), 654321).Return(nil)
	b.HandleUpdate(context.Background(), adminUpdate("/delete_delegate 654321"))
	if out := tg.texts(adminChat); strings.Count(out, "sendMessage") != 1 {
		t.Fatalf("при удалении не голосовавшего не должно быть ответа в админ-чат:\n%s", out)
	}
}

// Лимит писем исчерпан: если письмо с действующим кодом уже ушло — просим ввести код; если нет — к организаторам
func TestEmailLimitTextDependsOnEmailedCode(t *testing.T) {
	for _, emailed := range []bool{true, false} {
		t.Run(fmt.Sprint("emailed=", emailed), func(t *testing.T) {
			b, tg, db := newRegistrationBot(t)
			db.EXPECT().CheckFerification(gomock.Any(), 123456).Return(false, nil).AnyTimes()
			b.codeStore[1], b.userEmail[1], b.userStates[1] = 111111, 123456, StateWaitingForCode
			b.codeEmailed[1] = emailed
			b.emailsDay = time.Now().UTC().Format("2006-01-02")
			b.codesByTGDay[1] = perTGDailyLimit
			b.handleText(context.Background(), privateMsg(1, "st123456"))
			want := msgRegEmailLimit
			if emailed {
				want = fmt.Sprintf(msgRegEmailLimitCodeSentFmt, 123456)
			}
			if got := tg.last(); got != want {
				t.Fatalf("ответ %q, ожидался %q", got, want)
			}
			if code, ok, _ := pending(t, b, 1); !ok || code != 111111 {
				t.Fatalf("код должен остаться прежним: %d %v", code, ok)
			}
		})
	}
}

// Новый код сбрасывает отметку «письмо ушло»: код для другого делегата ещё никому не отправлен
func TestNewCodeResetsEmailedMark(t *testing.T) {
	b := newTestBot()
	b.codeStore[1], b.userEmail[1], b.codeEmailed[1] = 111111, 123456, true
	if _, err := b.ensurePendingCode(1, 654321); err != nil {
		t.Fatal(err)
	}
	if b.pendingCodeEmailed(1, 654321) || b.pendingCodeEmailed(1, 123456) {
		t.Fatal("отметка о письме пережила смену кода")
	}
}

// Повторный /start при ожидающем коде: бот остаётся в ожидании кода, код из письма принимается сразу
func TestStartWithPendingCodeKeepsWaitingForCode(t *testing.T) {
	b, tg, db := newRegistrationBot(t)
	db.EXPECT().CheckExistDelegateByTelegramID(gomock.Any(), int64(1)).Return(false, nil)
	db.EXPECT().VerificateDelegate(gomock.Any(), 123456, gomock.Any()).Return(nil)
	b.codeStore[1], b.userEmail[1], b.userStates[1] = 111111, 123456, StateWaitingForCode
	b.handleCommand(context.Background(), commandMsg(1, "private", "/start"))
	if got := tg.last(); got != fmt.Sprintf(msgRegStartCodePendingFmt, 123456) {
		t.Fatalf("ответ на /start: %q", got)
	}
	if _, ok, state := pending(t, b, 1); !ok || state != StateWaitingForCode {
		t.Fatalf("код ok=%v, состояние %q", ok, state)
	}
	b.handleText(context.Background(), privateMsg(1, "111111"))
	if got := tg.last(); got != msgRegDone {
		t.Fatalf("код после /start: %q", got)
	}
}

// /start без ожидающего кода — как раньше: приветствие и ожидание почты
func TestStartWithoutPendingCodeAsksEmail(t *testing.T) {
	b, tg, db := newRegistrationBot(t)
	db.EXPECT().CheckExistDelegateByTelegramID(gomock.Any(), int64(1)).Return(false, nil)
	b.handleCommand(context.Background(), commandMsg(1, "private", "/start"))
	if got := tg.last(); got != msgRegWelcome {
		t.Fatalf("ответ на /start: %q", got)
	}
	if _, _, state := pending(t, b, 1); state != StateWaitingForEmail {
		t.Fatalf("состояние %q", state)
	}
}
