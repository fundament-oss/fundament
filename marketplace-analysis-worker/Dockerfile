FROM golang:1.27.4-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY marketplace-analysis-worker/ ./marketplace-analysis-worker/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /build/bin/marketplace-analysis-worker ./marketplace-analysis-worker/cmd/fun-marketplace-analysis-worker

# The same digest-pinned release .github/workflows/image-scan.yml runs and
# verifies, so the SBOM step and the later vulnerability scan share one Trivy.
FROM aquasec/trivy:0.75.0@sha256:af6acf9a6b85dfe389a1941505c0ce9efef52a4719635e1a962f022a3d855daa AS trivy

FROM scratch
# Registries are reached over https.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=trivy /usr/local/bin/trivy /trivy
COPY --from=builder /build/bin/marketplace-analysis-worker /
USER 65532:65532
ENTRYPOINT ["/marketplace-analysis-worker"]
