package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
)

// Провайдер "postbox": HTTPS-API Yandex Cloud Postbox (совместим с SES v2), порт 443.
// https://yandex.cloud/ru/docs/postbox/operations/send-email

// postboxHTTPClient переопределяется в тестах; короткий connect-таймаут, т.к. часть соединений к API теряется.
// Ping по HTTP/2 выявляет молча умершее keep-alive соединение до того, как в него уйдёт письмо.
// Редиректы не следуем: 301 на заглушку превратил бы подписанный POST в GET, а 200 text/html — в «отправлено».
var postboxHTTPClient = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 4 * time.Second}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          4,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		HTTP2:                 &http.HTTP2Config{SendPingTimeout: 15 * time.Second, PingTimeout: 5 * time.Second},
	},
}

// postboxAttempts — попытки при сетевых ошибках, 429 и 5xx.
const postboxAttempts = 4

// ErrMaybeSent — запрос ушёл целиком, ответа нет: повтор мог бы отправить письмо дважды.
var ErrMaybeSent = errors.New("письмо могло быть отправлено, ответ не получен")

// postboxBackoff — пауза перед повтором attempt (1..3); переопределяется в тестах.
var postboxBackoff = func(attempt int, throttled bool) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 500 * time.Millisecond << (attempt - 1) // 500ms, 1s, 2s
	if throttled {
		d *= 2 // 429/5xx: 1s, 2s, 4s
	}
	return d
}

// postboxRequest — тело SendEmail (SES v2), простое текстовое письмо.
type postboxRequest struct {
	FromEmailAddress string `json:"FromEmailAddress"`
	Destination      struct {
		ToAddresses []string `json:"ToAddresses"`
	} `json:"Destination"`
	Content struct {
		Simple struct {
			Subject postboxContent `json:"Subject"`
			Body    struct {
				Text postboxContent `json:"Text"`
			} `json:"Body"`
		} `json:"Simple"`
	} `json:"Content"`
}

type postboxContent struct {
	Data    string `json:"Data"`
	Charset string `json:"Charset"`
}

// sendViaPostbox отправляет письмо через API Postbox (подпись SigV4).
func sendViaPostbox(ctx context.Context, from, to, subject, body string) error {
	if config.PostboxKeyID == "" || config.PostboxSecretKey == "" {
		return fmt.Errorf("postbox credentials not configured (POSTBOX_KEY_ID / POSTBOX_SECRET_KEY)")
	}

	var payload postboxRequest
	payload.FromEmailAddress = from
	payload.Destination.ToAddresses = []string{to}
	payload.Content.Simple.Subject = postboxContent{Data: subject, Charset: "UTF-8"}
	payload.Content.Simple.Body.Text = postboxContent{Data: body, Charset: "UTF-8"}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("postbox: marshal request: %w", err)
	}

	url := strings.TrimRight(config.PostboxEndpoint, "/") + "/v2/email/outbound-emails"

	// wait — пауза перед следующей попыткой; false — попытки кончились или контекст завершён
	wait := func(attempt int, throttled bool) bool {
		if attempt >= postboxAttempts {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(postboxBackoff(attempt, throttled)):
			return true
		}
	}

	var lastErr error
	for attempt := 1; attempt <= postboxAttempts; attempt++ {
		// подпись содержит время — запрос собираем заново
		var wrote atomic.Bool
		trace := &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { wrote.Store(true) }}
		req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("postbox: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		sigV4Sign(req, raw, config.PostboxKeyID, config.PostboxSecretKey, config.PostboxRegion, "ses", time.Now())

		resp, err := postboxHTTPClient.Do(req)
		if err != nil {
			if wrote.Load() {
				postboxHTTPClient.CloseIdleConnections()
				return fmt.Errorf("postbox: %w: %w", ErrMaybeSent, err)
			}
			lastErr = fmt.Errorf("postbox: attempt %d/%d: %w", attempt, postboxAttempts, err)
			if isTimeout(err) {
				postboxHTTPClient.CloseIdleConnections() // не переиспользовать зависшее соединение
			}
			if !wait(attempt, false) {
				return lastErr
			}
			continue
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("postbox: HTTP %d: %s", resp.StatusCode, errBody(respBody))
			if !wait(attempt, true) {
				return lastErr
			}
		default:
			// прочие 4xx (ключ, домен, запрос): повтор бессмыслен
			return fmt.Errorf("postbox: HTTP %d: %s", resp.StatusCode, errBody(respBody))
		}
	}
	return lastErr
}

func isTimeout(err error) bool {
	var nerr net.Error
	return errors.As(err, &nerr) && nerr.Timeout()
}

// errBody — до 512 байт тела ответа одной строкой, валидный UTF-8 (ошибки уходят в Telegram, он битые байты отвергает).
func errBody(b []byte) string {
	limit := 512
	s := strings.NewReplacer("\r", " ", "\n", " ").Replace(string(b))
	if len(s) > limit {
		for limit > 0 && !utf8.RuneStart(s[limit]) { // не резать руну пополам
			limit--
		}
		s = s[:limit] + "…"
	}
	return strings.TrimSpace(strings.ToValidUTF8(s, "�"))
}
