package main

import (
	"fmt"
	"os"
	"time"

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
)

func main() {
	if err := run(); err != nil {
		logger.Error("identity service encountered fatal error", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info("starting identity service")

	db, err := database.NewPool(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	jwtService := jwtsvc.NewService(cfg.JWTSecret, cfg.JWTExpiry, cfg.RefreshTokenExpiry)
	pwdService := pwdsvc.NewService()

	userRepo := idrepo.NewUserRepository(db)
	refreshTokenRepo := idrepo.NewRefreshTokenRepository(db)

	registerUC := usecase.NewRegisterUsecase(userRepo, refreshTokenRepo, pwdService, jwtService)
	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdService, jwtService)
	refreshUC := usecase.NewRefreshUsecase(userRepo, refreshTokenRepo, jwtService)
	logoutUC := usecase.NewLogoutUsecase(userRepo, refreshTokenRepo)
	profileUC := usecase.NewProfileUsecase(userRepo)

	handler := identityhttp.NewIdentityHandler(registerUC, loginUC, refreshUC, logoutUC, profileUC)

	app := httpserver.New()

	httpserver.RegisterHealthRoutes(app, "/api/v1", "identity-service",
		httpserver.ReadinessCheck{Name: "database", Check: db.Ping},
	)

	api := app.Group("/api/v1")
	authLimiter := platformmw.RateLimit(10, time.Minute)

	api.Post("/auth/register", authLimiter, handler.Register)
	api.Post("/auth/token", authLimiter, handler.Token)
	api.Post("/auth/logout", authLimiter, handler.Logout)

	protected := api.Group("", authmw.JWTAuth(jwtService))
	protected.Get("/users/me", handler.Profile)

	return httpserver.ListenAndServeWithGracefulShutdown(app, cfg.ServerPort, "identity service", 10*time.Second)
}
