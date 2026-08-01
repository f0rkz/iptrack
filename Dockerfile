FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/iptrack ./cmd/iptrack

FROM alpine:3.22
ARG VERSION=dev
ARG REVISION=unknown
RUN apk add --no-cache ca-certificates iputils && addgroup -S iptrack && adduser -S -G iptrack iptrack
COPY --from=build /out/iptrack /usr/local/bin/iptrack
LABEL org.opencontainers.image.title="iptrack" \
      org.opencontainers.image.description="Automation-first IP address management" \
      org.opencontainers.image.source="https://github.com/f0rkz/iptrack" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
USER iptrack
EXPOSE 8080
ENTRYPOINT ["iptrack", "-listen", ":8080"]
