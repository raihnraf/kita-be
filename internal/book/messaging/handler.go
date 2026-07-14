package messaging

import (
	"context"
	"fmt"

	bookdomain "kita-be/internal/book/domain"
	"kita-be/internal/book/usecase"
	"kita-be/internal/platform/logger"
	"kita-be/internal/platform/rabbitmq"
)

type Handler struct {
	stockUC   *usecase.StockUsecase
	publisher ResultPublisher
}

func NewHandler(stockUC *usecase.StockUsecase, publisher ResultPublisher) *Handler {
	return &Handler{stockUC: stockUC, publisher: publisher}
}

func (h *Handler) HandleStockEvent(msg rabbitmq.Message) error {
	logger.Info("processing stock event",
		"event_id", msg.EventID,
		"event_type", msg.EventType,
		"book_id", msg.BookID,
		"transaction_id", msg.TransactionID,
	)

	operation, err := rabbitmq.OperationFromCommandEventType(msg.EventType)
	if err != nil {
		return err
	}

	switch operation {
	case "DECREASE":
		event, err := h.stockUC.DecreaseStockEvent(context.Background(), msg.BookID, msg.Quantity, msg.TransactionID, msg.EventID)
		if err != nil {
			logger.Error("stock decrease failed",
				"event_id", msg.EventID,
				"book_id", msg.BookID,
				"error", err.Error(),
			)
			return fmt.Errorf("stock decrease failed: %w", err)
		}
		if err := h.publishResult(event); err != nil {
			return err
		}

	case "INCREASE":
		event, err := h.stockUC.IncreaseStockEvent(context.Background(), msg.BookID, msg.Quantity, msg.TransactionID, msg.EventID)
		if err != nil {
			logger.Error("stock increase failed",
				"event_id", msg.EventID,
				"book_id", msg.BookID,
				"error", err.Error(),
			)
			return fmt.Errorf("stock increase failed: %w", err)
		}
		if err := h.publishResult(event); err != nil {
			return err
		}
	}

	logger.Info("stock event processed successfully", "event_id", msg.EventID)
	return nil
}

func (h *Handler) publishResult(event *bookdomain.BookStockEvent) error {
	if h.publisher == nil {
		return fmt.Errorf("result publisher is not configured")
	}
	if err := h.publisher.PublishStockResult(context.Background(), event); err != nil {
		logger.Error("failed to publish stock result",
			"event_id", event.EventID,
			"event_type", event.EventType,
			"error", err.Error(),
		)
		return fmt.Errorf("failed to publish stock result: %w", err)
	}
	return nil
}
