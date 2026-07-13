package messaging

import (
	"context"
	"fmt"

	"kita-be/internal/book/usecase"
	"kita-be/internal/platform/logger"
	"kita-be/internal/platform/rabbitmq"
)

type Handler struct {
	stockUC *usecase.StockUsecase
}

func NewHandler(stockUC *usecase.StockUsecase) *Handler {
	return &Handler{stockUC: stockUC}
}

func (h *Handler) HandleStockEvent(msg rabbitmq.Message) error {
	logger.Info("processing stock event",
		"event_id", msg.EventID,
		"event_type", msg.EventType,
		"book_id", msg.BookID,
		"transaction_id", msg.TransactionID,
	)

	switch msg.EventType {
	case "DECREASE":
		_, err := h.stockUC.DecreaseStockEvent(context.Background(), msg.BookID, msg.Quantity, msg.TransactionID, msg.EventID)
		if err != nil {
			logger.Error("stock decrease failed",
				"event_id", msg.EventID,
				"book_id", msg.BookID,
				"error", err.Error(),
			)
			return fmt.Errorf("stock decrease failed: %w", err)
		}

	case "INCREASE":
		_, err := h.stockUC.IncreaseStockEvent(context.Background(), msg.BookID, msg.Quantity, msg.TransactionID, msg.EventID)
		if err != nil {
			logger.Error("stock increase failed",
				"event_id", msg.EventID,
				"book_id", msg.BookID,
				"error", err.Error(),
			)
			return fmt.Errorf("stock increase failed: %w", err)
		}

	default:
		return fmt.Errorf("unknown event type: %s", msg.EventType)
	}

	logger.Info("stock event processed successfully", "event_id", msg.EventID)
	return nil
}
