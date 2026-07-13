package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"

	authmw "kita-be/internal/auth/middleware"
	bookhttp "kita-be/internal/book/delivery/http"
	bookrepo "kita-be/internal/book/repository/postgres"
	"kita-be/internal/book/usecase"
	"kita-be/internal/platform/config"
	"kita-be/internal/platform/database"
	"kita-be/internal/platform/httpserver"
	"kita-be/internal/platform/logger"
	platformmw "kita-be/internal/platform/middleware"
	"kita-be/internal/platform/response"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger.Info("starting book service")

	db, err := database.NewPool(cfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	bookRepo := bookrepo.NewBookRepository(db)

	listBooksUC := usecase.NewListBooksUsecase(bookRepo)
	getBookUC := usecase.NewGetBookUsecase(bookRepo)
	createBookUC := usecase.NewCreateBookUsecase(bookRepo)
	updateBookUC := usecase.NewUpdateBookUsecase(bookRepo)
	stockUC := usecase.NewStockUsecase(bookRepo)

	handler := bookhttp.NewBookHandler(listBooksUC, getBookUC, createBookUC, updateBookUC, stockUC)

	app := httpserver.New()

	app.Get("/api/v1/health", func(c *fiber.Ctx) error {
		return response.OK(c, fiber.Map{
			"service": "book-service",
			"status":  "healthy",
		})
	})

	app.Get("/api/v1/ready", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			return response.Error(c, fiber.StatusServiceUnavailable, "NOT_READY", "database is not reachable")
		}

		return response.OK(c, fiber.Map{
			"service": "book-service",
			"status":  "ready",
		})
	})

	api := app.Group("/api/v1")
	writeLimiter := platformmw.RateLimit(60, time.Minute)

	api.Get("/books", handler.List)
	api.Get("/books/:id", handler.Get)
	api.Get("/books/:id/availability", handler.Availability)

	admin := api.Group("", authmw.InternalAuth(cfg.InternalAPIToken))
	admin.Post("/books", writeLimiter, handler.Create)
	admin.Put("/books/:id", writeLimiter, handler.Update)

	internal := api.Group("/internal", authmw.InternalAuth(cfg.InternalAPIToken))
	internal.Post("/books/:id/stock/decrease", writeLimiter, handler.InternalDecreaseStock)
	internal.Post("/books/:id/stock/increase", writeLimiter, handler.InternalIncreaseStock)

	go func() {
		addr := fmt.Sprintf(":%s", cfg.ServerPort)
		if err := app.Listen(addr); err != nil {
			logger.Error("server failed", "error", err.Error())
			os.Exit(1)
		}
	}()

	logger.Info("book service listening", "port", cfg.ServerPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down book service")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err.Error())
	}

	logger.Info("book service stopped")
}
