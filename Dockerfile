FROM node:22-alpine AS web
WORKDIR /build/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ .
RUN npm run build

FROM golang:1.23-alpine AS api
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /build/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /jerboa ./cmd/jerboa

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates tzdata
COPY --from=api /jerboa /usr/local/bin/jerboa
COPY migrations/ /migrations/
ENV JERBOA_MIGRATIONS_PATH=/migrations
EXPOSE 8080
ENTRYPOINT ["jerboa"]
