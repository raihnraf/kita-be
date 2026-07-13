package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"

	jwtsvc "kita-be/internal/auth/jwt"
	authmw "kita-be/internal/auth/middleware"
	"kita-be/internal/platform/config"
	"kita-be/internal/platform/database"
	"kita-be/internal/platform/httpserver"
	"kita-be/internal/platform/logger"
	platformmw "kita-be/internal/platform/middleware"
	"kita-be/internal/platform/rabbitmq"
	"kita-be/internal/platform/response"
	bookclient "kita-be/internal/transaction/client/book"
	txnhttp "kita-be/internal/transaction/delivery/http"
	txnmsg "kita-be/internal/transaction/messaging"
	txnrepo "kita-be/internal/transaction/repository/postgres"
	"kita-be/internal/transaction/usecase"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger.Info("starting transaction service")

	db, err := database.NewPool(cfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	jwtService := jwtsvc.NewService(cfg.JWTSecret, cfg.JWTExpiry, cfg.RefreshTokenExpiry)

	if cfg.BookServiceURL == "" {
		logger.Error("BOOK_SERVICE_URL is required for transaction service")
		os.Exit(1)
	}

	bookClient := bookclient.NewClient(cfg.BookServiceURL, cfg.InternalAPIToken)

	txnRepo := txnrepo.NewTransactionRepository(db)
	auditRepo := txnrepo.NewAuditRepository(db)
	idempotencyRepo := txnrepo.NewIdempotencyRepository(db)

	fineCalc := usecase.NewFineCalculator(cfg.DailyFineAmountCents)

	borrowUC := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempotencyRepo, bookClient, cfg.MaxActiveBorrows, cfg.LoanDays)
	returnUC := usecase.NewReturnUsecase(txnRepo, auditRepo, idempotencyRepo, bookClient, fineCalc)
	historyUC := usecase.NewHistoryUsecase(txnRepo, auditRepo)

	if cfg.RabbitMQURL == "" {
		logger.Info("rabbitmq stock event publisher disabled")
	} else if rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQURL); err != nil {
		logger.Warn("rabbitmq not available, running without async stock events", "error", err.Error())
	} else {
		defer rmqConn.Close()

		rmqPublisher := rabbitmq.NewPublisher(rmqConn)
		if err := rmqPublisher.Setup(); err != nil {
			logger.Warn("failed to setup rabbitmq publisher", "error", err.Error())
		} else {
			msgPublisher := txnmsg.NewPublisher(rmqPublisher)
			borrowUC.SetEventPublisher(msgPublisher)
			returnUC.SetEventPublisher(msgPublisher)
			logger.Info("rabbitmq stock event publisher enabled")
		}
	}

	handler := txnhttp.NewTransactionHandler(borrowUC, returnUC, historyUC)

	app := httpserver.New()

	app.Get("/api/v1/health", func(c *fiber.Ctx) error {
		return response.OK(c, fiber.Map{
			"service": "transaction-service",
			"status":  "healthy",
		})
	})

	app.Get("/api/v1/ready", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			return response.Error(c, fiber.StatusServiceUnavailable, "NOT_READY", "database is not reachable")
		}
		if err := bookClient.Ready(ctx); err != nil {
			return response.Error(c, fiber.StatusServiceUnavailable, "NOT_READY", "book service is not reachable")
		}

		return response.OK(c, fiber.Map{
			"service": "transaction-service",
			"status":  "ready",
		})
	})

	api := app.Group("/api/v1")
	writeLimiter := platformmw.RateLimit(30, time.Minute)

	protected := api.Group("", authmw.JWTAuth(jwtService))
	protected.Post("/transactions/borrow", writeLimiter, handler.Borrow)
	protected.Post("/transactions/:id/return", writeLimiter, handler.Return)
	protected.Get("/transactions/history", handler.History)
	protected.Get("/transactions/active", handler.Active)
	protected.Get("/transactions/:id", handler.Detail)

	internal := api.Group("/internal", authmw.InternalAuth(cfg.InternalAPIToken))
	internal.Get("/transactions", handler.InternalTransactions)
	internal.Get("/transactions/:id/audits", handler.InternalTransactionAudits)
	internal.Get("/transactions/:id", handler.InternalTransactionDetail)

	go func() {
		addr := fmt.Sprintf(":%s", cfg.ServerPort)
		if err := app.Listen(addr); err != nil {
			logger.Error("server failed", "error", err.Error())
			os.Exit(1)
		}
	}()

	logger.Info("transaction service listening", "port", cfg.ServerPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down transaction service")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err.Error())
	}

	logger.Info("transaction service stopped")
}
