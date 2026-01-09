# building binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o ptt-alertor .

# building executable image
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/ptt-alertor .

ENTRYPOINT ["./ptt-alertor"]

EXPOSE 9090 6060