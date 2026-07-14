package messaging

import (
	"context"
	"time"

	"kita-be/internal/platform/logger"
	"kita-be/internal/transaction/usecase"
)

type ReconciliationWorker struct {
	repo           usecase.TransactionRepository
	interval       time.Duration
	stuckThreshold time.Duration
}

func NewReconciliationWorker(repo usecase.TransactionRepository, interval time.Duration, stuckThreshold time.Duration) *ReconciliationWorker {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if stuckThreshold <= 0 {
		stuckThreshold = 1 * time.Minute
	}
	return &ReconciliationWorker{
		repo:           repo,
		interval:       interval,
		stuckThreshold: stuckThreshold,
	}
}

func (w *ReconciliationWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	logger.Info("starting borrow transaction reconciliation background worker")

	for {
		w.reconcileOnce(ctx)

		select {
		case <-ctx.Done():
			logger.Info("stopping borrow transaction reconciliation background worker")
			return
		case <-ticker.C:
		}
	}
}

func (w *ReconciliationWorker) reconcileOnce(ctx context.Context) {
	threshold := time.Now().UTC().Add(-w.stuckThreshold)
	txns, err := w.repo.FindPendingOlderThan(ctx, threshold)
	if err != nil {
		logger.Warn("failed to fetch stuck pending transactions for reconciliation", "error", err.Error())
		return
	}

	for _, txn := range txns {
		eventType := "DECREASE"
		if txn.IsReturnPending() {
			eventType = "INCREASE"
		}
		logger.Info("re-dispatching stock command for stuck transaction",
			"transaction_id", txn.ID,
			"status", txn.Status,
			"event_type", eventType,
			"created_at", txn.CreatedAt,
		)

		err := w.repo.RequeueStockCommand(ctx, txn.ID, eventType)
		if err != nil {
			logger.Error("failed to re-dispatch stock command for stuck transaction", "transaction_id", txn.ID, "error", err.Error())
			continue
		}

		logger.Info("successfully re-dispatched stock command for stuck transaction", "transaction_id", txn.ID, "event_type", eventType)
	}
}
