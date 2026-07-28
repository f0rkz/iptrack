FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/iptrack ./cmd/iptrack

FROM alpine:3.22
RUN apk add --no-cache ca-certificates iputils && addgroup -S iptrack && adduser -S -G iptrack iptrack
COPY --from=build /out/iptrack /usr/local/bin/iptrack
USER iptrack
EXPOSE 8080
ENTRYPOINT ["iptrack", "-listen", ":8080"]
