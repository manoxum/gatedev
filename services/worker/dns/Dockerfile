FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/dns-provider ./cmd/dns-provider

FROM alpine:3.22
COPY --from=build /out/dns-provider /usr/local/bin/dns-provider
EXPOSE 53/udp
ENTRYPOINT ["/usr/local/bin/dns-provider"]
