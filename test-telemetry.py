from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.sdk.resources import Resource
from opentelemetry.semconv.resource import ResourceAttributes
import time
import random

# Configuration
COLLECTOR_ENDPOINT = "localhost:4317"  # OTLP gRPC endpoint
SOURCES = ["src1", "src2", "src3", "src4"]  # 4 sources
EXPORT_INTERVAL_SECONDS = 0.5  # How often to export metrics
GENERATION_INTERVAL_SECONDS = 0.1  # How often to generate new data

# Create a separate resource and meter provider per source
resources = {}
meter_providers = {}
counters = {}

for source in SOURCES:
    # Resource with source.id
    res = Resource(attributes={
        ResourceAttributes.SERVICE_NAME: "telemetry-generator",
        "source.id": source,  # Key fix: resource attribute
    })
    resources[source] = res

    # Exporter (shared, but per-resource meter provider)
    exporter = OTLPMetricExporter(endpoint=COLLECTOR_ENDPOINT, insecure=True)
    reader = PeriodicExportingMetricReader(exporter, export_interval_millis=EXPORT_INTERVAL_SECONDS * 1000)

    # Per-source meter provider
    provider = MeterProvider(resource=res, metric_readers=[reader])
    meter_providers[source] = provider

    meter = provider.get_meter(f"telemetry-generator-{source}")
    counter = meter.create_counter(
        name="custom.request.count",
        description="Number of requests per source"
    )
    counters[source] = counter

print("Generating metrics... Press Ctrl+C to stop")

try:
    while True:
        for source, counter in counters.items():
            value = random.randint(1, 50)
            counter.add(value)  # No extra attributes needed (source.id is on resource)
            print(f"Sent {value} requests for {source}")
        
        time.sleep(GENERATION_INTERVAL_SECONDS)
except KeyboardInterrupt:
    print("\nShutting down...")
    for provider in meter_providers.values():
        provider.shutdown()