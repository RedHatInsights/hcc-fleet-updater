# HCC Operator (Work in Progress)

A Kubernetes operator that automates fleet-wide container image rollouts across [ClowdApp](https://github.com/RedHatInsights/clowder) custom resources. It sits on top of Clowder and patches ClowdApp image references in coordinated, wave-based rollouts with health monitoring.

## Overview

The HCC operator introduces two CRDs:

| CRD | Purpose |
|-----|---------|
| **HCCFleetUpdate** | Orchestrates a fleet-wide image rollout across multiple ClowdApps with wave-based ordering, parallelism control, health checks, and failure policies. |
| **HCCAppConfiguration** | Provides persistent inventory and health monitoring for a single ClowdApp — tracks current images and deployment health. |

**API Group:** `hcc.redhat.com/v1alpha1`

### How It Works

1. You create an `HCCFleetUpdate` CR listing the ClowdApps to update with their desired image:tag pairs.
2. The operator groups apps into waves by `priority` (lower values go first).
3. Within each wave, apps are updated in parallel up to `maxParallelism`.
4. After patching each ClowdApp's image references, the operator monitors deployment health before advancing to the next wave.
5. The operator records previous image tags on each app status for rollback reference.

## CRDs

### HCCFleetUpdate

Orchestrates a fleet-wide image rollout.

```yaml
apiVersion: hcc.redhat.com/v1alpha1
kind: HCCFleetUpdate
metadata:
  name: march-2026-cves
spec:
  description: "March 2026 monthly CVE image refresh"
  strategy:
    maxParallelism: 5        # Max concurrent app updates (default: 5)
    healthCheckTimeout: 10m  # Time to wait for healthy deployments (default: 10m)
    failurePolicy: Continue  # Continue or Halt on failure (default: Continue)
  apps:
  - clowdAppName: host-inventory
    namespace: hcc-inventory
    images:
    - image: quay.io/cloudservices/insights-host-inventory
      tag: "a1b2c3d"
    priority: 1              # Lower = earlier wave (default: 10)
  - clowdAppName: advisor-backend
    namespace: hcc-advisor
    images:
    - image: quay.io/cloudservices/advisor-backend
      tag: "e4f5g6h"
    priority: 5
  paused: false              # Set to true to pause the rollout
```

**Status fields:**

| Field | Description |
|-------|-------------|
| `phase` | `Pending`, `InProgress`, `Completed`, `PartiallyFailed`, `Failed`, `Paused` |
| `summary` | Aggregate counters: `total`, `pending`, `inProgress`, `succeeded`, `failed`, `skipped` |
| `appStatuses[]` | Per-app phase, message, previousImages, and lastTransitionTime |
| `conditions` | Standard Kubernetes conditions: `Ready`, `Progressing`, `Degraded` |

**kubectl output:**

```
NAME              PHASE       SUCCEEDED   FAILED   TOTAL   AGE
march-2026-cves   Completed   3           0        3       5m
```

**Failure policies:**

- **Continue** — skip the failed app and keep rolling out the remaining apps.
- **Halt** — automatically pause the rollout when any app fails. Resume by setting `spec.paused: false`.

### HCCAppConfiguration

Monitors a single ClowdApp's current images and deployment health.

```yaml
apiVersion: hcc.redhat.com/v1alpha1
kind: HCCAppConfiguration
metadata:
  name: host-inventory
spec:
  clowdAppName: host-inventory
  namespace: hcc-inventory
  enabled: true
  images:
  - image: quay.io/cloudservices/insights-host-inventory
```

**Status fields:**

| Field | Description |
|-------|-------------|
| `currentImages` | Image:tag pairs currently running on the ClowdApp |
| `healthy` | Whether all deployments have ready replicas |
| `lastUpdated` | Timestamp of last status refresh |

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    HCC Operator                          │
│                                                          │
│  ┌─────────────────────┐   ┌──────────────────────────┐  │
│  │  FleetUpdate        │   │  AppConfiguration        │  │
│  │  Controller         │   │  Controller              │  │
│  │                     │   │                          │  │
│  │  - Wave management  │   │  - Image inventory       │  │
│  │  - Parallelism      │   │  - Health monitoring     │  │
│  │  - Failure policy   │   │  - Periodic reconcile    │  │
│  └────────┬────────────┘   └────────────┬─────────────┘  │
│           │                             │                │
│  ┌────────▼─────────────────────────────▼─────────────┐  │
│  │  ClowdApp Patcher / Health Checker                 │  │
│  │  - Image matching & patching                       │  │
│  │  - Deployment readiness checks                     │  │
│  │  - Pod error detection (CrashLoop, ImagePull)      │  │
│  └────────────────────────┬───────────────────────────┘  │
└───────────────────────────┼──────────────────────────────┘
                            │
                 ┌──────────▼──────────┐
                 │  ClowdApp CRs       │
                 │  (cloud.redhat.com)  │
                 └─────────────────────┘
```

The operator uses **local ClowdApp type definitions** (`internal/clowdapp/types.go`) instead of importing the Clowder module directly, avoiding transitive dependency conflicts between controller-runtime versions.

### Health Checking

The operator monitors deployment health by:

1. Listing Deployments labeled `app=<clowdAppName>` in the target namespace.
2. Checking that `readyReplicas >= replicas` for each Deployment.
3. Inspecting pod container statuses for error states (`CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`).

Health states: **Healthy** (all replicas ready), **Pending** (waiting for rollout), **Unhealthy** (pod errors detected).

## Prerequisites

- Go 1.25+
- kubectl v1.28+
- Access to a Kubernetes cluster with Clowder installed
- podman or docker (auto-detected)

## Development

### Build

```sh
make generate    # Generate DeepCopy methods
make manifests   # Generate CRDs and RBAC
make build       # Build the manager binary
```

Or all at once:

```sh
make build       # Runs generate + manifests + fmt + vet + build
```

### Test

```sh
go test ./...
```

### Run Locally

```sh
make run
```

This runs the controller against whatever cluster your kubeconfig points at.

### Container Image

```sh
make docker-build IMG=quay.io/myorg/hcc-operator:latest
make docker-push IMG=quay.io/myorg/hcc-operator:latest
```

The Dockerfile uses Red Hat UBI base images:
- **Builder:** `registry.access.redhat.com/ubi8/go-toolset`
- **Runtime:** `registry.access.redhat.com/ubi8/ubi-minimal`

## Deployment

### Install CRDs

```sh
make install
```

### Deploy the Operator

```sh
make deploy IMG=quay.io/myorg/hcc-operator:latest
```

### Apply Sample CRs

```sh
kubectl apply -k config/samples/
```

### Uninstall

```sh
kubectl delete -k config/samples/   # Delete CRs
make uninstall                       # Delete CRDs
make undeploy                        # Remove the operator
```

### Consolidated Installer

Generate a single YAML file containing all resources:

```sh
make build-installer IMG=quay.io/myorg/hcc-operator:latest
# Output: dist/install.yaml
```

## Project Structure

```
├── api/v1alpha1/                  # CRD type definitions
│   ├── hccfleetupdate_types.go
│   ├── hccappconfiguration_types.go
│   └── groupversion_info.go
├── cmd/main.go                    # Entrypoint
├── internal/
│   ├── clowdapp/types.go          # Local ClowdApp type definitions
│   └── controller/
│       ├── hccfleetupdate_controller.go       # Fleet update reconciler
│       ├── hccappconfiguration_controller.go  # App config reconciler
│       ├── clowdapp_patcher.go                # Image matching & patching
│       └── health_checker.go                  # Deployment health checks
├── config/
│   ├── crd/bases/                 # Generated CRD manifests
│   ├── rbac/                      # Generated RBAC roles
│   ├── manager/                   # Controller manager deployment
│   ├── samples/                   # Example CRs
│   └── default/                   # Kustomize overlay
├── Dockerfile
└── Makefile
```

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0. See [LICENSE](http://www.apache.org/licenses/LICENSE-2.0) for details.
