package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"vote_system/internal/bot"
	"vote_system/internal/chain"
	"vote_system/internal/db"
	"vote_system/internal/schulze"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {

	// Читаем переменную окружения с URL базы данных
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Инициализируем структуру Storage и UserChain
	storage, err := db.NewStorage(dsn)
	if err != nil {
		log.Fatalf("unable to connect Storage: %v", err)
	}
	defer storage.Close()

	voteChain := chain.NewVoteChain(storage)

	// Читаем токен для Telegram бота
	token := os.Getenv("TELEGRAM_APITOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	// Инициализируем бота
	botAPI, err := tgbotapi.NewBotAPI(token)
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

	// Устанавливаем обработчик для обновлений
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		botHandler.HandleWebhook(w, r)
	})
	http.HandleFunc("/results", func(w http.ResponseWriter, r *http.Request) {
		//TODO
	})

	// Запускаем HTTP сервер
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{Addr: ":" + port}

	go func() {
		log.Infof("Starting server on port %s", port)
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
