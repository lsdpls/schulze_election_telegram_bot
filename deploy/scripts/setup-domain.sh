#!/bin/bash

# Настройка домена вместо ngrok: DNS, certbot (standalone), хуки продления, webhook Telegram.
# Использование: ./scripts/setup-domain.sh [--drop-pending]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(dirname "$SCRIPT_DIR")"
ENV_FILE="$DEPLOY_DIR/.env"

fail() { echo "❌ $*" >&2; exit 1; }

# apt-get, certbot и хуки в /etc/letsencrypt требуют root
[ "$(id -u)" -eq 0 ] || fail "Запускать от root: sudo $0 $*"

DROP_PENDING_FLAG=""
for arg in "$@"; do
    case "$arg" in
        --drop-pending) DROP_PENDING_FLAG=true ;;
        -h|--help) echo "Использование: $0 [--drop-pending]"; exit 0 ;;
        *) fail "Неизвестный аргумент: $arg" ;;
    esac
done

[ -f "$ENV_FILE" ] || fail "Нет файла $ENV_FILE"

# Из .env экспортируем только нужные ключи; одна пара кавычек вокруг значения снимается
while IFS= read -r line || [ -n "$line" ]; do
    line=${line%$'\r'}
    case "$line" in ''|'#'*) continue ;; esac
    case "$line" in *=*) ;; *) continue ;; esac
    key=${line%%=*}
    val=${line#*=}
    [[ $key =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    case "$val" in
        \"*\") val=${val#\"}; val=${val%\"} ;;
        \'*\') val=${val#\'}; val=${val%\'} ;;
    esac
    case "$key" in
        DOMAIN|EMAIL|TELEGRAM_APITOKEN|WEBHOOK_SECRET|DROP_PENDING) export "$key=$val" ;;
    esac
done < "$ENV_FILE"

DOMAIN=${DOMAIN:-}
EMAIL=${EMAIL:-}
TELEGRAM_APITOKEN=${TELEGRAM_APITOKEN:-}
WEBHOOK_SECRET=${WEBHOOK_SECRET:-}
DROP_PENDING=${DROP_PENDING_FLAG:-${DROP_PENDING:-false}}

# Те же правила, что в internal/config (WEBHOOK_SECRET) и у Telegram (токен)
host_re='^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$'
token_re='^[0-9]+:[A-Za-z0-9_-]{30,}$'
secret_re='^[A-Za-z0-9_-]+$'   # длина отдельно: {32,256} превышает RE_DUP_MAX=255 в bash 3.2/BSD

[ -n "$DOMAIN" ] || fail "Переменная DOMAIN не задана в .env"
[[ $DOMAIN =~ $host_re ]] || fail "DOMAIN не похож на имя хоста: '$DOMAIN'"
[ -n "$EMAIL" ] || fail "Переменная EMAIL не задана в .env (контакт для Let's Encrypt)"
[ -n "$TELEGRAM_APITOKEN" ] || fail "Переменная TELEGRAM_APITOKEN не задана в .env"
[[ $TELEGRAM_APITOKEN =~ $token_re ]] || fail "TELEGRAM_APITOKEN неверного формата (ожидается 123456:ABC-DEF..., без кавычек)"
[ -n "$WEBHOOK_SECRET" ] || fail "Переменная WEBHOOK_SECRET не задана в .env (бот отвергает webhook без неё)"
[[ $WEBHOOK_SECRET =~ $secret_re && ${#WEBHOOK_SECRET} -ge 32 && ${#WEBHOOK_SECRET} -le 256 ]] || fail "WEBHOOK_SECRET: 32..256 символов A-Za-z0-9_-, без кавычек"
case "$DROP_PENDING" in
    true|false) ;;
    *) fail "DROP_PENDING: true или false, получено '$DROP_PENDING'" ;;
esac

echo "🌐 Настройка домена $DOMAIN для Telegram Bot"

# 1. Проверяем, что домен указывает на сервер
echo "📡 Проверяем DNS..."
IP=$(curl -4fsS --max-time 10 https://ifconfig.me) || fail "Не удалось узнать внешний IP сервера (ifconfig.me)"
[ -n "$IP" ] || fail "ifconfig.me вернул пустой ответ"

resolve_domain() {
    if command -v getent >/dev/null 2>&1; then
        getent ahostsv4 "$DOMAIN" | awk '{print $1}'
    elif command -v dig >/dev/null 2>&1; then
        dig +short A "$DOMAIN"
    elif command -v nslookup >/dev/null 2>&1; then
        nslookup "$DOMAIN" | awk 'NR>2 && /^Address/ {print $2}'
    fi
}
RESOLVED=$(resolve_domain 2>/dev/null | sort -u || true)
if ! grep -qxF "$IP" <<< "$RESOLVED"; then
    echo "❌ Домен $DOMAIN не указывает на этот сервер ($IP), резолвится в: ${RESOLVED:-ничего}" >&2
    echo "Настройте A-запись в DNS: $DOMAIN → $IP" >&2
    exit 1
fi
echo "✅ DNS настроен правильно"

# 2. Устанавливаем certbot
echo "🔧 Устанавливаем certbot..."
if ! command -v certbot >/dev/null 2>&1; then
    apt-get update
    apt-get install -y certbot
fi

# 3. Останавливаем nginx (если запущен): standalone-режим certbot занимает порт 80.
# Работавший nginx поднимаем обратно при любом выходе, в том числе по ошибке под set -e
echo "⏹️ Останавливаем nginx..."
cd "$DEPLOY_DIR"
NGINX_WAS_UP=$(docker compose -f docker-compose.yml ps --status running -q nginx 2>/dev/null || true)
trap '[ -n "$NGINX_WAS_UP" ] && docker compose -f "$DEPLOY_DIR/docker-compose.yml" up -d nginx' EXIT
docker compose -f docker-compose.yml stop nginx 2>/dev/null || true

# 4. Получаем SSL сертификат
echo "🔒 Получаем SSL сертификат..."
certbot certonly --standalone --keep-until-expiring --non-interactive --agree-tos \
    --email "$EMAIL" -d "$DOMAIN"

# 5. Nginx автоматически подхватит переменные из .env

# 6. SSL сертификат получен, сервисы можно запустить отдельно
echo "✅ SSL сертификат успешно получен!"

# 7. Автообновление: certbot.timer из пакета (2 раза в сутки) + хуки, освобождающие порт 80
echo "🔄 Настраиваем автообновление сертификата..."
HOOKS_DIR=/etc/letsencrypt/renewal-hooks
mkdir -p "$HOOKS_DIR/pre" "$HOOKS_DIR/post"
cat > "$HOOKS_DIR/pre/schulze-stop-nginx.sh" <<HOOK
#!/bin/sh
# standalone-продление certbot требует свободный порт 80
docker compose -f "$DEPLOY_DIR/docker-compose.yml" stop nginx || true
HOOK
cat > "$HOOKS_DIR/post/schulze-start-nginx.sh" <<HOOK
#!/bin/sh
docker compose -f "$DEPLOY_DIR/docker-compose.yml" up -d nginx || true
HOOK
chmod +x "$HOOKS_DIR/pre/schulze-stop-nginx.sh" "$HOOKS_DIR/post/schulze-start-nginx.sh"
# Старая cron-строка запускала renew при занятом nginx порту 80 — убираем
crontab -l 2>/dev/null | grep -v 'certbot renew' | crontab - || true

# 8. Настраиваем webhook. setWebhook вызываем всегда: getWebhookInfo не показывает secret_token
echo "📱 Настраиваем webhook для домена $DOMAIN..."
WEBHOOK_URL="https://$DOMAIN/election_bot/"
API="https://api.telegram.org/bot$TELEGRAM_APITOKEN"

# Тело через stdin, чтобы секрет не светился в списке процессов
RESPONSE=$(printf '{"url": "%s", "secret_token": "%s", "allowed_updates": ["message", "callback_query"], "drop_pending_updates": %s}' \
        "$WEBHOOK_URL" "$WEBHOOK_SECRET" "$DROP_PENDING" \
    | curl -sS --fail-with-body --max-time 30 -X POST "$API/setWebhook" \
        -H 'Content-Type: application/json' --data @-) || fail "Ошибка setWebhook: ${RESPONSE:-нет ответа}"
grep -q '"ok":true' <<< "$RESPONSE" || fail "Ошибка setWebhook: $RESPONSE"
echo "✅ Webhook установлен: $WEBHOOK_URL (drop_pending_updates=$DROP_PENDING)"

INFO=$(curl -sS --fail-with-body --max-time 30 "$API/getWebhookInfo") || fail "Ошибка getWebhookInfo: ${INFO:-нет ответа}"
echo "ℹ️  getWebhookInfo:"
for k in url pending_update_count last_error_date last_error_message; do
    grep -oE "\"$k\":(\"[^\"]*\"|[^,}]*)" <<< "$INFO" || true
done

echo "✅ Настройка завершена!"
echo ""
if [ -z "$NGINX_WAS_UP" ]; then
    echo "🚀 Для запуска приложения выполните:"
    echo "cd $DEPLOY_DIR && docker compose up -d"
    echo ""
fi
echo "Проверить автообновление сертификата после запуска: certbot renew --dry-run"
echo "Если бот молчит: make webhook_info (last_error_message), после смены WEBHOOK_SECRET — make webhook-prod"
echo ""
