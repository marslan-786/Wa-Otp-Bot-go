# بلڈ سٹیج
FROM golang:1.21-alpine AS builder
RUN apk add --no-cache gcc musl-dev sqlite-dev
WORKDIR /app
COPY . .
RUN go mod tidy
RUN go build -o bot .

# رن سٹیج
FROM alpine:latest
RUN apk add --no-cache sqlite-libs ca-certificates
WORKDIR /app
COPY --from=builder /app/bot .
# ڈیٹا بیس محفوظ رکھنے کے لیے
VOLUME /app/data
CMD ["./bot"]