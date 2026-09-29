package email

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
)

const testSMTPPassword = "app-password"

// fakeSMTP — минимальный SMTP-сервер для тестов: implicit TLS или STARTTLS, PLAIN, DATA.
type fakeSMTP struct {
	ln       net.Listener
	tlsConf  *tls.Config
	implicit bool // TLS с первого байта
	starttls bool // анонсировать и принимать STARTTLS
	silent   bool // после TLS-рукопожатия не слать баннер

	authReply string // ответ на AUTH поверх TLS; по умолчанию "235 2.7.0 ok"

	mu   sync.Mutex
	cmds []string
	data string
	wg   sync.WaitGroup
}

func newFakeSMTP(t *testing.T, implicit, starttls, silent bool) *fakeSMTP {
	t.Helper()
	cert, pool := testCert(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{ln: ln, tlsConf: &tls.Config{Certificates: []tls.Certificate{cert}}, implicit: implicit, starttls: starttls, silent: silent, authReply: "235 2.7.0 ok"}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.wg.Add(1)
			go func() { defer s.wg.Done(); s.serve(conn) }()
		}
	}()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	oldTLS := smtpTLSConfig
	smtpTLSConfig = func(host string) *tls.Config {
		return &tls.Config{ServerName: host, RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	oldHost, oldPort, oldEmail, oldPass, oldUser, oldMode, oldProv := config.SMTPHost, config.SMTPPort, config.SMTPEmail, config.SMTPPassword, config.SMTPUser, config.SMTPTLS, config.EmailProvider
	config.SMTPHost, config.SMTPPort, config.SMTPEmail, config.SMTPPassword, config.SMTPUser = "127.0.0.1", port, "noreply@example.org", testSMTPPassword, ""
	config.EmailProvider = config.EmailProviderSMTP
	config.SMTPTLS = config.SMTPTLSStartTLS
	if implicit {
		config.SMTPTLS = config.SMTPTLSImplicit
	}
	t.Cleanup(func() {
		ln.Close()
		s.wg.Wait()
		smtpTLSConfig = oldTLS
		config.SMTPHost, config.SMTPPort, config.SMTPEmail, config.SMTPPassword, config.SMTPUser, config.SMTPTLS, config.EmailProvider = oldHost, oldPort, oldEmail, oldPass, oldUser, oldMode, oldProv
	})
	return s
}

func (s *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	secure := false
	if s.implicit {
		tc := tls.Server(conn, s.tlsConf)
		if err := tc.Handshake(); err != nil {
			return
		}
		conn, secure = tc, true
	}
	if s.silent {
		io.Copy(io.Discard, conn) // ждём, пока клиент закроет соединение
		return
	}
	tp := textproto.NewConn(conn)
	tp.PrintfLine("220 fake ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.cmds = append(s.cmds, line)
		s.mu.Unlock()
		verb, _, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO":
			tp.PrintfLine("250-fake")
			tp.PrintfLine("250-AUTH PLAIN")
			tp.PrintfLine("250-8BITMIME")
			if s.starttls && !secure {
				tp.PrintfLine("250-STARTTLS")
			}
			tp.PrintfLine("250 OK")
		case "STARTTLS":
			if !s.starttls || secure {
				tp.PrintfLine("502 STARTTLS not supported")
				continue
			}
			tp.PrintfLine("220 go ahead")
			tc := tls.Server(conn, s.tlsConf)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, secure = tc, true
			tp = textproto.NewConn(conn)
		case "AUTH":
			if !secure {
				tp.PrintfLine("538 encryption required")
				continue
			}
			s.mu.Lock()
			reply := s.authReply
			s.mu.Unlock()
			tp.PrintfLine("%s", reply)
		case "MAIL", "RCPT":
			tp.PrintfLine("250 OK")
		case "DATA":
			tp.PrintfLine("354 go")
			b, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.data = string(b)
			s.mu.Unlock()
			tp.PrintfLine("250 queued")
		case "QUIT":
			tp.PrintfLine("221 bye")
			return
		default:
			tp.PrintfLine("500 unknown")
		}
	}
}

func (s *fakeSMTP) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cmds...)
}

func (s *fakeSMTP) message() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

// testCert — самоподписанный сертификат на 127.0.0.1 и пул с ним.
func testCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func hasCommand(cmds []string, prefix string) bool {
	for _, c := range cmds {
		if strings.HasPrefix(strings.ToUpper(c), prefix) {
			return true
		}
	}
	return false
}

// checkSession проверяет команды сессии и разбирает письмо.
func checkSession(t *testing.T, s *fakeSMTP, code string) {
	t.Helper()
	cmds := s.commands()
	if !hasCommand(cmds, "EHLO EXAMPLE.ORG") {
		t.Errorf("no 'EHLO example.org' in %q", cmds)
	}
	var auth string
	for _, c := range cmds {
		if strings.HasPrefix(c, "AUTH PLAIN ") {
			auth = strings.TrimPrefix(c, "AUTH PLAIN ")
		}
	}
	raw, err := base64.StdEncoding.DecodeString(auth)
	if err != nil || string(raw) != "\x00noreply@example.org\x00"+testSMTPPassword {
		t.Errorf("bad AUTH PLAIN payload %q (err %v)", raw, err)
	}
	if !hasCommand(cmds, "MAIL FROM:<NOREPLY@EXAMPLE.ORG>") || !hasCommand(cmds, "RCPT TO:<ST123456@STUDENT.SPBU.RU>") {
		t.Errorf("MAIL/RCPT missing in %q", cmds)
	}

	msg, err := mail.ReadMessage(strings.NewReader(s.message()))
	if err != nil {
		t.Fatalf("ReadMessage: %v\n%s", err, s.message())
	}
	subj, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subj != "Ваш код подтверждения" {
		t.Errorf("Subject = %q (err %v)", subj, err)
	}
	if _, err := msg.Header.Date(); err != nil {
		t.Errorf("Date: %v", err)
	}
	if id := msg.Header.Get("Message-ID"); !strings.HasSuffix(id, "@example.org>") {
		t.Errorf("Message-ID = %q", id)
	}
	if cte := msg.Header.Get("Content-Transfer-Encoding"); cte != "quoted-printable" {
		t.Errorf("Content-Transfer-Encoding = %q", cte)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil || !strings.Contains(string(body), code) {
		t.Errorf("body %q (err %v) does not contain %s", body, err, code)
	}
}

func TestSMTPImplicitTLS(t *testing.T) {
	s := newFakeSMTP(t, true, false, false)
	if err := SendVerificationCodeToEmailWithContext(context.Background(), "st123456@student.spbu.ru", 123456); err != nil {
		t.Fatalf("send: %v", err)
	}
	checkSession(t, s, "123456")
}

func TestSMTPStartTLS(t *testing.T) {
	s := newFakeSMTP(t, false, true, false)
	if err := SendVerificationCodeToEmailWithContext(context.Background(), "st123456@student.spbu.ru", 654321); err != nil {
		t.Fatalf("send: %v", err)
	}
	checkSession(t, s, "654321")
	cmds := s.commands()
	if !hasCommand(cmds, "STARTTLS") {
		t.Errorf("no STARTTLS in %q", cmds)
	}
}

// Сервер не поддерживает STARTTLS: ни AUTH, ни MAIL открытым текстом.
func TestSMTPStartTLSRefusedFailsClosed(t *testing.T) {
	s := newFakeSMTP(t, false, false, false)
	err := SendVerificationCodeToEmailWithContext(context.Background(), "st123456@student.spbu.ru", 123456)
	if err == nil || !strings.Contains(err.Error(), "failed to start TLS") {
		t.Fatalf("expected STARTTLS error, got %v", err)
	}
	cmds := s.commands()
	if hasCommand(cmds, "AUTH") || hasCommand(cmds, "MAIL") {
		t.Fatalf("credentials or mail sent in clear: %q", cmds)
	}
}

// Отмена контекста после dial обрывает сессию, а не ждёт дедлайн сокета.
func TestSMTPContextCancel(t *testing.T) {
	newFakeSMTP(t, true, false, true)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	start := time.Now()
	err := SendVerificationCodeToEmailWithContext(ctx, "st123456@student.spbu.ru", 123456)
	if err == nil {
		t.Fatal("expected error after cancel")
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("returned after %v, expected ~100ms", el)
	}
}

// starttls против порта с implicit TLS: сервер молчит — в ошибке подсказка про SMTP_TLS.
func TestSMTPStartTLSAgainstImplicitPortHint(t *testing.T) {
	newFakeSMTP(t, true, false, true)
	config.SMTPTLS = config.SMTPTLSStartTLS
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := SendVerificationCodeToEmailWithContext(ctx, "st123456@student.spbu.ru", 123456)
	if err == nil || !strings.Contains(err.Error(), "порт ожидает implicit TLS") {
		t.Fatalf("expected implicit-TLS hint, got %v", err)
	}
}

// Сервер отверг AUTH: ошибка аутентификации, MAIL не отправляется.
func TestSMTPAuthRejected(t *testing.T) {
	s := newFakeSMTP(t, true, false, false)
	s.mu.Lock()
	s.authReply = "535 5.7.8 auth failed"
	s.mu.Unlock()
	err := SendVerificationCodeToEmailWithContext(context.Background(), "st123456@student.spbu.ru", 123456)
	if err == nil || !strings.Contains(err.Error(), "SMTP authentication failed") {
		t.Fatalf("expected auth error, got %v", err)
	}
	if cmds := s.commands(); hasCommand(cmds, "MAIL") {
		t.Fatalf("MAIL sent after failed auth: %q", cmds)
	}
}

// Обёртка с таймаутом по умолчанию.
func TestSendVerificationCodeDefaultTimeout(t *testing.T) {
	s := newFakeSMTP(t, true, false, false)
	if err := SendVerificationCodeToEmail("st123456@student.spbu.ru", 123456); err != nil {
		t.Fatalf("send: %v", err)
	}
	checkSession(t, s, "123456")
}
