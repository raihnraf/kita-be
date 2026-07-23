package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
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
	if err := run(); err != nil {
		logger.Error("book worker encountered fatal error", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info("starting book worker")

	db, err := database.NewPool(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	rmqConn, err := rabbitmq.ConnectWithRetry(cfg.RabbitMQURL, 30, 2*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}
	defer rmqConn.Close()

	rmqPublisher := rabbitmq.NewPublisher(rmqConn)
	consumer := rabbitmq.NewConsumer(rmqConn, rabbitmq.CommandQueueName)
	if err := consumer.Setup(); err != nil {
		return fmt.Errorf("failed to setup consumer topology: %w", err)
	}

	bookRepo := bookrepo.NewBookRepository(db)
	stockUC := usecase.NewStockUsecase(bookRepo)
	resultPublisher := bookmsg.NewPublisher(rmqPublisher)
	handler := bookmsg.NewHandler(stockUC, resultPublisher)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		logger.Info("book worker running", "queue", rabbitmq.CommandQueueName)
		consumer.ConsumeWithReconnect(ctx, handler.HandleStockEvent)
		logger.Info("consumer stopped")
	}()

	<-ctx.Done()
	// Restore default signal behavior so a second signal force-kills the process.
	stop()
	logger.Info("shutting down book worker")

	wg.Wait()
	logger.Info("book worker stopped cleanly")
	return nil
}
