FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /courtscore-api ./cmd/api

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /courtscore-api /courtscore-api
ENTRYPOINT ["/courtscore-api"]
