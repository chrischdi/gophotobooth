FROM golang:1.26.1 AS builder
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,id=gophotobooth-gomod,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,id=gophotobooth-gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gophotobooth-gobuild,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o /ctl ./cmd/ctl

FROM gcr.io/distroless/static-debian12
COPY --from=builder /ctl /ctl
ENTRYPOINT ["/ctl"]
