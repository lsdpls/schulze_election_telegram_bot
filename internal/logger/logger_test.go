package logger

import (
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sirupsen/logrus"
)

func TestFormatNotificationLinksUserID(t *testing.T) {
	got := formatNotification("WARN", "⚠️", "123 <b>x</b> & y", int64(123), " <b>x</b> & y")
	want := `⚠️<a href="tg://user?id=123">WARN</a>: 123 &lt;b&gt;x&lt;/b&gt; &amp; y`
	if got != want {
		t.Fatalf("int64:\n got %q\nwant %q", got, want)
	}
	got = formatNotification("INFO", "🔎", "7 ok", 7)
	want = `🔎<a href="tg://user?id=7">INFO</a>: 7 ok`
	if got != want {
		t.Fatalf("int:\n got %q\nwant %q", got, want)
	}
}

func TestFormatNotificationNoLinkForNonID(t *testing.T) {
	err := errors.New("smtp: 550 <bad> & worse")
	got := formatNotification("ERROR", "📛", "Ошибка: "+err.Error(), err)
	want := "📛ERROR: Ошибка: smtp: 550 &lt;bad&gt; &amp; worse"
	if got != want {
		t.Fatalf("error arg:\n got %q\nwant %q", got, want)
	}
	for _, first := range []interface{}{"str", 1.5, nil, []int{1}} {
		got := formatNotification("ERROR", "📛", "x", first)
		if got != "📛ERROR: x" {
			t.Errorf("first arg %#v: got %q", first, got)
		}
	}
}

func TestFormatNotificationNoArgs(t *testing.T) {
	if got, want := formatNotification("INFO", "🔎", "plain"), "🔎INFO: plain"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := formatNotification("INFO", "🔎", ""), "🔎INFO: "; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatNotificationTruncates(t *testing.T) {
	text := strings.Repeat("€", 2000) // 6000 байт, 3500 не делится на 3 — режем по границе руны
	got := formatNotification("ERROR", "📛", text, int64(1))
	prefix := `📛<a href="tg://user?id=1">ERROR</a>: `
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("unexpected prefix in %q", got[:60])
	}
	body := strings.TrimPrefix(got, prefix)
	if !strings.HasSuffix(body, "…") {
		t.Fatal("нет многоточия")
	}
	if !utf8.ValidString(body) {
		t.Fatal("обрезка порвала руну")
	}
	if n := len(body) - len("…"); n != 3498 {
		t.Fatalf("тело %d байт, want 3498", n)
	}
	if len(got) > 4096 {
		t.Fatalf("сообщение %d байт, превышает лимит Telegram", len(got))
	}
}

// Битый UTF-8 в тексте (обрывок SMTP-ответа) не должен дойти до Telegram как есть
func TestFormatNotificationInvalidUTF8(t *testing.T) {
	text := "smtp: 550 \xff\xfe bad \xc3 tail"
	if utf8.ValidString(text) {
		t.Fatal("тестовая строка должна быть невалидной")
	}
	got := formatNotification("ERROR", "📛", text, int64(9))
	if !utf8.ValidString(got) {
		t.Fatalf("результат невалиден: %q", got)
	}
	if !strings.Contains(got, "�") {
		t.Fatalf("нет замены U+FFFD: %q", got)
	}
	if !strings.Contains(got, "smtp: 550 ") || !strings.Contains(got, " bad ") || !strings.HasSuffix(got, " tail") {
		t.Fatalf("валидные части потеряны: %q", got)
	}
	// Замена стоит до экранирования: сама она HTML-безопасна, а обрезка идёт по валидным рунам
	long := strings.Repeat("\xff", 5000)
	if got := formatNotification("ERROR", "📛", long); !utf8.ValidString(got) || len(got) > 4096 {
		t.Fatalf("длинный битый текст: valid=%v len=%d", utf8.ValidString(got), len(got))
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 3); got != "abc" {
		t.Fatalf("короткая строка изменена: %q", got)
	}
	// Сущность на границе не разрывается
	s := strings.Repeat("a", 3497) + "&amp;" + strings.Repeat("b", 100)
	if got, want := truncate(s, telegramMaxBody), strings.Repeat("a", 3497)+"…"; got != want {
		t.Fatalf("сущность порвана: хвост %q", got[3490:])
	}
	// Завершённая сущность сохраняется
	s = strings.Repeat("a", 3495) + "&amp;" + strings.Repeat("b", 100)
	if got, want := truncate(s, telegramMaxBody), strings.Repeat("a", 3495)+"&amp;…"; got != want {
		t.Fatalf("сущность потеряна: хвост %q", got[3490:])
	}
	if got := truncate(strings.Repeat("я", 10), 5); got != "яя…" {
		t.Fatalf("граница руны: %q", got)
	}
}

// Методы логгера без Telegram API и без аргументов не паникуют
func TestLoggerNilAPINoPanic(t *testing.T) {
	lg := logrus.New()
	lg.SetOutput(io.Discard)
	l := &Logger{entry: logrus.NewEntry(lg), telegramLogLevel: logrus.DebugLevel}
	l.Info()
	l.Error(int64(5), " <b>&")
	l.Warnf("%d <b>", 5)
	l.Debug(errors.New("e"))
	l.Errorf("no args")
}
