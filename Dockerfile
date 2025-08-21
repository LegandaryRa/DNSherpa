FROM golang:1.25-alpine AS builder

# Build arguments for version information
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
ARG BUILD_TIME=unknown
ARG BUILD_BY=unknown

WORKDIR /app

# Install git for build information (if needed)
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

# Build with version information embedded
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags "-X 'main.Version=${VERSION}' \
              -X 'main.GitCommit=${GIT_COMMIT}' \
              -X 'main.GitBranch=${GIT_BRANCH}' \
              -X 'main.BuildTime=${BUILD_TIME}' \
              -X 'main.BuildBy=${BUILD_BY}'" \
    -o dnsherpa .

FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/dnsherpa .

CMD ["./dnsherpa"]