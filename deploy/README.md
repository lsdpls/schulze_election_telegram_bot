# Deployment Configuration

Эта папка содержит конфигурации и скрипты для развертывания приложения.

## Структура

```
deploy/
├── migrations/        # Миграции goose: схема БД (prod и dev)
├── migrations_dev/    # Тестовые данные (только dev, make migrate-dev)
├── nginx/             # Конфигурация Nginx
│   └── nginx.conf     # Конфигурация Nginx для production
├── scripts/           # Скрипты автоматизации
│   └── setup-domain.sh # Настройка домена и SSL
├── docker-compose.yml # Docker Compose: postgres, nginx, bot, frontend; ngrok — profile dev
├── dockerfile         # Dockerfile для сборки приложения
├── makefile           # Makefile с командами для деплоя
├── .env               # Переменные окружения
├── .env.example       # Пример переменных окружения
└── README.md          # Этот файл
```

## Использование

### Production (с доменом)
```bash
# Настройка домена, SSL и webhook (от root; читает DOMAIN, EMAIL, TELEGRAM_APITOKEN, WEBHOOK_SECRET, DROP_PENDING из .env)
sudo ./scripts/setup-domain.sh
# сброс очереди update при установке webhook — только до начала голосования
sudo ./scripts/setup-domain.sh --drop-pending

# Запуск в production: БД → миграции → bot, frontend, nginx
make app-prod
```

Страница с бюллетенями и результатами: `https://DOMAIN/votes`. Её JSON (`/election_bot/{votes,candidates,result}`) nginx отдаёт только запросам с самой страницы (`Sec-Fetch-Site: same-origin` или `Referer` с нашего домена), остальным — 403, плюс лимит 30 запросов/с с IP (заголовки можно подделать, поэтому защита — в самих данных). Данные обезличены: вместо делегата — HMAC-токен, времени голосования нет, недопущенные кандидаты не показываются. Бюллетени видны в реальном времени по мере поступления (решение владельца), результаты появляются после `/results`.

`make app-prod` накатывает только `migrations/` (схему). Тестовые данные из `migrations_dev/` в prod не загружаются — их ставит отдельная команда `make migrate-dev`, которую вызывает только dev-сценарий `make app2`.

### Development (с ngrok)
```bash
# БД → миграции → тестовые данные → bot → ngrok (сервис ngrok в docker-compose.yml, profile dev)
make app2
# webhook на NGROK_URL из .env
make webhook_create
```

`make ngrok-docker` = `docker compose --profile dev up -d ngrok`; без `--profile dev` compose сервис ngrok не видит.

### База данных
- `make migrate` — схема (`migrations/`), `make migrate-dev` — тестовые данные (`migrations_dev/`, после `migrate`).
- `make clean-db` — `TRUNCATE` всех таблиц (данные), схема остаётся.
- `make drop-schema CONFIRM=yes` — `goose reset`: откат всех миграций, все таблицы удаляются.
- goose ходит в `127.0.0.1:5432` — порт postgres опубликован только на loopback.

## SSL Сертификаты

`setup-domain.sh` получает сертификат Let's Encrypt через `certbot certonly --standalone` (nginx на это время останавливается и, если он работал, поднимается обратно при любом выходе из скрипта) и настраивает автообновление:

- продление запускает `certbot.timer` из apt-пакета (дважды в сутки, обновляет за 30 дней до истечения);
- хуки `/etc/letsencrypt/renewal-hooks/pre/schulze-stop-nginx.sh` и `post/schulze-start-nginx.sh` освобождают порт 80 на время продления и поднимают nginx обратно;
- старая cron-строка `certbot renew` (падала из-за занятого порта 80) при запуске скрипта удаляется.

Проверка после запуска стека: `certbot renew --dry-run` (кратко останавливает nginx).

Сертификаты хранятся в `/etc/letsencrypt/live/yourdomain.com/`

## Перед каждыми выборами

- Сгенерировать новый `VOTE_TOKEN_SECRET` (`openssl rand -hex 32`) до `/start_voting`: иначе делегат получит тот же токен бюллетеня, что и в прошлые выборы. После `/start_voting` ключ не менять (выданные токены станут недействительны).

## Диагностика: бот молчит

1. `make webhook_info` — смотреть `url`, `pending_update_count`, `last_error_message` (`403 Forbidden` = secret_token в Telegram не совпадает с `WEBHOOK_SECRET` бота; `301` = URL без завершающего `/`).
2. После смены `WEBHOOK_SECRET` в `.env` перезапустить бота и выполнить `make webhook-prod` (`setup-domain.sh` тоже всегда переустанавливает webhook).
3. `make webhook-prod DROP_PENDING=true` сбрасывает очередь необработанных update — только до начала голосования, иначе пропадут нажатия кнопок.
4. `docker compose logs bot nginx` — 403 на webhook пишется в лог бота.
5. Делегату не пришло письмо с кодом (или исчерпан лимит: 2 письма в сутки на аккаунт, 10 на делегата): в админ-чате `/show_code 123456` покажет ожидающий код, его можно передать делегату вручную.
