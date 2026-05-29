# Contributing to AetherPak CLI

Thank you for your interest in contributing to the AetherPak CLI project! This document outlines the development workflow, testing requirements, and quality standards for contributors.

## Development Setup

### System Prerequisites
To build, run, and test the CLI locally, your development environment must have the following tools installed:
* **Go** (version 1.26 or newer)
* **flatpak** and **ostree** (required for local system executions)
* **gpg** (required for GPG key generation, signing operations, and integration tests)
* **flatpak-builder-lint** (required to run build-related check-points with linting active)
* **Docker** or **Podman** (with `docker-compose`/`podman-compose` to drive local OCI registry integration tests)

---

## Developer Workflow

### Compilation
To compile the CLI binary:
```bash
make build
```
The compiled output binary is written to `bin/aetherpak`.

### Running Tests
AetherPak CLI comes with an extensive suite of unit and integration tests.

* **Unit Tests:** Run standard unit checks in-memory (and against simulated subprocess mock wrappers):
  ```bash
  make test
  ```
* **E2E Integration Tests:** Spin up a local OCI registry container and run end-to-end publishing tests:
  ```bash
  make test/integration
  ```

### Code Formatting & Quality
Ensure your code adheres to standard styling rules before opening a pull request:

* **Formatting:** Run standard gofmt adjustments:
  ```bash
  make fmt
  ```
* **Linting / Vetting:** Run static analysis checks:
  ```bash
  make vet
  ```
All checks must pass successfully in our CI pipeline before code can be merged.
