package bot

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mocks "github.com/lsdpls/schulze_election_telegram_bot/internal/bot/mocks"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/models"

	"github.com/golang/mock/gomock"
)

func lifecycleBot(t *testing.T) *Bot {
	t.Helper()
	old := runStateFile
	runStateFile = filepath.Join(t.TempDir(), "bot.state")
	t.Cleanup(func() { runStateFile = old })
	db := mocks.NewMockvoteChain(gomock.NewController(t))
	db.EXPECT().GetAllDelegates(gomock.Any()).Return([]models.Delegate{
		{DelegateID: 1, TelegramID: sql.NullInt64{Int64: 11, Valid: true}, HasVoted: true},
		{DelegateID: 2, TelegramID: sql.NullInt64{Int64: 22, Valid: true}},
		{DelegateID: 3},
	}, nil).AnyTimes()
	db.EXPECT().GetAllVotes(gomock.Any()).Return([]models.Vote{{DelegateID: 1}}, nil).AnyTimes()
	db.EXPECT().GetAllCandidates(gomock.Any()).Return([]models.Candidate{{CandidateID: 7, IsEligible: true}, {CandidateID: 8}}, nil).AnyTimes()
	db.EXPECT().GetAllResults(gomock.Any()).Return(nil, nil).AnyTimes()
	b := newTestBot()
	b.voteChain = db
	b.rankedList = make(map[int64][]int)
	return b
}

func lastLog(t *testing.T) string {
	t.Helper()
	e := logHook.LastEntry()
	if e == nil {
		t.Fatal("в лог ничего не записано")
	}
	return e.Message
}

func stateFile(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(runStateFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Первый запуск: сведений о прошлом запуске нет, в сообщении сводка по базе, файл состояния — running
func TestLogStartupFirstRun(t *testing.T) {
	b := lifecycleBot(t)
	b.LogStartup(context.Background())
	msg := lastLog(t)
	for _, want := range []string{"Бот запущен", "Сведений о предыдущем запуске нет", "Голосование сейчас закрыто", "/start_voting",
		"делегатов 3 (зарегистрировано 2, проголосовало 1), бюллетеней 1, допущенных кандидатов 1, строк результатов 0"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("нет %q в %q", want, msg)
		}
	}
	if !strings.Contains(stateFile(t), "status=running") {
		t.Fatalf("файл состояния: %q", stateFile(t))
	}
}

// Штатная остановка при открытом голосовании: что пропадёт; следующий запуск видит штатную остановку и открытое голосование
func TestLogShutdownThenStartup(t *testing.T) {
	b := lifecycleBot(t)
	b.LogStartup(context.Background())
	b.activeVoting = true
	b.Candidates = map[int]models.Candidate{7: {CandidateID: 7}, 9: {CandidateID: 9}}
	b.rankedList[100] = []int{7}    // недозаполненный бюллетень
	b.rankedList[200] = []int{7, 9} // сданный бюллетень не считается
	b.codeStore[300] = 123456
	b.LogShutdown("сигнал terminated")
	msg := lastLog(t)
	for _, want := range []string{"Бот останавливается (сигнал terminated)", "Голосование ОТКРЫТО", "незаконченных бюллетеней — 1", "ожидающих ввода кодов подтверждения — 1"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("нет %q в %q", want, msg)
		}
	}
	b.LogStartup(context.Background())
	msg = lastLog(t)
	if !strings.Contains(msg, "остановлен штатно") || !strings.Contains(msg, "голосование было ОТКРЫТО") {
		t.Fatalf("сообщение о запуске после штатной остановки: %q", msg)
	}
}

// Падение посреди голосования: файл остался running с открытым голосованием — запуск сообщает об аварии
func TestLogStartupAfterCrash(t *testing.T) {
	b := lifecycleBot(t)
	b.LogStartup(context.Background())
	noteVotingState(true) // /start_voting, затем процесс упал, не записав остановку
	b.LogStartup(context.Background())
	msg := lastLog(t)
	if !strings.Contains(msg, "АВАРИЙНО") || !strings.Contains(msg, "голосование было ОТКРЫТО") {
		t.Fatalf("сообщение после падения: %q", msg)
	}
}
