package domain

type IdempotencyRecord struct {
	ID              string
	Scope           string
	IdempotencyKey  string
	RequestHash     string
	ResponsePayload []byte
	Status          string
}
