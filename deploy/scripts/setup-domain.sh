#!/bin/bash

# Скрипт для настройки домена вместо ngrok

set -e

# Определяем путь к директории скрипта
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(dirname "$SCRIPT_DIR")"

# Загружаем переменные из .env файла
if [ -f "$DEPLOY_DIR/.env" ]; then
    export $(grep -v '^#' "$DEPLOY_DIR/.env" | xargs)
fi

# Проверяем, что переменные заданы
if [ -z "$DOMAIN" ]; then
    echo "❌ Ошибка: Переменная DOMAIN не задана в .env файле"
    exit 1
fi

if [ -z "$EMAIL" ]; then
    echo "❌ Ошибка: Переменная EMAIL не задана в .env файле"
    exit 1
fi

echo "🌐 Настройка домена $DOMAIN для Telegram Bot"

# 1. Проверяем, что домен указывает на сервер
echo "📡 Проверяем DNS..."
if ! nslookup $DOMAIN | grep -q "$(curl -s ifconfig.me)"; then
    echo "❌ Ошибка: Домен $DOMAIN не указывает на этот сервер"
    echo "Настройте A-запись в DNS: $DOMAIN → $(curl -s ifconfig.me)"
    exit 1
fi
echo "✅ DNS настроен правильно"

# 2. Устанавливаем certbot
echo "🔧 Устанавливаем certbot..."
if ! command -v certbot &> /dev/null; then
    apt update
    apt install -y certbot
fi

# 3. Останавливаем nginx (если запущен)
echo "⏹️ Останавливаем nginx..."
cd "$DEPLOY_DIR"
docker-compose -f docker-compose.yml down nginx 2>/dev/null || true

# 4. Получаем SSL сертификат
echo "🔒 Получаем SSL сертификат..."
certbot certonly --standalone -d $DOMAIN --email $EMAIL --agree-tos --non-interactive

# 5. Nginx автоматически подхватит переменные из .env

# 6. SSL сертификат получен, сервисы можно запустить отдельно
echo "✅ SSL сертификат успешно получен!"

# 7. Настраиваем автообновление сертификата
echo "🔄 Настраиваем автообновление сертификата..."
(crontab -l 2>/dev/null; echo "0 2 * * 0 /usr/bin/certbot renew --quiet && cd $DEPLOY_DIR && docker-compose restart nginx") | crontab -

# 8. Проверяем и настраиваем webhook
echo "🔗 Проверяем текущий webhook..."
CURRENT_WEBHOOK=$(curl -s "https://api.telegram.org/bot$TELEGRAM_APITOKEN/getWebhookInfo" | grep -o '"url":"[^"]*"' | cut -d'"' -f4)
WEBHOOK_URL="https://$DOMAIN/election_bot/"
    
if [ "$CURRENT_WEBHOOK" = "$WEBHOOK_URL" ]; then
    echo "✅ Webhook уже настроен правильно: $CURRENT_WEBHOOK"
else
    echo "📱 Настраиваем webhook для домена $DOMAIN..."

    RESPONSE=$(curl -s -X POST "https://api.telegram.org/bot$TELEGRAM_APITOKEN/setWebhook" \
        -H "Content-Type: application/json" \
        -d "{\"url\": \"$WEBHOOK_URL\"}")
    
    if echo "$RESPONSE" | grep -q '"ok":true'; then
        # Проверяем, что webhook действительно настроился правильно
        sleep 2  # Даем время Telegram API обновиться
        CURRENT_WEBHOOK=$(curl -s "https://api.telegram.org/bot$TELEGRAM_APITOKEN/getWebhookInfo" | grep -o '"url":"[^"]*"' | cut -d'"' -f4)
        
        if [ "$CURRENT_WEBHOOK" = "$WEBHOOK_URL" ]; then
            echo "✅ Webhook подтвержден: $CURRENT_WEBHOOK"
        else
            echo "⚠️  Webhook настроен, но проверка не прошла. Текущий URL: $CURRENT_WEBHOOK"
        fi
    else
        echo "❌ Ошибка настройки webhook: $RESPONSE"
    fi
fi

echo "✅ Настройка завершена!"
echo ""
echo "🚀 Для запуска приложения выполните:"
echo "cd $DEPLOY_DIR && docker-compose up -d"
echo ""
