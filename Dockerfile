FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/vibetutor ./cmd/vibetutor

FROM alpine:3.20

RUN adduser -D -u 10001 app \
	&& mkdir -p /data \
	&& chown -R app:app /data

COPY --from=build /out/vibetutor /usr/local/bin/vibetutor

USER app
ENV VIBETUTOR_DATA_DIR=/data
VOLUME ["/data"]

ENTRYPOINT ["vibetutor"]
