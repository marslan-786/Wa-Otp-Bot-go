# بلڈ سٹیج
FROM golang:1.24-alpine AS builder

# سسٹم ٹولز
RUN apk add --no-cache gcc musl-dev git sqlite-dev

WORKDIR /app

# تمام فائلیں کاپی کریں
COPY . .

# اگر پہلے سے کوئی go.mod ہے تو اسے صاف کر کے نیا بنائیں تاکہ ورژن کنفلکٹ نہ ہو
RUN rm -f go.mod go.sum || true
RUN go mod init otp-bot
RUN go get go.mau.fi/whatsmeow@latest
RUN go get github.com/lib/pq@latest
RUN go get github.com/mattn/go-sqlite3@latest
RUN go mod tidy

# بوٹ بلڈ کرنا
RUN go build -o bot .

# رن سٹیج
FROM alpine:latest
RUN apk add --no-cache ca-certificates sqlite-libs
WORKDIR /app
COPY --from=builder /app/bot .

# بوٹ اسٹارٹ کریں
CMD ["./bot"]