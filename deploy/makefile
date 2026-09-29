SHELL := /bin/bash
# без .env работают run/mock/test; guards ниже требуют непустых значений
-include .env

# drop_pending_updates при setWebhook: make webhook-prod DROP_PENDING=true —
# только до начала голосования, во время него теряются нажатия кнопок в очереди
ifeq ($(strip $(DROP_PENDING)),)
DROP_PENDING := false
endif

# goose ходит в опубликованный порт postgres (127.0.0.1, см. docker-compose.yml)
GOOSE_DSN = postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@127.0.0.1:5432/$(POSTGRES_DB)?sslmode=disable

# make оставляет кавычки из .env как есть: одинарные проходят shell-guard в '...'
# и попадают в DSN/JSON, поэтому проверка на уровне make, до запуска shell
define noquotes
$(foreach v,$(1),$(if $(findstring ',$($(v)))$(findstring ",$($(v))),$(error $(v): значение без кавычек)))
endef

# quick start app with first build and dependencies
app:
	docker compose up -d bot --build

# dev: БД → миграции → тестовые данные → bot → ngrok (profile dev)
app2: up-db migrate migrate-dev app ngrok-docker
	@echo -e "\033[32mAll containers started successfully\033[0m"

# Production with domain (instead of ngrok): БД → миграции (без тестовых данных) → bot, frontend, nginx
app-prod: up-db migrate
	docker compose up -d --build postgres bot frontend nginx
	@echo -e "\033[32mProduction app started with domain\033[0m"

app-prod-bot:
	docker compose up -d bot --build

# Domain setup
setup-domain:
	./scripts/setup-domain.sh

# Guards. Регэкспы те же, что в scripts/setup-domain.sh.
# Длина отдельно: {32,256} не проходит через BSD grep (RE_DUP_MAX=255)
check-token:
	@$(call noquotes,TELEGRAM_APITOKEN)
	@printf '%s\n' '$(TELEGRAM_APITOKEN)' | grep -Eq '^[0-9]+:[A-Za-z0-9_-]{30,}$$' || { echo 'TELEGRAM_APITOKEN: ожидается 123456:ABC-DEF..., без кавычек' >&2; exit 1; }

# '#' в значении make читает как комментарий, compose и Go — как часть значения: проверяем сырой .env
check-env-hash:
	@! grep -qE '^(TELEGRAM_APITOKEN|WEBHOOK_SECRET|DOMAIN|POSTGRES_USER|POSTGRES_PASSWORD|POSTGRES_DB)=[^#]*#' .env 2>/dev/null || { echo '.env: символ # внутри значения недопустим' >&2; exit 1; }

check-webhook-env: check-token check-env-hash
	@$(call noquotes,WEBHOOK_SECRET DROP_PENDING)
	@s='$(WEBHOOK_SECRET)'; printf '%s\n' "$$s" | grep -Eq '^[A-Za-z0-9_-]+$$' && test $${#s} -ge 32 -a $${#s} -le 256 || { echo 'WEBHOOK_SECRET: 32..256 символов A-Za-z0-9_-, без кавычек' >&2; exit 1; }
	@test '$(DROP_PENDING)' = true -o '$(DROP_PENDING)' = false || { echo 'DROP_PENDING: true или false' >&2; exit 1; }

check-domain:
	@$(call noquotes,DOMAIN)
	@printf '%s\n' '$(DOMAIN)' | grep -Eq '^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$$' || { echo 'DOMAIN: ожидается имя хоста (example.org), без кавычек' >&2; exit 1; }

# Пароль и имена уходят в URL без экранирования: только URL-safe символы,
# иначе make, compose, bash и Go прочитают .env по-разному
check-db-env: check-env-hash
	@$(call noquotes,POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB)
	@for v in POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB; do \
		case $$v in \
			POSTGRES_USER) val='$(POSTGRES_USER)';; \
			POSTGRES_PASSWORD) val='$(POSTGRES_PASSWORD)';; \
			POSTGRES_DB) val='$(POSTGRES_DB)';; \
		esac; \
		printf '%s\n' "$$val" | grep -Eq '^[A-Za-z0-9_.~-]+$$' || { echo "$$v: непустое значение только из A-Za-z0-9_.~- (без кавычек)" >&2; exit 1; }; \
	done

# Production webhook
# URL со слэшем: без него nginx отвечает 301, а Telegram редиректы не следует.
# setWebhook сам заменяет старый webhook, deleteWebhook перед ним не нужен.
webhook-prod: check-webhook-env check-domain
	@printf '%s' '{"url": "https://$(DOMAIN)/election_bot/", "secret_token": "$(WEBHOOK_SECRET)", "allowed_updates": ["message", "callback_query"], "drop_pending_updates": $(DROP_PENDING)}' \
		| curl -sS --fail-with-body --request POST --url "https://api.telegram.org/bot$(TELEGRAM_APITOKEN)/setWebhook" --header 'content-type: application/json' --data @- && echo

run:
	cd .. && go run ./cmd/main.go

# Build docker image
build:
	docker compose build
stop:
	docker compose stop
start:
	docker compose start
down:
	docker compose down

# Run the postgres container (--wait: до healthcheck, иначе goose стартует раньше БД)
up-db:
	docker compose up -d --wait postgres
stop-db:
	docker compose stop postgres
start-db:
	docker compose start postgres
down-db:
	docker compose down postgres

# Миграции: схема (prod и dev) и тестовые данные (только dev, после migrate)
migrate: check-db-env
	goose -dir ./migrations postgres "$(GOOSE_DSN)" up
migrate-dev: check-db-env
	goose -dir ./migrations_dev postgres "$(GOOSE_DSN)" up

# Откат всех миграций: Down у create_election_tables удаляет все таблицы вместе с данными.
# goose reset по объединённому каталогу: тестовые данные (если накатывались) откатываются
# первыми, иначе goose не найдёт файл текущей версии и упадёт
drop-schema: check-db-env
	@test '$(CONFIRM)' = yes || { echo 'drop-schema удаляет все таблицы БД $(POSTGRES_DB) (goose reset). Запуск: make drop-schema CONFIRM=yes' >&2; exit 1; }
	@echo 'drop-schema: goose reset — откат тестовых данных (если есть) и схемы БД $(POSTGRES_DB), все таблицы будут удалены'
	@d=$$(mktemp -d) && cp ./migrations/*.sql "$$d" && { cp ./migrations_dev/*.sql "$$d" 2>/dev/null || true; } && goose -dir "$$d" postgres "$(GOOSE_DSN)" reset; rc=$$?; rm -rf "$$d"; exit $$rc

# Run tests
test:
	cd .. && go test ./...
test-integration:
	cd .. && go test -tags=integration ./...
# Clean database after tests
clean-db: check-db-env
	docker exec -i postgres psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) -c "TRUNCATE TABLE votes, results, delegates, candidates RESTART IDENTITY CASCADE;"

# Telegram bot webhook (webhook_info — первый шаг диагностики: last_error_message, pending_update_count)
webhook_info: check-token
	@curl -sS --fail-with-body --request POST --url "https://api.telegram.org/bot$(TELEGRAM_APITOKEN)/getWebhookInfo" && echo
webhook_delete: check-token
	@curl -sS --fail-with-body --request POST --url "https://api.telegram.org/bot$(TELEGRAM_APITOKEN)/deleteWebhook" && echo
webhook_create: check-webhook-env
	@$(call noquotes,NGROK_URL)
	@test -n "$(NGROK_URL)" || { echo 'NGROK_URL не задан' >&2; exit 1; }
	@printf '%s' '{"url": "https://$(NGROK_URL)/", "secret_token": "$(WEBHOOK_SECRET)", "allowed_updates": ["message", "callback_query"], "drop_pending_updates": $(DROP_PENDING)}' \
		| curl -sS --fail-with-body --request POST --url "https://api.telegram.org/bot$(TELEGRAM_APITOKEN)/setWebhook" --header 'content-type: application/json' --data @- && echo

# Ngrok run (docker: сервис ngrok из docker-compose.yml, profile dev)
ngrok-native:
	ngrok http --domain $(NGROK_URL) $(NGROK_PORT)
ngrok-docker:
	docker compose --profile dev up -d ngrok

# Сравнить ключи .env и .env.example (ничего не перезаписывает)
envx:
	@diff <(grep -o '^[A-Z_]*=' .env | sort) <(grep -o '^[A-Z_]*=' .env.example | sort) && echo '.env и .env.example содержат одинаковые ключи'

exportmove:
	export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin

gooseins:
	go install github.com/pressly/goose/v3/cmd/goose@latest

mock:
	cd .. && mockgen -source=internal/bot/bot.go -destination=internal/bot/mocks/mock_chain_schulze.go -package=bot
	cd .. && mockgen -source=internal/chain/init.go -destination=internal/chain/mocks/mock_storage.go -package=chain
	cd .. && mockgen -source=internal/schulze/init.go -destination=internal/schulze/mocks/mock_chain.go -package=schulze

.PHONY: app app2 app-prod app-prod-bot setup-domain check-token check-env-hash check-webhook-env check-domain check-db-env \
	webhook-prod run build stop start down up-db stop-db start-db down-db migrate migrate-dev drop-schema \
	test test-integration clean-db webhook_info webhook_delete webhook_create ngrok-native ngrok-docker \
	envx exportmove gooseins mock
