package usecase

import (
	"context"
	"fmt"

	"kita-be/internal/platform/apperror"
	domain "kita-be/internal/transaction/domain"
)

type HistoryUsecase struct {
	txnRepo   TransactionRepository
	auditRepo AuditRepository
}

func NewHistoryUsecase(txnRepo TransactionRepository, auditRepo AuditRepository) *HistoryUsecase {
	return &HistoryUsecase{
		txnRepo:   txnRepo,
		auditRepo: auditRepo,
	}
}

type HistoryInput struct {
	UserID  string
	Page    int
	PerPage int
}

type HistoryOutput struct {
	Transactions []domain.BorrowTransaction
	Total        int64
}

func (uc *HistoryUsecase) GetHistory(ctx context.Context, input HistoryInput) (*HistoryOutput, error) {
	normalizeHistoryInput(&input)

	txs, total, err := uc.txnRepo.GetHistory(ctx, input.UserID, input.Page, input.PerPage)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	return &HistoryOutput{Transactions: txs, Total: total}, nil
}

func (uc *HistoryUsecase) GetAll(ctx context.Context, input HistoryInput) (*HistoryOutput, error) {
	normalizeHistoryInput(&input)

	txs, total, err := uc.txnRepo.ListAll(ctx, input.Page, input.PerPage)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}

	return &HistoryOutput{Transactions: txs, Total: total}, nil
}

func normalizeHistoryInput(input *HistoryInput) {
	if input.Page < 1 {
		input.Page = 1
	}
	if input.PerPage < 1 || input.PerPage > 100 {
		input.PerPage = 20
	}
}

func (uc *HistoryUsecase) GetActive(ctx context.Context, userID string) ([]domain.BorrowTransaction, error) {
	txs, err := uc.txnRepo.FindActiveByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active loans: %w", err)
	}
	return txs, nil
}

func (uc *HistoryUsecase) GetDetail(ctx context.Context, txnID, userID string) (*domain.BorrowTransaction, error) {
	txn, err := uc.txnRepo.FindByID(ctx, txnID)
	if err != nil {
		return nil, apperror.NotFound("transaction not found")
	}

	if !txn.BelongsTo(userID) {
		return nil, apperror.Forbidden("access denied")
	}

	return txn, nil
}

func (uc *HistoryUsecase) GetInternalDetail(ctx context.Context, txnID string) (*domain.BorrowTransaction, error) {
	txn, err := uc.txnRepo.FindByID(ctx, txnID)
	if err != nil {
		return nil, apperror.NotFound("transaction not found")
	}
	return txn, nil
}

func (uc *HistoryUsecase) GetInternalAudits(ctx context.Context, txnID string) ([]domain.TransactionAudit, error) {
	if _, err := uc.txnRepo.FindByID(ctx, txnID); err != nil {
		return nil, apperror.NotFound("transaction not found")
	}
	audits, err := uc.auditRepo.FindByTransaction(ctx, txnID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction audits: %w", err)
	}
	return audits, nil
}
