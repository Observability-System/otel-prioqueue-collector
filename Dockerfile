# Stage 1: Builder
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make bash

WORKDIR /app

# Copy all modules
COPY weightupdateextension/ weightupdateextension/
COPY weightedqueueprocessor/ weightedqueueprocessor/
COPY prioqueue-collector/ prioqueue-collector/

WORKDIR /app/prioqueue-collector

# Install OCB
RUN go install go.opentelemetry.io/collector/cmd/builder@latest

# Build
RUN builder --config manifest.yaml

# Stage 2: Runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/prioqueue-collector/build/prioqueue-collector /usr/local/bin/prioqueue-collector

COPY prioqueue-collector/config.yaml /etc/otelcol/config.yaml

EXPOSE 4317 4318 4500 8888

ENTRYPOINT ["prioqueue-collector"]
CMD ["--config=/etc/otelcol/config.yaml"]