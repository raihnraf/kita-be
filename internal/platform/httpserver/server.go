package httpserver

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"kita-be/internal/platform/apperror"
	"kita-be/internal/platform/logger"
	"kita-be/internal/platform/middleware"
)

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
