# kube-sched-lens

**A desktop debugger for GPU/accelerator scheduling on Kubernetes with Dynamic Resource Allocation (DRA).**

When a Pod that requests a GPU sits in `Pending`, the answer is scattered across four
`resource.k8s.io/v1` objects (DeviceClass, ResourceSlice, ResourceClaim, ResourceClaimTemplate),
scheduler events, and node capacity. kube-sched-lens joins all of them live and tells you **why**
— in one sentence, with the evidence attached.

![screenshot placeholder](docs/screenshot.png)

## What it does

- **Pending Pods** — live table of every Pending pod with a diagnosed category:
  `unallocated-claim`, `no-matching-device`, `insufficient-capacity`, `taint`, `affinity`, `unknown`.
- **Diagnosis** — per-pod verdict: one-sentence summary, a suggestion, and the evidence trail
  (scheduler events, claim allocation status, matching/missing ResourceSlice devices).
- **GPU Capacity** — per driver/pool/node device inventory from ResourceSlices, with
  allocated vs. free counts derived from allocated ResourceClaims.

Updates stream over WebSocket from client-go informers — no polling, no `kubectl` round-trips.

## Architecture

```
┌──────────────────────────── Electron app ───────────────────────────┐
│  React/TS renderer  ── REST + WebSocket ──►  Go backend (child     │
│  (3 views)                                   process, port 8151)   │
└─────────────────────────────────────────────────────┬──────────────┘
                                                      │ client-go informers
                                                      ▼
                        Pods · Events · Nodes · resource.k8s.io/v1
                        (DeviceClass, ResourceSlice, ResourceClaim,
                         ResourceClaimTemplate)
```

- **Backend** (Go): client-go informers keep an in-memory index; a diagnosis engine
  cross-references FailedScheduling events with claim allocation state and slice inventory.
- **Frontend** (Electron + Vite + React/TS): spawns the backend as a child process,
  renders live state over WebSocket.

## Quickstart

### Demo mode (no cluster needed)

```sh
go run ./cmd/kube-sched-lens --demo
cd app && npm install && npm run dev
```

Demo fixtures are modeled on a kind cluster running the upstream
[dra-example-driver](https://github.com/kubernetes-sigs/dra-example-driver), including a
deliberately stuck, unallocatable ResourceClaim.

### Against a real cluster

Requires Kubernetes ≥ 1.34 (DRA GA, `resource.k8s.io/v1`).

```sh
go run ./cmd/kube-sched-lens            # uses your kubeconfig
cd app && npm run dev
```

## Development

```sh
go test ./...        # diagnosis engine unit tests
cd app && npm run build
```

## Why this exists

Built while preparing a proposal for the LFX mentorship
[*Adding Dynamic Resource Allocation (DRA) to Headlamp*](https://github.com/kubernetes-sigs/headlamp)
— as a from-scratch exploration of the same problem space: making the DRA object graph
navigable and debuggable for humans.

## License

Apache-2.0
