package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"kita-be/internal/platform/logger"
	"kita-be/internal/platform/rabbitmq"
	domain "kita-be/internal/transaction/domain"
	"kita-be/internal/transaction/usecase"
)

type ResultHandler struct {
	txnRepo   usecase.TransactionRepository
	auditRepo usecase.AuditRepository
}

func NewResultHandler(txnRepo usecase.TransactionRepository, auditRepo usecase.AuditRepository) *ResultHandler {
	return &ResultHandler{txnRepo: txnRepo, auditRepo: auditRepo}
}

func (h *ResultHandler) HandleStockResult(msg rabbitmq.Message) error {
	txn, err := h.txnRepo.FindByID(context.Background(), msg.TransactionID)
	if err != nil {
		return fmt.Errorf("failed to find transaction for stock result: %w", err)
	}

	operation, err := rabbitmq.OperationFromResultEventType(msg.EventType)
	if err != nil {
		logger.Warn("ignoring unknown stock result event type", "event_type", msg.EventType, "transaction_id", msg.TransactionID)
		return nil
	}

	switch operation {
	case "DECREASE":
		return h.handleBorrowResult(txn, msg)
	case "INCREASE":
		return h.handleReturnResult(txn, msg)
	}

	return nil
}

func (h *ResultHandler) handleBorrowResult(txn *domain.BorrowTransaction, msg rabbitmq.Message) error {
	switch msg.EventType {
	case rabbitmq.EventTypeDecreaseStockSucceeded:
		if txn.IsActive() {
			return nil
		}
		if !txn.IsPending() {
			logger.Warn("ignoring borrow success result for non-pending transaction", "transaction_id", txn.ID, "status", txn.Status)
			return nil
		}
		if err := h.txnRepo.ActivateBorrow(context.Background(), txn.ID, msg.EventID); err != nil {
			return fmt.Errorf("failed to activate borrow from stock result: %w", err)
		}
		return h.createAudit(txn.ID, string(domain.TransactionPending), string(domain.TransactionActive), "Borrow activated after stock reservation", msg.ErrorMessage)
	case rabbitmq.EventTypeDecreaseStockRejected:
		if txn.Status == domain.TransactionCancelled {
			return nil
		}
		if !txn.IsPending() {
			logger.Warn("ignoring borrow rejection result for non-pending transaction", "transaction_id", txn.ID, "status", txn.Status)
			return nil
		}
		if err := h.txnRepo.CancelBorrow(context.Background(), txn.ID); err != nil {
			return fmt.Errorf("failed to cancel borrow from stock rejection: %w", err)
		}
		reason := "Borrow cancelled after stock reservation was rejected"
		if msg.ErrorMessage != "" {
			reason = fmt.Sprintf("%s: %s", reason, msg.ErrorMessage)
		}
		return h.createAudit(txn.ID, string(domain.TransactionPending), string(domain.TransactionCancelled), reason, msg.ErrorMessage)
	default:
		return fmt.Errorf("unsupported borrow stock result event type: %s", msg.EventType)
	}
}

func (h *ResultHandler) handleReturnResult(txn *domain.BorrowTransaction, msg rabbitmq.Message) error {
	switch msg.EventType {
	case rabbitmq.EventTypeIncreaseStockSucceeded:
		if txn.Status == domain.TransactionReturned || txn.Status == domain.TransactionReturnedLate {
			return nil
		}
		if !txn.IsReturnPending() {
			logger.Warn("ignoring return success result for non-return-pending transaction", "transaction_id", txn.ID, "status", txn.Status)
			return nil
		}
		if err := h.txnRepo.FinalizeReturn(context.Background(), txn.ID, msg.EventID); err != nil {
			return fmt.Errorf("failed to finalize return from stock result: %w", err)
		}
		updatedTxn, err := h.txnRepo.FindByID(context.Background(), txn.ID)
		if err != nil {
			return fmt.Errorf("failed to reload finalized return transaction: %w", err)
		}
		return h.createAudit(updatedTxn.ID, string(domain.TransactionReturnPending), string(updatedTxn.Status), "Return completed after stock restore", msg.ErrorMessage)
	case rabbitmq.EventTypeIncreaseStockRejected:
		if txn.IsActive() {
			return nil
		}
		if !txn.IsReturnPending() {
			logger.Warn("ignoring return rejection result for non-return-pending transaction", "transaction_id", txn.ID, "status", txn.Status)
			return nil
		}
		if err := h.txnRepo.RejectReturn(context.Background(), txn.ID); err != nil {
			return fmt.Errorf("failed to reject return from stock result: %w", err)
		}
		reason := "Return request reverted after stock restore was rejected"
		if msg.ErrorMessage != "" {
			reason = fmt.Sprintf("%s: %s", reason, msg.ErrorMessage)
		}
		return h.createAudit(txn.ID, string(domain.TransactionReturnPending), string(domain.TransactionActive), reason, msg.ErrorMessage)
	default:
		return fmt.Errorf("unsupported return stock result event type: %s", msg.EventType)
	}
}

func (h *ResultHandler) createAudit(transactionID, fromStatus, toStatus, reason, metadata string) error {
	var from *string
	if fromStatus != "" {
		from = &fromStatus
	}
	var meta *string
	if metadata != "" {
		meta = &metadata
	}
	return h.auditRepo.Create(context.Background(), &domain.TransactionAudit{
		ID:            uuid.NewString(),
		TransactionID: transactionID,
		FromStatus:    from,
		ToStatus:      toStatus,
		Reason:        reason,
		Metadata:      meta,
		CreatedAt:     time.Now(),
	})
}
