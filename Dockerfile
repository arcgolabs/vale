# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

WORKDIR /src
COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -C cmd -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/valed .

FROM --platform=$BUILDPLATFORM alpine:3 AS optimize

RUN apk add --no-cache upx

COPY --from=build /out/valed /out/valed

RUN upx --best --lzma /out/valed

FROM alpine:3

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

LABEL org.opencontainers.image.title="Vale" \
      org.opencontainers.image.description="Library-first reverse proxy gateway and valed binary." \
      org.opencontainers.image.source="https://github.com/arcgolabs/vale" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${DATE}"

RUN apk add --no-cache ca-certificates tzdata

COPY --from=optimize /out/valed /usr/local/bin/valed

USER 65532:65532
EXPOSE 8080 19090

ENTRYPOINT ["/usr/local/bin/valed"]
