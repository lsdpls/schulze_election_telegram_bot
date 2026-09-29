package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// sigV4Sign подписывает запрос по AWS Signature V4 (Postbox совместим с SES v2).
// Подписываются host, x-amz-date и content-type — тот же набор, что у curl --aws-sigv4.
func sigV4Sign(req *http.Request, body []byte, keyID, secret, region, service string, now time.Time) {
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(body)

	req.Header.Set("X-Amz-Date", amzDate)

	// канонические заголовки: lower-case, trim, пробелы схлопнуты до одного, сортировка по имени
	headers := map[string]string{
		"host":       req.Host,
		"x-amz-date": amzDate,
	}
	if ct := req.Header.Get("Content-Type"); ct != "" {
		headers["content-type"] = ct
	}
	names := make([]string, 0, len(headers))
	for n := range headers {
		names = append(names, n)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, n := range names {
		canonicalHeaders.WriteString(n)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.Join(strings.Fields(headers[n]), " "))
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(names, ";")

	// путь кодируется один раз (как у curl, в отличие от S3-стиля); endpoint без пути — проверено в config
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery(req.URL.RawQuery),
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	req.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential="+keyID+"/"+credentialScope+
			", SignedHeaders="+signedHeaders+
			", Signature="+signature)
}

// canonicalQuery — пары query, перекодированные по RFC 3986 и отсортированные по закодированным
// ключу и значению (SigV4: сортировка после кодирования). Разделитель только '&'; '+' — литерал.
// curl 8.7.1 отходит от спецификации: не декодирует уже закодированные октеты (%7E остаётся %7E)
// и не кодирует сырой '=' внутри значения; здесь — как в спецификации и aws-sdk (декодировать и
// перекодировать: %7E → '~', '=' → %3D). Postbox запросы без query, расхождение не задето.
func canonicalQuery(rawQuery string) string {
	type pair struct{ k, v string }
	var pairs []pair
	for _, kv := range strings.Split(rawQuery, "&") {
		if kv == "" {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		pairs = append(pairs, pair{awsEscape(queryUnescape(k)), awsEscape(queryUnescape(v))})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
}

// queryUnescape снимает %XX, не трогая '+'; битые %XX остаются как есть.
func queryUnescape(s string) string {
	u, err := url.QueryUnescape(strings.ReplaceAll(s, "+", "%2B"))
	if err != nil {
		return s
	}
	return u
}

// awsEscape — percent-encoding по RFC 3986, как требует SigV4.
func awsEscape(s string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&15])
		}
	}
	return b.String()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
