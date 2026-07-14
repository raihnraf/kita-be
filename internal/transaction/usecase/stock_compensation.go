package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "kita-be/internal/transaction/domain"
)

const compensationEnqueueTimeout = 10 * time.Second

func enqueueCompensationStockEvent(
	txnRepo TransactionRepository,
	tx *domain.BorrowTransaction,
	eventType string,
	reason string,
) error {
	outbox := domain.NewStockEventOutbox(uuid.NewString(), eventType, tx)
	outbox.SetCompensationMetadata(inverseStockEventType(eventType), reason)

	ctx, cancel := context.WithTimeout(context.Background(), compensationEnqueueTimeout)
	defer cancel()

	if err := txnRepo.EnqueueStockEvent(ctx, outbox); err != nil {
		return fmt.Errorf("failed to enqueue compensation stock event: %w", err)
	}
	return nil
}

func inverseStockEventType(eventType string) string {
	if eventType == "INCREASE" {
		return "DECREASE"
	}
	return "INCREASE"
}
