package main

import (
	"fmt"
	"os"
	"time"

	authmw "kita-be/internal/auth/middleware"
	bookhttp "kita-be/internal/book/delivery/http"
	bookrepo "kita-be/internal/book/repository/postgres"
	"kita-be/internal/book/usecase"
	"kita-be/internal/platform/config"
	"kita-be/internal/platform/database"
	"kita-be/internal/platform/httpserver"
	"kita-be/internal/platform/logger"
	platformmw "kita-be/internal/platform/middleware"
)

func main() {
	if err := run(); err != nil {
		logger.Error("book service encountered fatal error", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info("starting book service")

	db, err := database.NewPool(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
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

	httpserver.RegisterHealthRoutes(app, "/api/v1", "book-service",
		httpserver.ReadinessCheck{Name: "database", Check: db.Ping},
	)

	api := app.Group("/api/v1")
	readLimiter := platformmw.RateLimit(300, time.Minute)
	writeLimiter := platformmw.RateLimit(60, time.Minute)

	api.Get("/books", readLimiter, handler.List)
	api.Get("/books/:id", readLimiter, handler.Get)
	api.Get("/books/:id/availability", readLimiter, handler.Availability)

	admin := api.Group("", authmw.InternalAuth(cfg.InternalAPIToken))
	admin.Post("/books", writeLimiter, handler.Create)
	admin.Put("/books/:id", writeLimiter, handler.Update)

	internal := api.Group("/internal", authmw.InternalAuth(cfg.InternalAPIToken))
	internal.Post("/books/:id/stock/decrease", writeLimiter, handler.InternalDecreaseStock)
	internal.Post("/books/:id/stock/increase", writeLimiter, handler.InternalIncreaseStock)

	return httpserver.ListenAndServeWithGracefulShutdown(app, cfg.ServerPort, "book service", 10*time.Second)
}
