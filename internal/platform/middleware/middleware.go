package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/google/uuid"

	"kita-be/internal/platform/logger"
	"kita-be/internal/platform/response"
)

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = c.Get("X-Request-Id")
		}
		if requestID == "" {
			requestID = c.Get("x-request-id")
		}
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Locals("request_id", requestID)
		c.Set("X-Request-ID", requestID)
		return c.Next()
	}
}

func RateLimit(max int, expiration time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: expiration,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP() + ":" + c.Path()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return response.Error(c, fiber.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
		},
	})
}

func Recovery() fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				err, ok := r.(error)
				if !ok {
					err = fiber.ErrInternalServerError
				}

				logger.Error("panic recovered",
					"error", err.Error(),
					"path", c.Path(),
					"method", c.Method(),
					"request_id", c.Locals("request_id"),
				)

				_ = response.InternalError(c, "INTERNAL_ERROR", "an unexpected error occurred")
			}
		}()

		return c.Next()
	}
}

func Logger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		elapsed := time.Since(start)
		status := c.Response().StatusCode()
		requestID, _ := c.Locals("request_id").(string)

		args := []any{
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"latency_ms", elapsed.Milliseconds(),
			"request_id", requestID,
		}

		if status >= 500 {
			logger.Error("request completed", args...)
		} else if status >= 400 {
			logger.Warn("request completed", args...)
		} else {
			logger.Info("request completed", args...)
		}

		return err
	}
}
