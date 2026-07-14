#!/usr/bin/env bash
set -e

# Configuration
IDENTITY_URL="http://localhost:3000"
BOOK_URL="http://localhost:3001"
TRANSACTION_URL="http://localhost:3002"

EMAIL="verify_user_$(date +%s)@example.com"
PASSWORD="securePassword123"
FULL_NAME="Verification User"

echo "=== 1. Registering New User ==="
REGISTER_RESP=$(curl -s -X POST "$IDENTITY_URL/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"full_name\":\"$FULL_NAME\",\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")

echo "Register response: $REGISTER_RESP"
SUCCESS=$(echo "$REGISTER_RESP" | jq -r '.success')
if [ "$SUCCESS" != "true" ]; then
  echo "Registration failed!"
  exit 1
fi

echo "=== 2. Logging In to Retrieve JWT ==="
LOGIN_RESP=$(curl -s -X POST "$IDENTITY_URL/api/v1/auth/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password&email=$EMAIL&password=$PASSWORD")

TOKEN=$(echo "$LOGIN_RESP" | jq -r '.data.access_token')
if [ -z "$TOKEN" ] || [ "$TOKEN" == "null" ]; then
  echo "Login failed! Response: $LOGIN_RESP"
  exit 1
fi
echo "JWT Token retrieved successfully."

echo "=== 3. Fetching Book Catalog ==="
BOOKS_RESP=$(curl -s "$BOOK_URL/api/v1/books?page=1&per_page=10")
BOOK_ITEM=$(echo "$BOOKS_RESP" | jq -c '.data[] | select(.available_stock > 0)' | head -n 1)
BOOK_ID=$(echo "$BOOK_ITEM" | jq -r '.id')
BOOK_TITLE=$(echo "$BOOK_ITEM" | jq -r '.title')
INITIAL_STOCK=$(echo "$BOOK_ITEM" | jq -r '.available_stock')

if [ -z "$BOOK_ID" ] || [ "$BOOK_ID" == "null" ]; then
  echo "No borrowable books found in catalog!"
  exit 1
fi
echo "Selected Book: $BOOK_TITLE ($BOOK_ID)"
echo "Initial Stock: $INITIAL_STOCK"

echo "=== 4. Requesting Borrow ==="
IDEMPOTENCY_KEY_BORROW="idemp-borrow-$(date +%s)"
BORROW_RAW=$(mktemp)
BORROW_HTTP_STATUS=$(curl -s -o "$BORROW_RAW" -w "%{http_code}" -X POST "$TRANSACTION_URL/api/v1/transactions/borrow" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"book_id\":\"$BOOK_ID\",\"idempotency_key\":\"$IDEMPOTENCY_KEY_BORROW\"}")
BORROW_RESP=$(jq -c . "$BORROW_RAW")
rm -f "$BORROW_RAW"

echo "Borrow initial response: $BORROW_RESP"
echo "Borrow HTTP status: $BORROW_HTTP_STATUS"
TX_ID=$(echo "$BORROW_RESP" | jq -r '.data.id')
INITIAL_STATUS=$(echo "$BORROW_RESP" | jq -r '.data.status')

if [ "$BORROW_HTTP_STATUS" != "202" ]; then
	echo "Expected borrow HTTP status 202, got: $BORROW_HTTP_STATUS"
	exit 1
fi

if [ -z "$TX_ID" ] || [ "$TX_ID" == "null" ]; then
  echo "Borrow request failed!"
  exit 1
fi

if [ "$INITIAL_STATUS" != "PENDING" ]; then
  echo "Expected initial borrow status PENDING, got: $INITIAL_STATUS"
  exit 1
fi
echo "Borrow Transaction created with ID: $TX_ID. Initial status is PENDING."

echo "=== 5. Polling for Borrow Finalization (PENDING -> ACTIVE) ==="
STATUS="PENDING"
RETRIES=0
MAX_RETRIES=10

while [ "$STATUS" == "PENDING" ] && [ $RETRIES -lt $MAX_RETRIES ]; do
  sleep 1
  TX_DETAIL=$(curl -s -X GET "$TRANSACTION_URL/api/v1/transactions/$TX_ID" \
    -H "Authorization: Bearer $TOKEN")
  STATUS=$(echo "$TX_DETAIL" | jq -r '.data.status')
  echo "Poll #$RETRIES - Current status: $STATUS"
  RETRIES=$((RETRIES+1))
done

if [ "$STATUS" != "ACTIVE" ]; then
  echo "Transaction failed to reach ACTIVE status! Final status: $STATUS"
  exit 1
fi
echo "Transaction $TX_ID successfully transitioned to ACTIVE."

echo "=== 6. Verifying Stock Decrement ==="
AVAIL_RESP=$(curl -s "$BOOK_URL/api/v1/books/$BOOK_ID/availability")
POST_BORROW_STOCK=$(echo "$AVAIL_RESP" | jq -r '.data.available_stock')
echo "Stock after borrow: $POST_BORROW_STOCK"
EXPECTED_STOCK=$((INITIAL_STOCK - 1))

if [ "$POST_BORROW_STOCK" -ne "$EXPECTED_STOCK" ]; then
  echo "Expected stock to be $EXPECTED_STOCK, but got $POST_BORROW_STOCK!"
  exit 1
fi
echo "Stock decremented correctly."

echo "=== 7. Requesting Return ==="
IDEMPOTENCY_KEY_RETURN="idemp-return-$(date +%s)"
RETURN_RAW=$(mktemp)
RETURN_HTTP_STATUS=$(curl -s -o "$RETURN_RAW" -w "%{http_code}" -X POST "$TRANSACTION_URL/api/v1/transactions/$TX_ID/return" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"idempotency_key\":\"$IDEMPOTENCY_KEY_RETURN\"}")
RETURN_RESP=$(jq -c . "$RETURN_RAW")
rm -f "$RETURN_RAW"

echo "Return initial response: $RETURN_RESP"
echo "Return HTTP status: $RETURN_HTTP_STATUS"
RETURN_STATUS=$(echo "$RETURN_RESP" | jq -r '.data.status')

if [ "$RETURN_HTTP_STATUS" != "202" ]; then
	echo "Expected return HTTP status 202, got: $RETURN_HTTP_STATUS"
	exit 1
fi

if [ "$RETURN_STATUS" != "RETURN_PENDING" ]; then
  echo "Expected initial return status RETURN_PENDING, got: $RETURN_STATUS"
  exit 1
fi
echo "Return request submitted. Initial status is RETURN_PENDING."

echo "=== 8. Polling for Return Finalization (RETURN_PENDING -> RETURNED) ==="
STATUS="RETURN_PENDING"
RETRIES=0

while [ "$STATUS" == "RETURN_PENDING" ] && [ $RETRIES -lt $MAX_RETRIES ]; do
  sleep 1
  TX_DETAIL=$(curl -s -X GET "$TRANSACTION_URL/api/v1/transactions/$TX_ID" \
    -H "Authorization: Bearer $TOKEN")
  STATUS=$(echo "$TX_DETAIL" | jq -r '.data.status')
  echo "Poll #$RETRIES - Current status: $STATUS"
  RETRIES=$((RETRIES+1))
done

if [ "$STATUS" != "RETURNED" ] && [ "$STATUS" != "RETURNED_LATE" ]; then
  echo "Transaction failed to reach RETURNED/RETURNED_LATE status! Final status: $STATUS"
  exit 1
fi
echo "Transaction $TX_ID successfully transitioned to final status: $STATUS."

echo "=== 9. Verifying Stock Recovery ==="
AVAIL_RESP2=$(curl -s "$BOOK_URL/api/v1/books/$BOOK_ID/availability")
POST_RETURN_STOCK=$(echo "$AVAIL_RESP2" | jq -r '.data.available_stock')
echo "Stock after return: $POST_RETURN_STOCK"

if [ "$POST_RETURN_STOCK" -ne "$INITIAL_STOCK" ]; then
  echo "Expected stock to return to $INITIAL_STOCK, but got $POST_RETURN_STOCK!"
  exit 1
fi
echo "Stock restored correctly."

echo "=== SUCCESS: End-to-end integration verified successfully! ==="
