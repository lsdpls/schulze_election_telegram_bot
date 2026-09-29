package email

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
)

// withFakePostbox поднимает фейковый API и подменяет конфиг, клиент и backoff до конца теста.
func withFakePostbox(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	oldKey, oldSecret, oldEndpoint, oldRegion := config.PostboxKeyID, config.PostboxSecretKey, config.PostboxEndpoint, config.PostboxRegion
	config.PostboxKeyID, config.PostboxSecretKey = "id", "secret"
	config.PostboxEndpoint, config.PostboxRegion = srv.URL, "ru-central1"
	oldClient, oldBackoff := postboxHTTPClient, postboxBackoff
	client := *srv.Client()
	client.CheckRedirect = oldClient.CheckRedirect // редиректы не следуем, как в проде
	postboxHTTPClient = &client
	postboxBackoff = func(int, bool) time.Duration { return 0 }
	t.Cleanup(func() {
		postboxHTTPClient, postboxBackoff = oldClient, oldBackoff
		config.PostboxKeyID, config.PostboxSecretKey, config.PostboxEndpoint, config.PostboxRegion = oldKey, oldSecret, oldEndpoint, oldRegion
		srv.Close()
	})
	return srv
}

// 503/429 на первых попытках → успех после повтора.
func TestPostboxRetriesOnServerError(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls int32
			withFakePostbox(t, func(w http.ResponseWriter, r *http.Request) {
				if atomic.AddInt32(&calls, 1) < 3 {
					w.WriteHeader(status)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"MessageId":"ok"}`))
			})
			if err := sendViaPostbox(context.Background(), "noreply@amcp.space", "u@example.com", "s", "b"); err != nil {
				t.Fatalf("expected success after retries, got %v", err)
			}
			if calls != 3 {
				t.Fatalf("expected 3 calls, got %d", calls)
			}
		})
	}
}

// Экспоненциальный backoff: 500ms·2^(attempt-1), при 429/5xx вдвое больше; attempt<1 считается первым.
func TestPostboxBackoffTable(t *testing.T) {
	cases := []struct {
		attempt   int
		throttled bool
		want      time.Duration
	}{
		{1, false, 500 * time.Millisecond},
		{2, false, time.Second},
		{3, false, 2 * time.Second},
		{1, true, time.Second},
		{2, true, 2 * time.Second},
		{3, true, 4 * time.Second},
		{0, false, 500 * time.Millisecond},
	}
	for _, c := range cases {
		if got := postboxBackoff(c.attempt, c.throttled); got != c.want {
			t.Errorf("postboxBackoff(%d, %v) = %v, want %v", c.attempt, c.throttled, got, c.want)
		}
	}
}

// 4xx (кроме 429) не повторяются.
func TestPostboxNoRetryOnClientError(t *testing.T) {
	var calls int32
	withFakePostbox(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"__type":"MailFromDomainNotVerifiedException"}`))
	})
	err := sendViaPostbox(context.Background(), "noreply@amcp.space", "u@example.com", "s", "b")
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "MailFromDomainNotVerified") {
		t.Fatalf("expected HTTP 400 error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call, got %d", calls)
	}
}

// 301 на заглушку — ошибка, а не «отправлено»: редирект не следуем, повтора нет.
func TestPostboxRedirectIsError(t *testing.T) {
	var landingCalls int32
	landing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&landingCalls, 1)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>welcome</html>"))
	}))
	defer landing.Close()

	var apiCalls int32
	withFakePostbox(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		http.Redirect(w, r, landing.URL+"/", http.StatusMovedPermanently)
	})
	err := sendViaPostbox(context.Background(), "noreply@amcp.space", "u@example.com", "s", "b")
	if err == nil || !strings.Contains(err.Error(), "HTTP 301") {
		t.Fatalf("expected HTTP 301 error, got %v", err)
	}
	if apiCalls != 1 || landingCalls != 0 {
		t.Fatalf("expected 1 API call and 0 landing calls, got %d and %d", apiCalls, landingCalls)
	}
}

// Сеть недоступна: все попытки исчерпаны.
func TestPostboxNetworkFailureExhaustsAttempts(t *testing.T) {
	srv := withFakePostbox(t, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close()
	err := sendViaPostbox(context.Background(), "noreply@amcp.space", "u@example.com", "s", "b")
	if err == nil || !strings.Contains(err.Error(), "attempt 4/4") {
		t.Fatalf("expected exhausted attempts error, got %v", err)
	}
}

// Connection refused: ровно postboxAttempts попыток dial, без пауз после последней.
func TestPostboxRetriesOnConnectionRefused(t *testing.T) {
	srv := withFakePostbox(t, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close()
	var dials int32
	postboxHTTPClient = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			atomic.AddInt32(&dials, 1)
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
	start := time.Now()
	err := sendViaPostbox(context.Background(), "noreply@amcp.space", "u@example.com", "s", "b")
	if err == nil || !strings.Contains(err.Error(), "attempt 4/4") {
		t.Fatalf("expected exhausted attempts error, got %v", err)
	}
	if dials != postboxAttempts {
		t.Fatalf("expected %d dials, got %d", postboxAttempts, dials)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("retries took %v, expected fast", el)
	}
}

// Запрос записан, ответа нет: повтора быть не должно (письмо могло уйти).
// Хэндлер молчит до обрыва соединения клиентом по ResponseHeaderTimeout — без sleep.
func TestPostboxNoRetryAfterRequestWritten(t *testing.T) {
	var calls int32
	withFakePostbox(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	})
	postboxHTTPClient = &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 100 * time.Millisecond}}
	err := sendViaPostbox(context.Background(), "noreply@amcp.space", "u@example.com", "s", "b")
	if !errors.Is(err, ErrMaybeSent) {
		t.Fatalf("expected ErrMaybeSent, got %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected exactly 1 call, got %d", n)
	}
}

// Обрезка тела ошибки не ломает руну и даёт валидный UTF-8.
func TestErrBodyUTF8(t *testing.T) {
	const maxLen = 512 + len("…")
	for _, in := range []string{"a" + strings.Repeat("я", 300), strings.Repeat("€", 200)} {
		got := errBody([]byte(in))
		if !utf8.ValidString(got) {
			t.Errorf("errBody(%q…) is not valid UTF-8: %q", in[:8], got)
		}
		if len(got) > maxLen {
			t.Errorf("errBody len = %d, want ≤ %d", len(got), maxLen)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("errBody(%q…) not truncated: %q", in[:8], got)
		}
	}
	if got := errBody([]byte("bad \xff bytes")); !utf8.ValidString(got) || !strings.Contains(got, "�") {
		t.Errorf("invalid bytes not replaced: %q", got)
	}
}

// EMAIL_PROVIDER=postbox: письмо уходит через API от адреса SMTP_EMAIL.
func TestDispatchPostbox(t *testing.T) {
	var gotBody atomic.Pointer[string]
	withFakePostbox(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s := string(b)
		gotBody.Store(&s)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"MessageId":"ok"}`))
	})
	oldProv, oldEmail := config.EmailProvider, config.SMTPEmail
	config.EmailProvider, config.SMTPEmail = config.EmailProviderPostbox, "noreply@example.org"
	t.Cleanup(func() { config.EmailProvider, config.SMTPEmail = oldProv, oldEmail })

	if err := SendVerificationCodeToEmailWithContext(context.Background(), "st123456@student.spbu.ru", 123456); err != nil {
		t.Fatalf("send: %v", err)
	}
	body := gotBody.Load()
	if body == nil || !strings.Contains(*body, `"FromEmailAddress":"noreply@example.org"`) {
		t.Fatalf("unexpected body: %v", body)
	}
}
