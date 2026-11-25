FROM node:22-alpine AS web

WORKDIR /web
COPY web/package.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build


FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sourcebox .


FROM alpine:3.21

RUN apk add --no-cache ca-certificates && adduser -D -u 10001 sourcebox

WORKDIR /app
COPY --from=build /out/sourcebox ./sourcebox
COPY --from=web /web/dist ./web/dist

ENV DATA_DIR=/data \
    WEB_DIR=/app/web/dist \
    PORT=8080 \
    HOST=0.0.0.0

RUN mkdir -p /data && chown -R sourcebox:sourcebox /data /app

USER sourcebox
VOLUME ["/data"]
EXPOSE 8080

CMD ["./sourcebox"]

