package domain

import (
	"time"
)

type TransactionAudit struct {
	ID            string
	TransactionID string
	FromStatus    *string
	ToStatus      string
	Reason        string
	Metadata      *string
	CreatedAt     time.Time
}
