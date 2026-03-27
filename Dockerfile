FROM golang:1.25-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/goeoserv ./cmd/goeoserv

FROM debian:bookworm-slim

WORKDIR /app

RUN useradd --create-home --shell /usr/sbin/nologin goeoserv

COPY --from=build /out/goeoserv ./goeoserv
COPY config ./config
COPY sql ./sql

RUN mkdir -p /app/data && chown -R goeoserv:goeoserv /app

USER goeoserv

EXPOSE 8078 8079

CMD ["./goeoserv"]
