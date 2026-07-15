package messaging

import (
	"context"
	"fmt"
	"time"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/book/usecase"
	"kita-be/internal/platform/logger"
	"kita-be/internal/platform/rabbitmq"
)

const handlerTimeout = 10 * time.Second

type Handler struct {
	stockUC   *usecase.StockUsecase
	publisher ResultPublisher
}

func NewHandler(stockUC *usecase.StockUsecase, publisher ResultPublisher) *Handler {
	return &Handler{stockUC: stockUC, publisher: publisher}
}

func (h *Handler) HandleStockEvent(msg rabbitmq.Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

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
		event, err := h.stockUC.DecreaseStockEvent(ctx, msg.BookID, msg.Quantity, msg.TransactionID, msg.EventID)
		if err != nil {
			logger.Error("stock decrease failed",
				"event_id", msg.EventID,
				"book_id", msg.BookID,
				"error", err.Error(),
			)
			return fmt.Errorf("stock decrease failed: %w", err)
		}
		if err := h.publishResult(ctx, event); err != nil {
			return err
		}

	case "INCREASE":
		event, err := h.stockUC.IncreaseStockEvent(ctx, msg.BookID, msg.Quantity, msg.TransactionID, msg.EventID)
		if err != nil {
			logger.Error("stock increase failed",
				"event_id", msg.EventID,
				"book_id", msg.BookID,
				"error", err.Error(),
			)
			return fmt.Errorf("stock increase failed: %w", err)
		}
		if err := h.publishResult(ctx, event); err != nil {
			return err
		}
	}

	logger.Info("stock event processed successfully", "event_id", msg.EventID)
	return nil
}

func (h *Handler) publishResult(ctx context.Context, event *domain.BookStockEvent) error {
	if h.publisher == nil {
		return fmt.Errorf("result publisher is not configured")
	}
	if err := h.publisher.PublishStockResult(ctx, event); err != nil {
		logger.Error("failed to publish stock result",
			"event_id", event.EventID,
			"event_type", event.EventType,
			"error", err.Error(),
		)
		return fmt.Errorf("failed to publish stock result: %w", err)
	}
	return nil
}
