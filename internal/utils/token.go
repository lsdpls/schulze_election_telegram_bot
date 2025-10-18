package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
)

// GenerateVoteToken генерирует детерминированный токен для делегата
// Использует HMAC-SHA256 для необратимого преобразования telegramID
// Один telegramID всегда даёт один и тот же токен
func GenerateVoteToken(telegramID int64) string {
	// HMAC-SHA256(secret_key, "vote_<telegram_id>")
	h := hmac.New(sha256.New, []byte(config.VoteTokenSecret))
	h.Write([]byte(fmt.Sprintf("vote_%d", telegramID)))
	hash := h.Sum(nil)

	// Кодируем первые 12 байт в base32 (без padding) для увеличения уникальности
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(hash[:12])

	// Берём 16 символов и форматируем XXXX-YYYY-ZZZZ-AAAA
	token := encoded[:16]
	return fmt.Sprintf("%s-%s-%s-%s", token[0:4], token[4:8], token[8:12], token[12:16])
}
