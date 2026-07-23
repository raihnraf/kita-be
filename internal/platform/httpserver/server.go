package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"

	"kita-be/internal/platform/apperror"
	"kita-be/internal/platform/logger"
	"kita-be/internal/platform/middleware"
	"kita-be/internal/platform/response"
)

// ListenAndServeWithGracefulShutdown starts the Fiber app on the specified port, listens for termination signals (SIGINT, SIGTERM),
// and performs graceful shutdown with the specified timeout.
func ListenAndServeWithGracefulShutdown(app *fiber.App, port string, serviceName string, shutdownTimeout time.Duration) error {
	serverErr := make(chan error, 1)
	go func() {
		if err := app.Listen(net.JoinHostPort("", port)); err != nil {
			serverErr <- err
		}
	}()

	logger.Info("server listening", "service", serviceName, "port", port)

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-sigCtx.Done():
		// Restore default signal behavior so a second signal force-kills the process.
		stop()
		logger.Info("shutdown signal received", "service", serviceName)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "service", serviceName, "error", err.Error())
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	logger.Info("server stopped cleanly", "service", serviceName)
	return nil
}

// ReadinessCheck verifies that a single dependency is reachable.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// RegisterHealthRoutes registers standard liveness and readiness endpoints.
// The readiness endpoint runs every check and returns 503 on the first failure.
func RegisterHealthRoutes(app *fiber.App, basePath string, serviceName string, checks ...ReadinessCheck) {
	app.Get(basePath+"/health", func(c *fiber.Ctx) error {
		return response.OK(c, fiber.Map{
			"service": serviceName,
			"status":  "healthy",
		})
	})

	app.Get(basePath+"/ready", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		for _, check := range checks {
			if err := check.Check(ctx); err != nil {
				logger.Warn("readiness check failed", "service", serviceName, "dependency", check.Name, "error", err.Error())
				return response.Error(c, fiber.StatusServiceUnavailable, "NOT_READY", check.Name+" is not reachable")
			}
		}

		return response.OK(c, fiber.Map{
			"service": serviceName,
			"status":  "ready",
		})
	})
}

func New() *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "kita-be",
		ErrorHandler: customErrorHandler,
	})

	app.Use(middleware.RequestID())
	app.Use(middleware.Recovery())
	app.Use(middleware.Logger())

	return app
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "an unexpected error occurred"
	var appErr *apperror.Error
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	} else if errors.As(err, &appErr) {
		code = statusForAppError(appErr.Kind)
		message = appErr.Message
	}
	errorCode := errorCodeForStatus(code)
	if code >= fiber.StatusInternalServerError {
		message = "an unexpected error occurred"
	}

	args := []any{
		"error", err.Error(),
		"path", c.Path(),
		"method", c.Method(),
		"request_id", c.Locals("request_id"),
	}
	if code >= fiber.StatusInternalServerError {
		logger.Error("request failed", args...)
	} else {
		logger.Warn("request rejected", args...)
	}

	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    errorCode,
			"message": message,
		},
	})
}

func statusForAppError(kind apperror.Kind) int {
	switch kind {
	case apperror.KindBadRequest:
		return fiber.StatusBadRequest
	case apperror.KindUnauthorized:
		return fiber.StatusUnauthorized
	case apperror.KindForbidden:
		return fiber.StatusForbidden
	case apperror.KindNotFound:
		return fiber.StatusNotFound
	case apperror.KindConflict:
		return fiber.StatusConflict
	default:
		return fiber.StatusInternalServerError
	}
}

func errorCodeForStatus(status int) string {
	switch status {
	case fiber.StatusBadRequest:
		return "VALIDATION_ERROR"
	case fiber.StatusUnauthorized:
		return "UNAUTHORIZED"
	case fiber.StatusForbidden:
		return "FORBIDDEN"
	case fiber.StatusNotFound:
		return "NOT_FOUND"
	case fiber.StatusConflict:
		return "CONFLICT"
	case fiber.StatusTooManyRequests:
		return "RATE_LIMITED"
	case fiber.StatusServiceUnavailable:
		return "NOT_READY"
	case fiber.StatusGatewayTimeout:
		return "TIMEOUT"
	default:
		return "INTERNAL_ERROR"
	}
}
