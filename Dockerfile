FROM golang:1.25-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/geoserv ./cmd/geoserv

FROM debian:bookworm-slim

WORKDIR /app

RUN useradd --create-home --shell /usr/sbin/nologin geoserv

COPY --from=build /out/geoserv ./geoserv
COPY config ./config

RUN mkdir -p /app/data && chown -R geoserv:geoserv /app

USER geoserv

EXPOSE 8078 8079

CMD ["./geoserv"]
