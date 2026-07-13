FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/identity-api ./cmd/identity-api
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/book-api ./cmd/book-api
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/transaction-api ./cmd/transaction-api
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/book-worker ./cmd/book-worker

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/bin/ ./

EXPOSE 3000 3001 3002

CMD ["./identity-api"]
