FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/backend ./cmd/backend

FROM alpine:3.22
RUN apk add --no-cache ca-certificates curl \
    && mkdir -p /usr/local/share/bindnet \
    && curl -fsSL https://standards-oui.ieee.org/oui/oui.csv -o /usr/local/share/bindnet/oui.csv \
    && apk del curl
COPY --from=build /out/backend /usr/local/bin/backend
ENTRYPOINT ["/usr/local/bin/backend"]
