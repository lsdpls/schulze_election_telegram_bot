package bot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Файл состояния запуска (рядом с bot.log, папка logs хранится между перезапусками контейнера).
// По нему новый запуск понимает, как завершился предыдущий: штатно (status=stopped) или аварийно
// (status=running — процесс не успел записать остановку: сбой, нехватка памяти, принудительное завершение),
// и было ли открыто голосование (voting=open)
var runStateFile = "./logs/bot.state"

type runState struct {
	stopped bool      // предыдущий запуск остановлен штатно
	voting  bool      // голосование было открыто
	at      time.Time // когда записано
}

func readRunState() (runState, bool) {
	raw, err := os.ReadFile(runStateFile)
	if err != nil {
		return runState{}, false
	}
	var st runState
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch k {
		case "status":
			st.stopped = v == "stopped"
		case "voting":
			st.voting = v == "open"
		case "at":
			st.at, _ = time.Parse(time.RFC3339, v)
		}
	}
	return st, true
}

func writeRunState(stopped, voting bool) {
	status, vote := "running", "closed"
	if stopped {
		status = "stopped"
	}
	if voting {
		vote = "open"
	}
	data := fmt.Sprintf("status=%s\nvoting=%s\nat=%s\n", status, vote, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(runStateFile, []byte(data), 0o644); err != nil {
		log.Errorf("Не удалось записать файл состояния запуска %s: %v", runStateFile, err)
	}
}

// LogStartup пишет в лог и лог-чат, что бот запущен: как завершился предыдущий запуск, что из-за перезапуска
// потеряно (всё, что жило только в памяти) и что сохранено в базе. Вызывать один раз при старте
func (b *Bot) LogStartup(ctx context.Context) {
	prev, ok := readRunState()
	var how string
	switch {
	case !ok:
		how = "Сведений о предыдущем запуске нет (первый запуск или файл состояния удалён)."
	case prev.stopped:
		how = fmt.Sprintf("Предыдущий запуск остановлен штатно %s.", prev.at.Format("02.01 15:04:05"))
	default:
		how = fmt.Sprintf("Предыдущий запуск завершился АВАРИЙНО: сбой процесса, нехватка памяти или принудительная остановка. "+
			"Последняя запись о нём — %s.", prev.at.Format("02.01 15:04:05"))
	}
	voting := ""
	if ok && prev.voting {
		voting = " До перезапуска голосование было ОТКРЫТО."
	}
	log.Warnf("🔄 Бот запущен. %s%s Голосование сейчас закрыто: чтобы продолжить, отправьте /start_voting в админ-чат. "+
		"Из памяти бота стёрты: незаконченные бюллетени (делегатам — заново /vote), коды подтверждения, ожидавшие ввода "+
		"(делегатам — заново ввести почту), лимиты и паузы писем, уровень логов, заданный через /log. В базе сохранено: %s.",
		how, voting, b.dbSummary(ctx))
	writeRunState(false, false)
}

// LogShutdown пишет в лог и лог-чат, что бот останавливается и что при этом пропадёт из памяти. reason — причина (сигнал)
func (b *Bot) LogShutdown(reason string) {
	b.mu.RLock()
	active := b.activeVoting
	total := len(b.Candidates)
	unfinished := 0
	for _, ranked := range b.rankedList {
		if len(ranked) < total {
			unfinished++
		}
	}
	pending := len(b.codeStore)
	b.mu.RUnlock()

	state := "закрыто"
	if active {
		state = "ОТКРЫТО — после запуска его нужно открыть снова: /start_voting"
	}
	log.Warnf("⏹ Бот останавливается (%s). Голосование %s. Будут потеряны: незаконченных бюллетеней — %d, "+
		"ожидающих ввода кодов подтверждения — %d, лимиты и паузы писем, уровень логов из /log. "+
		"Делегаты, регистрации, бюллетени, кандидаты и результаты хранятся в базе и сохранятся.", reason, state, unfinished, pending)
	writeRunState(true, active)
}

// noteVotingState обновляет файл состояния при открытии и закрытии голосования: если бот упадёт,
// следующий запуск сообщит, было ли голосование открыто
func noteVotingState(open bool) {
	writeRunState(false, open)
}

// dbSummary — сводка по базе для сообщения о запуске
func (b *Bot) dbSummary(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	delegates, err := b.voteChain.GetAllDelegates(ctx)
	if err != nil {
		return fmt.Sprintf("не удалось прочитать базу (%v)", err)
	}
	registered, voted := 0, 0
	for _, d := range delegates {
		if d.TelegramID.Valid {
			registered++
		}
		if d.HasVoted {
			voted++
		}
	}
	votes, err := b.voteChain.GetAllVotes(ctx)
	if err != nil {
		return fmt.Sprintf("не удалось прочитать базу (%v)", err)
	}
	candidates, err := b.voteChain.GetAllCandidates(ctx)
	if err != nil {
		return fmt.Sprintf("не удалось прочитать базу (%v)", err)
	}
	eligible := 0
	for _, c := range candidates {
		if c.IsEligible {
			eligible++
		}
	}
	results, err := b.voteChain.GetAllResults(ctx)
	if err != nil {
		return fmt.Sprintf("не удалось прочитать базу (%v)", err)
	}
	return fmt.Sprintf("делегатов %d (зарегистрировано %d, проголосовало %d), бюллетеней %d, допущенных кандидатов %d, строк результатов %d",
		len(delegates), registered, voted, len(votes), eligible, len(results))
}
