package rabbitmq

import "fmt"

const (
	EventTypeDecreaseStockRequested = "DecreaseStockRequested"
	EventTypeIncreaseStockRequested = "IncreaseStockRequested"
	EventTypeDecreaseStockSucceeded = "DecreaseStockSucceeded"
	EventTypeDecreaseStockRejected  = "DecreaseStockRejected"
	EventTypeIncreaseStockSucceeded = "IncreaseStockSucceeded"
	EventTypeIncreaseStockRejected  = "IncreaseStockRejected"
)

func CommandEventTypeForOperation(operation string) (string, error) {
	switch operation {
	case "DECREASE":
		return EventTypeDecreaseStockRequested, nil
	case "INCREASE":
		return EventTypeIncreaseStockRequested, nil
	default:
		return "", fmt.Errorf("unsupported stock command operation: %s", operation)
	}
}

func ResultEventTypeForOperation(operation string, rejected bool) (string, error) {
	switch operation {
	case "DECREASE":
		if rejected {
			return EventTypeDecreaseStockRejected, nil
		}
		return EventTypeDecreaseStockSucceeded, nil
	case "INCREASE":
		if rejected {
			return EventTypeIncreaseStockRejected, nil
		}
		return EventTypeIncreaseStockSucceeded, nil
	default:
		return "", fmt.Errorf("unsupported stock result operation: %s", operation)
	}
}

func OperationFromCommandEventType(eventType string) (string, error) {
	switch eventType {
	case EventTypeDecreaseStockRequested:
		return "DECREASE", nil
	case EventTypeIncreaseStockRequested:
		return "INCREASE", nil
	default:
		return "", fmt.Errorf("unsupported stock command event type: %s", eventType)
	}
}

func OperationFromResultEventType(eventType string) (string, error) {
	switch eventType {
	case EventTypeDecreaseStockSucceeded, EventTypeDecreaseStockRejected:
		return "DECREASE", nil
	case EventTypeIncreaseStockSucceeded, EventTypeIncreaseStockRejected:
		return "INCREASE", nil
	default:
		return "", fmt.Errorf("unsupported stock result event type: %s", eventType)
	}
}

func RoutingKeyForCommandEventType(eventType string) (string, error) {
	switch eventType {
	case EventTypeDecreaseStockRequested:
		return RoutingKeyDec, nil
	case EventTypeIncreaseStockRequested:
		return RoutingKeyInc, nil
	default:
		return "", fmt.Errorf("unsupported stock command routing event type: %s", eventType)
	}
}

func RoutingKeyForResultEventType(eventType string) (string, error) {
	switch eventType {
	case EventTypeDecreaseStockSucceeded, EventTypeDecreaseStockRejected:
		return RoutingKeyDecResult, nil
	case EventTypeIncreaseStockSucceeded, EventTypeIncreaseStockRejected:
		return RoutingKeyIncResult, nil
	default:
		return "", fmt.Errorf("unsupported stock result routing event type: %s", eventType)
	}
}
