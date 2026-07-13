package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/transaction/domain"
)

type AuditRepository struct {
	pool *pgxpool.Pool
}

func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

func (r *AuditRepository) Create(ctx context.Context, audit *domain.TransactionAudit) error {
	query := `
		INSERT INTO transaction_audits (id, transaction_id, from_status, to_status, reason, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.pool.Exec(ctx, query,
		audit.ID, audit.TransactionID, audit.FromStatus, audit.ToStatus,
		audit.Reason, audit.Metadata, audit.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create audit: %w", err)
	}
	return nil
}

func (r *AuditRepository) FindByTransaction(ctx context.Context, txnID string) ([]domain.TransactionAudit, error) {
	query := `
		SELECT id, transaction_id, from_status, to_status, reason, metadata, created_at
		FROM transaction_audits WHERE transaction_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, txnID)
	if err != nil {
		return nil, fmt.Errorf("failed to find audits: %w", err)
	}
	defer rows.Close()

	var audits []domain.TransactionAudit
	for rows.Next() {
		var a domain.TransactionAudit
		if err := rows.Scan(
			&a.ID, &a.TransactionID, &a.FromStatus, &a.ToStatus,
			&a.Reason, &a.Metadata, &a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit: %w", err)
		}
		audits = append(audits, a)
	}

	return audits, nil
}
