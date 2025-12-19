# بلڈ سٹیج
FROM golang:1.21-alpine AS builder
RUN apk add --no-cache gcc musl-dev sqlite-dev git
WORKDIR /app
COPY . .
# یہاں ہم ورژن کو خودکار اپڈیٹ کرنے کا حکم دے رہے ہیں
RUN go mod tidy
RUN go build -o bot .

# رن سٹیج
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/bot .
CMD ["./bot"]