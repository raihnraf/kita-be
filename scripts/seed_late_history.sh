#!/usr/bin/env bash
set -euo pipefail

IDENTITY_URL="${IDENTITY_URL:-http://localhost:3000}"
BOOK_URL="${BOOK_URL:-http://localhost:3001}"
TRANSACTION_URL="${TRANSACTION_URL:-http://localhost:3002}"

COUNT="${1:-${COUNT:-1}}"
LATE_DAYS="${LATE_DAYS:-}"
LATE_DAY_SEQUENCE="${LATE_DAY_SEQUENCE:-3,30,90}"
MAX_RETRIES="${MAX_RETRIES:-20}"
POLL_DELAY_SECONDS="${POLL_DELAY_SECONDS:-1}"
MAX_ACTIVE_BORROWS="${MAX_ACTIVE_BORROWS:-3}"
FULL_NAME="${FULL_NAME:-Demo User}"
EMAIL="${EMAIL:-demo@kita.test}"
PASSWORD="${PASSWORD:-123456}"
USE_EXISTING_USER="${USE_EXISTING_USER:-0}"
REUSE_ACTIVE_TRANSACTIONS="${REUSE_ACTIVE_TRANSACTIONS:-0}"
TMP_FILES=()

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Command '$1' tidak ditemukan. Install dulu sebelum menjalankan script ini."
    exit 1
  fi
}

cleanup() {
  if [ "${#TMP_FILES[@]}" -gt 0 ]; then
    rm -f "${TMP_FILES[@]}"
  fi
}

make_tmp_file() {
  local tmp_file
  tmp_file=$(mktemp)
  TMP_FILES+=("$tmp_file")
  printf '%s' "$tmp_file"
}

validate_positive_integer() {
  local name="$1"
  local value="$2"

  if ! [[ "$value" =~ ^[1-9][0-9]*$ ]]; then
    echo "$name harus berupa integer positif."
    exit 1
  fi
}

contains_exact_line() {
  local needle="$1"
  shift

  local item
  for item in "$@"; do
    if [ "$item" = "$needle" ]; then
      return 0
    fi
  done
  return 1
}

borrow_transaction() {
  local token="$1"
  local book_id="$2"
  local label="$3"
  local borrow_key="seed-late-borrow-$label-$(date +%s%N)"
  local borrow_raw
  borrow_raw=$(make_tmp_file)
  local borrow_http_status
  borrow_http_status=$(curl -s -o "$borrow_raw" -w "%{http_code}" -X POST "$TRANSACTION_URL/api/v1/transactions/borrow" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"book_id\":\"$book_id\",\"idempotency_key\":\"$borrow_key\"}")
  local borrow_resp
  borrow_resp=$(jq -c . "$borrow_raw")

  if [ "$borrow_http_status" != "202" ]; then
    echo "Borrow gagal dengan HTTP $borrow_http_status: $borrow_resp" >&2
    return 1
  fi

  local tx_id
  tx_id=$(printf '%s' "$borrow_resp" | jq -r '.data.id')
  if [ -z "$tx_id" ] || [ "$tx_id" = "null" ]; then
    echo "Borrow tidak menghasilkan transaction id: $borrow_resp" >&2
    return 1
  fi

  poll_transaction_status "$token" "$tx_id" "ACTIVE" >/dev/null
  printf '%s' "$tx_id"
}

mark_transaction_late() {
  local tx_id="$1"
  local late_days="$2"
  local borrow_age_days=$((late_days + 7))

  echo "Memundurkan due date transaksi $tx_id agar terlambat $late_days hari..."
  docker compose exec -T postgres psql -U postgres -d kita_transaction -v ON_ERROR_STOP=1 \
    -c "UPDATE borrow_transactions SET borrowed_at = NOW() - INTERVAL '$borrow_age_days days', due_at = NOW() - INTERVAL '$late_days days', updated_at = NOW() WHERE id = '$tx_id';" \
    >/dev/null
}

return_transaction_late() {
  local token="$1"
  local tx_id="$2"
  local label="$3"
  local return_key="seed-late-return-$label-$(date +%s%N)"
  local return_raw
  return_raw=$(make_tmp_file)
  local return_http_status
  return_http_status=$(curl -s -o "$return_raw" -w "%{http_code}" -X POST "$TRANSACTION_URL/api/v1/transactions/$tx_id/return" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"idempotency_key\":\"$return_key\"}")
  local return_resp
  return_resp=$(jq -c . "$return_raw")

  if [ "$return_http_status" != "202" ]; then
    echo "Return gagal dengan HTTP $return_http_status: $return_resp" >&2
    return 1
  fi

  poll_transaction_status "$token" "$tx_id" "RETURNED_LATE"
}

poll_transaction_status() {
  local token="$1"
  local tx_id="$2"
  local expected_status="$3"
  local attempt=1

  while [ "$attempt" -le "$MAX_RETRIES" ]; do
    local detail
    detail=$(curl -s -X GET "$TRANSACTION_URL/api/v1/transactions/$tx_id" \
      -H "Authorization: Bearer $token")
    local status
    status=$(printf '%s' "$detail" | jq -r '.data.status')

    echo "Poll #$attempt untuk $tx_id -> $status" >&2
    if [ "$status" = "$expected_status" ]; then
      printf '%s' "$detail"
      return 0
    fi

    sleep "$POLL_DELAY_SECONDS"
    attempt=$((attempt + 1))
  done

  echo "Transaksi $tx_id tidak mencapai status $expected_status dalam $MAX_RETRIES percobaan." >&2
  return 1
}

require_command curl
require_command jq
require_command docker

trap cleanup EXIT

if ! [[ "$COUNT" =~ ^[1-9][0-9]*$ ]]; then
  echo "COUNT harus berupa integer positif. Contoh: ./scripts/seed_late_history.sh 2"
  exit 1
fi

if [ "$COUNT" -gt 3 ]; then
  echo "COUNT maksimal 3 agar data demo tetap terkontrol."
  exit 1
fi

if [ "$USE_EXISTING_USER" != "0" ] && [ "$USE_EXISTING_USER" != "1" ]; then
  echo "USE_EXISTING_USER harus bernilai 0 atau 1."
  exit 1
fi

if [ "$REUSE_ACTIVE_TRANSACTIONS" != "0" ] && [ "$REUSE_ACTIVE_TRANSACTIONS" != "1" ]; then
  echo "REUSE_ACTIVE_TRANSACTIONS harus bernilai 0 atau 1."
  exit 1
fi

validate_positive_integer "MAX_ACTIVE_BORROWS" "$MAX_ACTIVE_BORROWS"

if [ -n "$LATE_DAYS" ]; then
  validate_positive_integer "LATE_DAYS" "$LATE_DAYS"
fi

IFS=',' read -r -a LATE_DAY_VALUES <<< "$LATE_DAY_SEQUENCE"
if [ "${#LATE_DAY_VALUES[@]}" -lt "$COUNT" ]; then
  echo "LATE_DAY_SEQUENCE harus punya minimal $COUNT nilai. Contoh default: 3,30,90"
  exit 1
fi

for idx in "${!LATE_DAY_VALUES[@]}"; do
  LATE_DAY_VALUES[$idx]="${LATE_DAY_VALUES[$idx]// /}"
  validate_positive_integer "LATE_DAY_SEQUENCE[$idx]" "${LATE_DAY_VALUES[$idx]}"
done

echo "=== 1. Cek Backend Ready ==="
curl -fsS "$IDENTITY_URL/api/v1/ready" >/dev/null
curl -fsS "$BOOK_URL/api/v1/ready" >/dev/null
curl -fsS "$TRANSACTION_URL/api/v1/ready" >/dev/null
echo "Backend ready."

echo "=== 2. Register User Seeder ==="
if [ "$USE_EXISTING_USER" = "1" ]; then
  echo "Melewati registrasi karena USE_EXISTING_USER=1"
else
  REGISTER_RESP=$(curl -sS -X POST "$IDENTITY_URL/api/v1/auth/register" \
    -H "Content-Type: application/json" \
    -d "{\"full_name\":\"$FULL_NAME\",\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")

  REGISTER_SUCCESS=$(printf '%s' "$REGISTER_RESP" | jq -r '.success')
  if [ "$REGISTER_SUCCESS" = "true" ]; then
    echo "User seeder dibuat: $EMAIL"
  else
    echo "Registrasi tidak membuat user baru. Script akan mencoba login dengan akun existing: $EMAIL"
  fi
fi

echo "=== 3. Login Seeder User ==="
LOGIN_RESP=$(curl -fsS -X POST "$IDENTITY_URL/api/v1/auth/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password&email=$EMAIL&password=$PASSWORD")

TOKEN=$(printf '%s' "$LOGIN_RESP" | jq -r '.data.access_token')
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  echo "Login gagal: $LOGIN_RESP"
  exit 1
fi
echo "Login berhasil."

echo "=== 4. Ambil Data Pinjaman Aktif User ==="
ACTIVE_RESP=$(curl -fsS "$TRANSACTION_URL/api/v1/transactions/active" \
  -H "Authorization: Bearer $TOKEN")
ACTIVE_COUNT=$(printf '%s' "$ACTIVE_RESP" | jq '.data | length')
ACTIVE_BOOK_IDS=()
if [ "$ACTIVE_COUNT" -gt 0 ]; then
  while IFS= read -r active_book_id; do
    [ -n "$active_book_id" ] && ACTIVE_BOOK_IDS+=("$active_book_id")
  done < <(printf '%s' "$ACTIVE_RESP" | jq -r '.data[].book_id')
fi

USE_ACTIVE_COUNT=0
if [ "$REUSE_ACTIVE_TRANSACTIONS" = "1" ]; then
  USE_ACTIVE_COUNT=$ACTIVE_COUNT
  if [ "$USE_ACTIVE_COUNT" -gt "$COUNT" ]; then
    USE_ACTIVE_COUNT=$COUNT
  fi
  if [ "$USE_ACTIVE_COUNT" -gt 0 ]; then
    echo "Ditemukan $ACTIVE_COUNT transaksi aktif. $USE_ACTIVE_COUNT akan dipakai untuk skenario denda karena REUSE_ACTIVE_TRANSACTIONS=1."
  fi
elif [ "$ACTIVE_COUNT" -gt 0 ]; then
  echo "Ditemukan $ACTIVE_COUNT transaksi aktif, tetapi script tidak akan mengubahnya karena REUSE_ACTIVE_TRANSACTIONS=0."
fi

echo "=== 5. Ambil Buku Yang Bisa Dipinjam ==="
BOOKS_RESP=$(curl -fsS "$BOOK_URL/api/v1/books?page=1&per_page=50")
AVAILABLE_BOOKS=()
AVAILABLE_TITLES=()

while IFS=$'\t' read -r candidate_id candidate_title; do
  [ -z "$candidate_id" ] && continue
  if contains_exact_line "$candidate_id" "${ACTIVE_BOOK_IDS[@]}"; then
    continue
  fi
  if contains_exact_line "$candidate_title" "${AVAILABLE_TITLES[@]}"; then
    continue
  fi
  AVAILABLE_BOOKS+=("$candidate_id")
  AVAILABLE_TITLES+=("$candidate_title")
done < <(printf '%s' "$BOOKS_RESP" | jq -r '.data[] | select(.available_stock > 0) | [.id, .title] | @tsv')

BORROW_NEEDED=$((COUNT - USE_ACTIVE_COUNT))
if [ "$REUSE_ACTIVE_TRANSACTIONS" = "0" ] && [ $((ACTIVE_COUNT + BORROW_NEEDED)) -gt "$MAX_ACTIVE_BORROWS" ]; then
  echo "Jumlah transaksi aktif user ($ACTIVE_COUNT) ditambah kebutuhan borrow baru ($BORROW_NEEDED) melebihi MAX_ACTIVE_BORROWS=$MAX_ACTIVE_BORROWS."
  echo "Return dulu transaksi aktif user, atau jalankan dengan REUSE_ACTIVE_TRANSACTIONS=1 jika memang ingin mengubah transaksi aktif menjadi skenario denda."
  exit 1
fi

if [ "$BORROW_NEEDED" -gt 0 ] && [ "${#AVAILABLE_BOOKS[@]}" -lt "$BORROW_NEEDED" ]; then
  echo "Buku unik yang bisa dipinjam tidak cukup. Dibutuhkan $BORROW_NEEDED, tersedia ${#AVAILABLE_BOOKS[@]}."
  exit 1
fi

if [ "${#AVAILABLE_BOOKS[@]}" -gt 0 ]; then
  echo "Kandidat buku baru: ${AVAILABLE_TITLES[*]}"
fi

CREATED_IDS=()
CREATED_BOOK_TITLES=()

for i in $(seq 1 "$COUNT"); do
  ITEM_LATE_DAYS="$LATE_DAYS"
  if [ -z "$ITEM_LATE_DAYS" ]; then
    ITEM_LATE_DAYS="${LATE_DAY_VALUES[$((i - 1))]}"
  fi

  if [ "$i" -le "$USE_ACTIVE_COUNT" ]; then
    TX_ID=$(printf '%s' "$ACTIVE_RESP" | jq -r ".data[$((i - 1))].id")
    BOOK_TITLE=$(printf '%s' "$ACTIVE_RESP" | jq -r ".data[$((i - 1))].book.title")
    echo "=== 5.$i Pakai transaksi aktif $TX_ID ==="
  else
    candidate_index=$((i - USE_ACTIVE_COUNT - 1))
    BOOK_ID="${AVAILABLE_BOOKS[$candidate_index]}"
    BOOK_TITLE="${AVAILABLE_TITLES[$candidate_index]}"
    echo "=== 5.$i Borrow baru untuk skenario denda ==="
    TX_ID=$(borrow_transaction "$TOKEN" "$BOOK_ID" "$i")
  fi

  mark_transaction_late "$TX_ID" "$ITEM_LATE_DAYS"

  echo "=== 6.$i Return Ke-$i ==="
  FINAL_DETAIL=$(return_transaction_late "$TOKEN" "$TX_ID" "$i")
  FINAL_FINE=$(printf '%s' "$FINAL_DETAIL" | jq -r '.data.fine_amount_cents')
  FINAL_LATE_DAYS=$(printf '%s' "$FINAL_DETAIL" | jq -r '.data.late_days')

  echo "Transaksi late fee berhasil: id=$TX_ID late_days=$FINAL_LATE_DAYS fine_amount_cents=$FINAL_FINE"
  CREATED_IDS+=("$TX_ID")
  CREATED_BOOK_TITLES+=("$BOOK_TITLE")
done

echo "=== 7. Ringkasan Seeder ==="
echo "Email login : $EMAIL"
echo "Password    : $PASSWORD"
echo "Books       : ${CREATED_BOOK_TITLES[*]}"
echo "Jumlah seed : $COUNT"
if [ -n "$LATE_DAYS" ]; then
  echo "Late days   : $LATE_DAYS (seragam)"
else
  echo "Late days   : ${LATE_DAY_VALUES[*]:0:$COUNT}"
fi
echo "Transaction IDs: ${CREATED_IDS[*]}"
echo "Sekarang buka halaman History di Flutter untuk melihat item RETURNED_LATE dan nominal denda."
