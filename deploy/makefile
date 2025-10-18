include .env

# quick start app with first build and dependencies
app:
	docker-compose up -d bot --build

app2: up-db app migrate ngrok-docker
	@echo -e "\033[32mAll containers started successfully\033[0m"

# Production with domain (instead of ngrok)
app-prod: up-db app-prod-bot migrate
	@echo -e "\033[32mProduction app started with domain\033[0m"

app-prod-bot:
	docker-compose up -d bot --build

# Domain setup
setup-domain:
	./scripts/setup-domain.sh

# Production webhook
webhook-prod: webhook_delete
	curl --request POST --url "https://api.telegram.org/bot$(TELEGRAM_APITOKEN)/setWebhook" --header 'content-type: application/json' --data '{"url": "https://$(DOMAIN)/election_bot"}'

run:
	cd .. && go run ./cmd/main.go

# Build docker image
build:
	docker-compose build
stop:
	docker-compose stop
start:
	docker-compose start
down:
	docker-compose down

# Run the postgres container
up-db:
	docker-compose up -d postgres
stop-db:
	docker-compose stop postgres
start-db:
	docker-compose start postgres
down-db:
	docker-compose down postgres

# Run migrations
migrate:
	goose -dir ./migrations postgres "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5432/${POSTGRES_DB}?sslmode=disable" up
unmigrate:
	goose -dir ./migrations postgres "postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5432/${POSTGRES_DB}?sslmode=disable" down


# Run tests
test:
	cd .. && go test ./...
test-integration:
	cd .. && go test -tags=integration ./...
# Clean database after tests
clean-db:
	docker exec -it postgres psql -U ${POSTGRES_USER} -d ${POSTGRES_DB} -c "TRUNCATE TABLE users RESTART IDENTITY CASCADE;"

# Telegram bot webhook
webhook_info:
	curl --request POST --url "https://api.telegram.org/bot$(TELEGRAM_APITOKEN)/getMe"
webhook_delete:
	curl --request POST --url "https://api.telegram.org/bot$(TELEGRAM_APITOKEN)/deleteWebhook"
webhook_create: webhook_delete
	curl --request POST --url "https://api.telegram.org/bot$(TELEGRAM_APITOKEN)/setWebhook" --header 'content-type: application/json' --data '{"url": "https://$(NGROK_URL)"}'

# Ngrok run
ngrok-native:
	ngrok http --domain $(NGROK_URL) $(NGROK_PORT)
ngrok-docker:
	docker-compose -f docker-compose.ngrok.yml up -d ngrok

envx:
	$(shell sed 's/=.*/=/' .env > .env.example)

exportmove:
	export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin

gooseins:
	go install github.com/pressly/goose/v3/cmd/goose@latest

mock:
	cd .. && mockgen -source=internal/bot/bot.go -destination=internal/bot/mocks/mock_chain_schulze.go -package=bot
	cd .. && mockgen -source=internal/chain/init.go -destination=internal/chain/mocks/mock_storage.go -package=chain
	cd .. && mockgen -source=internal/schulze/init.go -destination=internal/schulze/mocks/mock_chain.go -package=schulze