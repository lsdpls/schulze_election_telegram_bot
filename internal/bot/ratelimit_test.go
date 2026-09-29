package bot

import (
	"errors"
	"testing"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
)

// newTestBot — бот без Telegram, БД и логгера, с картами регистрации и антиспама как после NewBot
func newTestBot() *Bot {
	return &Bot{
		userStates:         make(map[int64]string),
		codeStore:          make(map[int64]int),
		userEmail:          make(map[int64]int),
		lastCodeByTG:       make(map[int64]time.Time),
		lastCodeByDelegate: make(map[int]time.Time),
		codesByTGDay:       make(map[int64]int),
		codesByDelegateDay: make(map[int]int),
		codeAttempts:       make(map[int64]int),
	}
}

func setDailyLimit(t *testing.T, limit int) {
	t.Helper()
	old := config.EmailDailyLimit
	config.EmailDailyLimit = limit
	t.Cleanup(func() { config.EmailDailyLimit = old })
}

func mustReserve(t *testing.T, b *Bot, tg int64, del int, now time.Time) {
	t.Helper()
	wait, err := b.reserveCodeSend(tg, del, now)
	if err != nil || wait != 0 {
		t.Fatalf("reserveCodeSend(%d, %d): wait=%v err=%v, want 0, nil", tg, del, wait, err)
	}
}

func mustCooldown(t *testing.T, b *Bot, tg int64, del int, now time.Time, want time.Duration) {
	t.Helper()
	wait, err := b.reserveCodeSend(tg, del, now)
	if err != nil || wait != want {
		t.Fatalf("reserveCodeSend(%d, %d): wait=%v err=%v, want %v, nil", tg, del, wait, err, want)
	}
}

func mustUserLimit(t *testing.T, b *Bot, tg int64, del int, now time.Time) {
	t.Helper()
	wait, err := b.reserveCodeSend(tg, del, now)
	if !errors.Is(err, errUserLimit) || wait != 0 {
		t.Fatalf("reserveCodeSend(%d, %d): wait=%v err=%v, want errUserLimit", tg, del, wait, err)
	}
}

// counters проверяет общий счётчик и счётчики по ключам
func counters(t *testing.T, b *Bot, tg int64, del int, want int) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.emailsSent != want || b.codesByTGDay[tg] != want || b.codesByDelegateDay[del] != want {
		t.Fatalf("emailsSent=%d codesByTGDay[%d]=%d codesByDelegateDay[%d]=%d, want все %d",
			b.emailsSent, tg, b.codesByTGDay[tg], del, b.codesByDelegateDay[del], want)
	}
}

// NewBot открывает логгер (файл в ./logs) и подменяет log пакета — вызывается один раз на пакет
func TestNewBotInitialisesRateLimitState(t *testing.T) {
	setDailyLimit(t, 1)
	b := NewBot(nil, nil, nil)
	if b.lastCodeByTG == nil || b.lastCodeByDelegate == nil || b.codesByTGDay == nil ||
		b.codesByDelegateDay == nil || b.codeAttempts == nil {
		t.Fatal("карты антиспама не инициализированы")
	}
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	mustReserve(t, b, 1, 100001, t0)
	b.releaseCodeSend(1, 100001, t0, true)
}

func TestReserveCodeSendCooldown(t *testing.T) {
	setDailyLimit(t, 100)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	mustReserve(t, b, 1, 100001, t0)
	mustCooldown(t, b, 1, 100001, t0.Add(10*time.Second), 50*time.Second) // тот же пользователь
	mustCooldown(t, b, 2, 100001, t0.Add(10*time.Second), 50*time.Second) // тот же делегат с другого аккаунта
	mustCooldown(t, b, 1, 100002, t0.Add(10*time.Second), 50*time.Second) // тот же аккаунт на другого делегата
	mustReserve(t, b, 1, 100001, t0.Add(codeCooldown))                    // кулдаун истёк
	if b.emailsSent != 2 {
		t.Fatalf("emailsSent=%d, want 2 (отказы не считаются)", b.emailsSent)
	}
}

func TestReserveCodeSendDailyLimit(t *testing.T) {
	setDailyLimit(t, 2)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	mustReserve(t, b, 1, 100001, t0)
	mustReserve(t, b, 2, 100002, t0)
	if _, err := b.reserveCodeSend(3, 100003, t0); !errors.Is(err, errDailyLimit) {
		t.Fatalf("err=%v, want errDailyLimit", err)
	}
	// Кулдаун проверяется раньше лимита
	mustCooldown(t, b, 1, 100001, t0.Add(time.Second), codeCooldown-time.Second)
	// Новые сутки (UTC) обнуляют счётчик
	t1 := t0.Add(24 * time.Hour)
	mustReserve(t, b, 3, 100003, t1)
	if b.emailsSent != 1 || b.emailsDay != "2026-09-29" {
		t.Fatalf("emailsSent=%d emailsDay=%q, want 1 и 2026-09-29", b.emailsSent, b.emailsDay)
	}
}

// Лимиты аккаунта/делегата проверяются раньше общего: при обоих исчерпанных — errUserLimit
func TestReserveCodeSendLimitOrder(t *testing.T) {
	setDailyLimit(t, 1)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	b.emailsDay, b.emailsSent = "2026-09-28", 1
	b.codesByTGDay[1] = perTGDailyLimit
	b.codesByDelegateDay[100002] = perDelegateDailyLimit

	mustUserLimit(t, b, 1, 100001, t0)
	mustUserLimit(t, b, 2, 100002, t0)
	if _, err := b.reserveCodeSend(3, 100003, t0); !errors.Is(err, errDailyLimit) {
		t.Fatalf("err=%v, want errDailyLimit", err)
	}
}

// (perTGDailyLimit+1)-й код одному аккаунту за сутки (на разных делегатов) — errUserLimit; другие аккаунты не затронуты
func TestReserveCodeSendPerTGLimit(t *testing.T) {
	setDailyLimit(t, 100)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	for i := 0; i < perTGDailyLimit; i++ {
		mustReserve(t, b, 1, 100001+i, t0.Add(time.Duration(i)*codeCooldown))
	}
	now := t0.Add(perTGDailyLimit * codeCooldown)
	mustUserLimit(t, b, 1, 100001+perTGDailyLimit, now)
	mustUserLimit(t, b, 1, 100001, now.Add(time.Hour))
	if b.emailsSent != perTGDailyLimit {
		t.Fatalf("emailsSent=%d, want %d (отказы не считаются)", b.emailsSent, perTGDailyLimit)
	}
	mustReserve(t, b, 2, 100001+perTGDailyLimit, now)
}

// (perDelegateDailyLimit+1)-й код на одного делегата за сутки (с разных аккаунтов) — errUserLimit; на другого делегата — можно
func TestReserveCodeSendPerDelegateLimit(t *testing.T) {
	setDailyLimit(t, 100)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	for i := 0; i < perDelegateDailyLimit; i++ {
		mustReserve(t, b, int64(i+1), 100001, t0.Add(time.Duration(i)*codeCooldown))
	}
	now := t0.Add(perDelegateDailyLimit * codeCooldown)
	next := int64(perDelegateDailyLimit + 1)
	mustUserLimit(t, b, next, 100001, now)
	if b.emailsSent != perDelegateDailyLimit {
		t.Fatalf("emailsSent=%d, want %d (отказы не считаются)", b.emailsSent, perDelegateDailyLimit)
	}
	mustReserve(t, b, next, 100002, now)
}

// Новые сутки обнуляют счётчики по ключам и подавление логов
func TestReserveCodeSendDayRolloverClearsPerKey(t *testing.T) {
	setDailyLimit(t, 100)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	for i := 0; i < perTGDailyLimit; i++ {
		mustReserve(t, b, 1, 100001+i, t0.Add(time.Duration(i)*codeCooldown))
	}
	now := t0.Add(perTGDailyLimit * codeCooldown)
	mustUserLimit(t, b, 1, 100001, now)

	t1 := t0.Add(24 * time.Hour)
	mustReserve(t, b, 1, 100001, t1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.codesByTGDay) != 1 || b.codesByTGDay[1] != 1 || len(b.codesByDelegateDay) != 1 || b.codesByDelegateDay[100001] != 1 {
		t.Fatalf("после смены суток codesByTGDay=%v codesByDelegateDay=%v", b.codesByTGDay, b.codesByDelegateDay)
	}
	if b.emailsSent != 1 {
		t.Fatalf("после смены суток emailsSent=%d", b.emailsSent)
	}
}

// Возврат резерва: кулдауны сняты, счётчики откачены, тот же аккаунт/делегат может запросить код сразу
func TestReleaseCodeSendRefund(t *testing.T) {
	setDailyLimit(t, 100)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	mustReserve(t, b, 1, 100001, t0)
	counters(t, b, 1, 100001, 1)
	b.releaseCodeSend(1, 100001, t0, true)
	counters(t, b, 1, 100001, 0)
	if len(b.lastCodeByTG) != 0 || len(b.lastCodeByDelegate) != 0 {
		t.Fatalf("кулдауны не сняты: %v %v", b.lastCodeByTG, b.lastCodeByDelegate)
	}
	mustReserve(t, b, 1, 100001, t0.Add(time.Second))
	counters(t, b, 1, 100001, 1)

	// Повторный возврат не уводит счётчики ниже нуля
	b.releaseCodeSend(1, 100001, t0.Add(time.Second), true)
	b.releaseCodeSend(1, 100001, t0.Add(time.Second), true)
	counters(t, b, 1, 100001, 0)
}

// Без возврата (письмо могло уйти): кулдауны сняты, но квота остаётся потраченной
func TestReleaseCodeSendNoRefund(t *testing.T) {
	setDailyLimit(t, 100)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

	mustReserve(t, b, 1, 100001, t0)
	b.releaseCodeSend(1, 100001, t0, false)
	counters(t, b, 1, 100001, 1)
	if len(b.lastCodeByTG) != 0 || len(b.lastCodeByDelegate) != 0 {
		t.Fatalf("кулдауны не сняты: %v %v", b.lastCodeByTG, b.lastCodeByDelegate)
	}
	mustReserve(t, b, 1, 100001, t0.Add(time.Second))
	counters(t, b, 1, 100001, 2)
}

// Возврат старого резерва не снимает кулдаун более нового резерва того же ключа
func TestReleaseCodeSendStaleReservation(t *testing.T) {
	setDailyLimit(t, 100)
	b := newTestBot()
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(codeCooldown)

	mustReserve(t, b, 1, 100001, t0)
	mustReserve(t, b, 1, 100001, t1)
	b.releaseCodeSend(1, 100001, t0, true)
	mustCooldown(t, b, 1, 100001, t1.Add(time.Second), codeCooldown-time.Second)
	mustCooldown(t, b, 2, 100001, t1.Add(time.Second), codeCooldown-time.Second)
	mustCooldown(t, b, 1, 100002, t1.Add(time.Second), codeCooldown-time.Second)
	counters(t, b, 1, 100001, 1) // квота старого резерва возвращена, нового — нет
}

// Возврат резерва после смены суток не трогает счётчики нового дня
func TestReleaseCodeSendRefundAfterRollover(t *testing.T) {
	setDailyLimit(t, 100)
	b := newTestBot()
	tA := time.Date(2026, 9, 28, 23, 59, 59, 0, time.UTC)
	tB := time.Date(2026, 9, 29, 0, 0, 1, 0, time.UTC)

	mustReserve(t, b, 1, 100001, tA)
	mustReserve(t, b, 2, 100002, tB) // rollover: счётчики нового дня
	b.releaseCodeSend(1, 100001, tA, true)
	if b.emailsSent != 1 || b.codesByTGDay[2] != 1 || b.codesByDelegateDay[100002] != 1 {
		t.Fatalf("возврат вчерашнего резерва изменил счётчики нового дня: sent=%d byTG=%v byDel=%v", b.emailsSent, b.codesByTGDay, b.codesByDelegateDay)
	}
	if _, ok := b.lastCodeByTG[1]; ok {
		t.Fatal("кулдаун вчерашнего резерва должен быть снят")
	}
}
