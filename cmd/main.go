package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/lsdpls/schulze_election_telegram_bot/internal/api"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/bot"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/chain"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/config"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/db"
	"github.com/lsdpls/schulze_election_telegram_bot/internal/schulze"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Таймауты запросов к Telegram Bot API: ответ — не дольше telegramResponseTimeout после отправки запроса;
// весь запрос вместе с загрузкой файла (/csv, /send_logs) — не дольше telegramRequestTimeout
const (
	telegramResponseTimeout = 15 * time.Second
	telegramRequestTimeout  = 2 * time.Minute
)

func main() {
	// Загружаем конфигурацию
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Инициализируем структуру Storage и UserChain
	storage, err := db.NewStorage(config.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect Storage: %v", err)
	}
	defer storage.Close()

	voteChain := chain.NewVoteChain(storage)

	// Инициализируем бота. У HTTP-клиента есть таймауты: без них один зависший запрос к Telegram
	// держал бы обработку этого пользователя (а раньше — всех) бесконечно
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = telegramResponseTimeout
	botAPI, err := tgbotapi.NewBotAPIWithClient(config.TelegramAPIToken, tgbotapi.APIEndpoint,
		&http.Client{Timeout: telegramRequestTimeout, Transport: transport})
	if err != nil {
		log.Fatalf("unable to connect botAPI: %v", err)
	}

	// Логируем подключение
	botAPI.Debug = false
	log.Infof("Authorized on account %s", botAPI.Self.UserName)

	schulze := schulze.NewSchulze(voteChain)

	// Инициализация объекта бота
	botHandler := bot.NewBot(botAPI, voteChain, schulze)
	defer botHandler.Close()

	// Инициализируем API handler
	apiHandler := api.NewHandler(voteChain)

	// Health check должен быть ПЕРЕД catch-all обработчиком
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})
	http.HandleFunc("/votes", apiHandler.GetVotes)
	http.HandleFunc("/candidates", apiHandler.GetCandidates)
	http.HandleFunc("/result", apiHandler.GetResults)
	// Catch-all обработчик для webhook (должен быть последним)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		botHandler.HandleWebhook(w, r)
	})

	// Запускаем HTTP сервер
	srv := &http.Server{Addr: ":" + config.AppPort}

	go func() {
		log.Infof("Starting server on port %s", config.AppPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	// Обрабатываем системные сигналы для корректного завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	log.Infoln("Shutting down bot...")
	botAPI.StopReceivingUpdates()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Error shutting down server: %v", err)
	}
	defer log.Infoln("Gracefully shutdown bot")
}
