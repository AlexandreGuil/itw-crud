# syntax=docker/dockerfile:1.7
FROM golang:1.25-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w -X main.Version=$VERSION" \
    -o /out/server ./cmd/server

# dbmate static binary (used by init container at runtime)
FROM amacneil/dbmate:latest AS dbmate

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server /server
COPY --from=dbmate /usr/local/bin/dbmate /dbmate
COPY db/migrations /db/migrations
USER nonroot:nonroot
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/server"]
