# Deployment Configuration

Эта папка содержит конфигурации и скрипты для развертывания приложения.

## Структура

```
deploy/
├── configs/           # Конфигурационные файлы
│   └── nginx.conf     # Конфигурация Nginx для production
├── scripts/           # Скрипты автоматизации
│   └── setup-domain.sh # Настройка домена и SSL
├── docker-compose.yml # Docker Compose для production
├── docker-compose.ngrok.yml # Docker Compose для разработки с ngrok
├── dockerfile         # Dockerfile для сборки приложения
├── makefile           # Makefile с командами для деплоя
├── .env               # Переменные окружения
├── .env.example       # Пример переменных окружения
└── README.md          # Этот файл
```

## Использование

### Production (с доменом)
```bash
# Настройка домена и SSL
./scripts/setup-domain.sh

# Запуск в production
docker-compose up -d
```

### Development (с ngrok)
```bash
# Запуск с ngrok для разработки
docker-compose -f docker-compose.ngrok.yml up -d
```

## SSL Сертификаты

SSL сертификаты автоматически получаются через Let's Encrypt и обновляются каждые 12 часов через cron.

Сертификаты хранятся в `/etc/letsencrypt/live/yourdomain.com/`
