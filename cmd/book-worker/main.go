package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	bookmsg "kita-be/internal/book/messaging"
	bookrepo "kita-be/internal/book/repository/postgres"
	"kita-be/internal/book/usecase"
	"kita-be/internal/platform/config"
	"kita-be/internal/platform/database"
	"kita-be/internal/platform/logger"
	"kita-be/internal/platform/rabbitmq"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger.Info("starting book worker")

	db, err := database.NewPool(cfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	rmqConn, err := connectRabbitMQWithRetry(cfg.RabbitMQURL, 30, 2*time.Second)
	if err != nil {
		logger.Error("failed to connect to rabbitmq", "error", err.Error())
		os.Exit(1)
	}
	defer rmqConn.Close()

	consumer := rabbitmq.NewConsumer(rmqConn, rabbitmq.QueueName)
	if err := consumer.Setup(); err != nil {
		logger.Error("failed to setup consumer topology", "error", err.Error())
		os.Exit(1)
	}

	bookRepo := bookrepo.NewBookRepository(db)
	stockUC := usecase.NewStockUsecase(bookRepo)
	handler := bookmsg.NewHandler(stockUC)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())

	// ConsumeWithReconnect blocks; runs reconnect loop internally on broker disconnect.
	go func() {
		logger.Info("book worker running", "queue", rabbitmq.QueueName)
		consumer.ConsumeWithReconnect(ctx, handler.HandleStockEvent)
		logger.Info("consumer stopped")
		// If the consumer exits on its own (exhausted reconnect attempts), signal shutdown.
		quit <- syscall.SIGTERM
	}()

	<-quit
	logger.Info("shutting down book worker")
	cancel()
	logger.Info("book worker stopped")
}

func connectRabbitMQWithRetry(url string, attempts int, delay time.Duration) (*rabbitmq.Connection, error) {
	var lastErr error
	for i := 1; i <= attempts; i++ {
		conn, err := rabbitmq.NewConnection(url)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		logger.Warn("rabbitmq connection attempt failed", "attempt", i, "max_attempts", attempts, "error", err.Error())
		time.Sleep(delay)
	}
	return nil, lastErr
}
