package middleware

import (
	"crypto/subtle"

	"github.com/gofiber/fiber/v2"

	"kita-be/internal/platform/response"
)

func InternalAuth(expectedToken string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Get("X-Internal-Token")
		if token == "" {
			return response.Unauthorized(c, "UNAUTHORIZED", "missing internal token")
		}
		if len(token) != len(expectedToken) || subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
			return response.Forbidden(c, "FORBIDDEN", "invalid internal token")
		}
		return c.Next()
	}
}
