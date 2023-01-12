FROM golang:latest AS builder
ENV CGO_ENABLED=0
WORKDIR /build
COPY . .
RUN apt update && apt install -y ca-certificates
RUN go build -ldflags '-s -w' -trimpath -o shc-prometheus-exporter .

FROM scratch AS final
COPY --from=builder /build/shc-prometheus-exporter /shc-prometheus-exporter
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/shc-prometheus-exporter"]
