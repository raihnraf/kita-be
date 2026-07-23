package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	jwtsvc "kita-be/internal/auth/jwt"
	authmw "kita-be/internal/auth/middleware"
	"kita-be/internal/platform/config"
	"kita-be/internal/platform/database"
	"kita-be/internal/platform/httpserver"
	"kita-be/internal/platform/logger"
	platformmw "kita-be/internal/platform/middleware"
	"kita-be/internal/platform/rabbitmq"
	bookclient "kita-be/internal/transaction/client/book"
	txnhttp "kita-be/internal/transaction/delivery/http"
	txnmsg "kita-be/internal/transaction/messaging"
	txnrepo "kita-be/internal/transaction/repository/postgres"
	"kita-be/internal/transaction/usecase"
)

func main() {
	if err := run(); err != nil {
		logger.Error("transaction service encountered fatal error", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info("starting transaction service")

	db, err := database.NewPool(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	serviceCtx, serviceCancel := context.WithCancel(context.Background())
	defer serviceCancel()

	jwtService := jwtsvc.NewService(cfg.JWTSecret, cfg.JWTExpiry, cfg.RefreshTokenExpiry)

	if cfg.BookServiceURL == "" {
		return fmt.Errorf("BOOK_SERVICE_URL is required for transaction service")
	}

	bookClient := bookclient.NewClient(cfg.BookServiceURL, cfg.InternalAPIToken)

	txnRepo := txnrepo.NewTransactionRepository(db)
	auditRepo := txnrepo.NewAuditRepository(db)
	idempotencyRepo := txnrepo.NewIdempotencyRepository(db)
	outboxRepo := txnrepo.NewStockEventOutboxRepository(db)

	fineCalc := usecase.NewFineCalculator(cfg.DailyFineAmountCents)

	borrowUC := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempotencyRepo, bookClient, cfg.MaxActiveBorrows, cfg.LoanDays)
	returnUC := usecase.NewReturnUsecase(txnRepo, auditRepo, idempotencyRepo, fineCalc)
	historyUC := usecase.NewHistoryUsecase(txnRepo, auditRepo)

	var wg sync.WaitGroup

	var rmqConn *rabbitmq.Connection
	if cfg.RabbitMQURL == "" {
		logger.Info("rabbitmq stock event outbox dispatcher disabled")
	} else if conn, err := rabbitmq.ConnectWithRetry(cfg.RabbitMQURL, 30, 2*time.Second); err != nil {
		logger.Warn("rabbitmq not available, running without async stock events", "error", err.Error())
	} else {
		rmqConn = conn
		defer rmqConn.Close()

		rmqPublisher := rabbitmq.NewPublisher(rmqConn)
		if err := rmqPublisher.Setup(); err != nil {
			logger.Warn("failed to setup rabbitmq publisher", "error", err.Error())
		} else {
			msgPublisher := txnmsg.NewPublisher(rmqPublisher)
			dispatcher := txnmsg.NewOutboxDispatcher(outboxRepo, msgPublisher, 2*time.Second, 10)
			resultConsumer := rabbitmq.NewConsumer(rmqConn, rabbitmq.ResultQueueName)
			resultHandler := txnmsg.NewResultHandler(txnRepo, auditRepo)
			wg.Add(2)
			go func() {
				defer wg.Done()
				dispatcher.Run(serviceCtx)
			}()
			go func() {
				defer wg.Done()
				resultConsumer.ConsumeWithReconnect(serviceCtx, resultHandler.HandleStockResult)
			}()
			logger.Info("rabbitmq stock event outbox dispatcher enabled")
		}
	}

	reconciler := txnmsg.NewReconciliationWorker(txnRepo, 30*time.Second, 1*time.Minute)
	wg.Add(1)
	go func() {
		defer wg.Done()
		reconciler.Run(serviceCtx)
	}()

	handler := txnhttp.NewTransactionHandler(borrowUC, returnUC, historyUC)

	app := httpserver.New()

	readinessChecks := []httpserver.ReadinessCheck{
		{Name: "database", Check: db.Ping},
		{Name: "book service", Check: bookClient.Ready},
	}
	if cfg.RabbitMQURL != "" {
		readinessChecks = append(readinessChecks, httpserver.ReadinessCheck{
			Name: "rabbitmq",
			Check: func(_ context.Context) error {
				if rmqConn == nil || !rmqConn.IsConnected() {
					return errors.New("rabbitmq is not connected")
				}
				return nil
			},
		})
	}
	httpserver.RegisterHealthRoutes(app, "/api/v1", "transaction-service", readinessChecks...)

	api := app.Group("/api/v1")
	readLimiter := platformmw.RateLimit(120, time.Minute)
	writeLimiter := platformmw.RateLimit(30, time.Minute)

	protected := api.Group("", authmw.JWTAuth(jwtService))
	protected.Post("/transactions/borrow", writeLimiter, handler.Borrow)
	protected.Post("/transactions/:id/return", writeLimiter, handler.Return)
	protected.Get("/transactions/history", readLimiter, handler.History)
	protected.Get("/transactions/active", readLimiter, handler.Active)
	protected.Get("/transactions/:id", readLimiter, handler.Detail)

	internal := api.Group("/internal", authmw.InternalAuth(cfg.InternalAPIToken))
	internal.Get("/transactions", readLimiter, handler.InternalTransactions)
	internal.Get("/transactions/:id/audits", readLimiter, handler.InternalTransactionAudits)
	internal.Get("/transactions/:id", readLimiter, handler.InternalTransactionDetail)

	err = httpserver.ListenAndServeWithGracefulShutdown(app, cfg.ServerPort, "transaction service", 10*time.Second)

	// Stop background workers and wait for them to finish before the deferred
	// cleanup closes the RabbitMQ connection and database pool.
	serviceCancel()
	wg.Wait()

	return err
}
