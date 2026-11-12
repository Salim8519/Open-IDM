# 0006 — HTTP/1.1 for segmented downloads

- Status: accepted
- Date: 2026-11-13

## Context
Most of the speed gain from segmented downloads comes from opening several TCP connections:
each one gets its own congestion window and, on many servers and CDNs, its own per-connection
bandwidth cap. HTTP/2 multiplexes all requests to a host over ONE TCP connection, which turns
8 "connections" back into one.

## Decision
The engine's HTTP client disables HTTP/2 (`TLSNextProto` set to an empty map) so every
segment worker opens its own TCP connection. HTTP/3 stays out of scope until it can be
benchmarked against this.

## Consequences
- Parallel segments are real parallel connections (verified by `TestSegmentedDownload`,
  which asserts the test server saw ≥ 4 concurrent connections).
- Slightly more TLS handshakes per download; negligible next to the gain on large files.
