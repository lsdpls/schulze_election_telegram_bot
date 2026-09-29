package email

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Сверка с curl --aws-sigv4: берём X-Amz-Date из его запроса и пересчитываем подпись сами.
// Покрывает только запрос без query (curl расходится со спецификацией в canonicalQuery — см. sigv4.go).
func TestSigV4MatchesCurl(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not found")
	}
	body := []byte(`{"FromEmailAddress":"noreply@amcp.space","Destination":{"ToAddresses":["x@example.com"]}}`)
	captured := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Clone(context.Background())
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	keyID, secret := "AKIDEXAMPLEKEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	cmd := exec.Command("curl", "-sS", "-o", "/dev/null",
		"--aws-sigv4", "aws:amz:ru-central1:ses",
		"--user", keyID+":"+secret,
		"-X", "POST", "-H", "Content-Type: application/json",
		"--data-binary", string(body),
		srv.URL+"/v2/email/outbound-emails")
	if out, err := cmd.CombinedOutput(); err != nil {
		if s := string(out); strings.Contains(s, "aws-sigv4") || strings.Contains(s, "unknown") {
			t.Skipf("curl without --aws-sigv4: %v: %s", err, out)
		}
		t.Fatalf("curl failed: %v: %s", err, out)
	}
	var got *http.Request
	select {
	case got = <-captured:
	case <-time.After(5 * time.Second):
		t.Fatal("no request captured")
	}
	curlAuth := got.Header.Get("Authorization")
	amzDate := got.Header.Get("X-Amz-Date")
	if curlAuth == "" || amzDate == "" {
		t.Fatalf("curl did not sign: auth=%q date=%q", curlAuth, amzDate)
	}
	if !strings.Contains(curlAuth, "SignedHeaders=content-type;host;x-amz-date,") {
		t.Skipf("curl signs a different header set, cannot compare: %s", curlAuth)
	}
	ts, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		t.Fatalf("bad X-Amz-Date from curl: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v2/email/outbound-emails", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	sigV4Sign(req, body, keyID, secret, "ru-central1", "ses", ts)
	if ours := req.Header.Get("Authorization"); ours != curlAuth {
		t.Fatalf("signature mismatch\n curl: %s\n ours: %s", curlAuth, ours)
	}
}

// Детерминированный вектор без curl в рантайме. Воспроизводится (проверено 2026-09-28, curl 8.7.1) командой
// curl --aws-sigv4 aws:amz:ru-central1:ses --user AKIDEXAMPLEKEY:wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
// -X POST -H 'Content-Type: application/json' -H 'Host: postbox.cloud.yandex.net' -H 'X-Amz-Date: 20260928T183224Z'
// --data-binary '<body>' http://127.0.0.1:<port>/v2/email/outbound-emails
// против httptest-сервера: curl подписывает переданный X-Amz-Date, Authorization снят с пришедшего запроса.
func TestSigV4KnownVector(t *testing.T) {
	const (
		keyID    = "AKIDEXAMPLEKEY"
		secret   = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
		amzDate  = "20260928T183224Z"
		wantAuth = "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLEKEY/20260928/ru-central1/ses/aws4_request, SignedHeaders=content-type;host;x-amz-date, Signature=5addd56892e76ced7c221cc2611a479f47c4f77f516fc7fe11bada25f21bd6c2"
	)
	body := []byte(`{"FromEmailAddress":"noreply@example.org","Destination":{"ToAddresses":["st123456@student.spbu.ru"]},"Content":{"Simple":{"Subject":{"Data":"Код","Charset":"UTF-8"},"Body":{"Text":{"Data":"Ваш код: 123456","Charset":"UTF-8"}}}}}`)
	ts, err := time.Parse("20060102T150405Z", amzDate)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "https://postbox.cloud.yandex.net/v2/email/outbound-emails", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	sigV4Sign(req, body, keyID, secret, "ru-central1", "ses", ts)
	if got := req.Header.Get("X-Amz-Date"); got != amzDate {
		t.Errorf("X-Amz-Date = %q, want %q", got, amzDate)
	}
	if got := req.Header.Get("Authorization"); got != wantAuth {
		t.Errorf("Authorization mismatch\n got: %s\nwant: %s", got, wantAuth)
	}
}

// Последовательные пробелы в значении заголовка схлопываются (SigV4).
func TestSigV4CollapsesHeaderSpaces(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sign := func(ct string) string {
		req, _ := http.NewRequest(http.MethodPost, "https://postbox.cloud.yandex.net/v2/email/outbound-emails", bytes.NewReader(nil))
		req.Header.Set("Content-Type", ct)
		sigV4Sign(req, nil, "id", "secret", "ru-central1", "ses", ts)
		return req.Header.Get("Authorization")
	}
	single, double := sign("application/json; charset=utf-8"), sign("application/json;   charset=utf-8  ")
	if single != double {
		t.Fatalf("signatures differ:\n single: %s\n double: %s", single, double)
	}
}

func TestCanonicalQuery(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"b=2&a=1", "a=1&b=2"},
		{"k=5&k=%3A", "k=%3A&k=5"},
		{"plus=1+2", "plus=1%2B2"},
		{"e=", "e="},
		{"e", "e="},
		{"a=1;b=2", "a=1%3Bb%3D2"},
		{"t=%7E", "t=~"},       // '~' unreserved: curl 8.7.1 оставил бы %7E
		{"x=a%3Db", "x=a%3Db"}, // '=' в значении остаётся %3D (roundtrip)
		{"x=a=b", "x=a%3Db"},   // сырой '=' в значении кодируется: curl 8.7.1 оставил бы a=b
	}
	for _, c := range cases {
		if got := canonicalQuery(c.in); got != c.want {
			t.Errorf("canonicalQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Тело и заголовки запроса к API Postbox на фейковом сервере.
func TestPostboxRequestShape(t *testing.T) {
	var gotBody string
	var gotAuth, gotCT string
	withFakePostbox(t, func(w http.ResponseWriter, r *http.Request) {
		b := new(bytes.Buffer)
		b.ReadFrom(r.Body)
		gotBody, gotAuth, gotCT = b.String(), r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		if r.URL.Path != "/v2/email/outbound-emails" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"MessageId":"test"}`))
	})

	if err := sendViaPostbox(context.Background(), "noreply@amcp.space", "user@student.spbu.ru", "Код", "Ваш код: 123456"); err != nil {
		t.Fatalf("sendViaPostbox: %v", err)
	}
	for _, want := range []string{`"FromEmailAddress":"noreply@amcp.space"`, `"ToAddresses":["user@student.spbu.ru"]`, `"Subject":{"Data":"Код","Charset":"UTF-8"}`, `"Text":{"Data":"Ваш код: 123456","Charset":"UTF-8"}`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body missing %s: %s", want, gotBody)
		}
	}
	if !strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256 Credential=id/") || gotCT != "application/json" {
		t.Errorf("bad headers: auth=%q ct=%q", gotAuth, gotCT)
	}
}
