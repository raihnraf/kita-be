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
	pwdsvc "kita-be/internal/auth/password"
	identityhttp "kita-be/internal/identity/delivery/http"
	idrepo "kita-be/internal/identity/repository/postgres"
	"kita-be/internal/identity/usecase"
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

	logger.Info("starting identity service")

	db, err := database.NewPool(cfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	jwtService := jwtsvc.NewService(cfg.JWTSecret, cfg.JWTExpiry, cfg.RefreshTokenExpiry)
	pwdService := pwdsvc.NewService()

	userRepo := idrepo.NewUserRepository(db)
	refreshTokenRepo := idrepo.NewRefreshTokenRepository(db)

	registerUC := usecase.NewRegisterUsecase(userRepo, pwdService, jwtService)
	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdService, jwtService)
	refreshUC := usecase.NewRefreshUsecase(userRepo, refreshTokenRepo, jwtService)
	logoutUC := usecase.NewLogoutUsecase(userRepo, refreshTokenRepo)
	profileUC := usecase.NewProfileUsecase(userRepo)

	handler := identityhttp.NewIdentityHandler(registerUC, loginUC, refreshUC, logoutUC, profileUC)

	app := httpserver.New()

	app.Get("/api/v1/health", func(c *fiber.Ctx) error {
		return response.OK(c, fiber.Map{
			"service": "identity-service",
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
			"service": "identity-service",
			"status":  "ready",
		})
	})

	api := app.Group("/api/v1")
	authLimiter := platformmw.RateLimit(10, time.Minute)

	api.Post("/auth/register", authLimiter, handler.Register)
	api.Post("/auth/token", authLimiter, handler.Token)
	api.Post("/auth/logout", authLimiter, handler.Logout)

	protected := api.Group("", authmw.JWTAuth(jwtService))
	protected.Get("/users/me", handler.Profile)

	go func() {
		addr := fmt.Sprintf(":%s", cfg.ServerPort)
		if err := app.Listen(addr); err != nil {
			logger.Error("server failed", "error", err.Error())
			os.Exit(1)
		}
	}()

	logger.Info("identity service listening", "port", cfg.ServerPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down identity service")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err.Error())
	}

	logger.Info("identity service stopped")
}
