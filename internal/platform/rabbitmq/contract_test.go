package rabbitmq

import "testing"

func TestCommandEventTypeForOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		expected  string
	}{
		{name: "decrease", operation: "DECREASE", expected: EventTypeDecreaseStockRequested},
		{name: "increase", operation: "INCREASE", expected: EventTypeIncreaseStockRequested},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := CommandEventTypeForOperation(tt.operation)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if actual != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, actual)
			}
		})
	}
}

func TestResultEventTypeForOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		rejected  bool
		expected  string
	}{
		{name: "decrease success", operation: "DECREASE", expected: EventTypeDecreaseStockSucceeded},
		{name: "decrease rejected", operation: "DECREASE", rejected: true, expected: EventTypeDecreaseStockRejected},
		{name: "increase success", operation: "INCREASE", expected: EventTypeIncreaseStockSucceeded},
		{name: "increase rejected", operation: "INCREASE", rejected: true, expected: EventTypeIncreaseStockRejected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := ResultEventTypeForOperation(tt.operation, tt.rejected)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if actual != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, actual)
			}
		})
	}
}
