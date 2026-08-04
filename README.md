# keepup-helm-scraper

A Kubernetes CronJob that scans every Deployment, StatefulSet, and DaemonSet in the cluster, detects known application images by regex, extracts their version, and reports the result to [`keepup`](https://github.com/code-tool/keepup) as `kubernetes_cluster_info` inventory.

## Contents

- [How it works](#how-it-works)
- [Deploying with Helm](#deploying-with-helm)
- [Configuration](#configuration)
- [Detection rules](#detection-rules)
- [Local development](#local-development)
- [Build](#build)

## How it works

On each run the scraper:

1. Connects to the cluster via in-cluster config.
2. Collects every container and init-container image, across all namespaces, from Deployments, StatefulSets, and DaemonSets.
3. Matches each image string against the rules in `rules.yaml` (`detectionRegex`).
4. Extracts and normalizes the matched version to `major.minor.patch` semver (`versionRegex`).
5. Sends one `PUT` request with the aggregated cluster payload to `API_URL`.

```jsonc
{
  "cluster_name": "prod-eu",
  "kube_version": "v1.29.4",
  "team": "platform",
  "helm_charts": [
    { "chart_name": "redis", "version": "7.4.0", "namespace": "database" },
    { "chart_name": "gitlab-runner", "version": "18.1.3", "namespace": "ci" }
  ]
}
```

There are no unit tests for this project.

## Deploying with Helm

```bash
helm repo add keepup-helm-scraper https://code-tool.github.io/keepup-helm-scraper/
```

Set the mandatory variables:

```yaml
env:
  CLUSTER_NAME: 'unique-name-for-metrics-labels'
  TEAM: 'team-owning-this-cluster'
  API_URL: 'https://keepup.host/helm-cluster'
  API_TOKEN: 'api-token-to-access-the-API_URL'
```

Deploy:

```bash
helm install keepup-helm-scraper keepup-helm-scraper/keepup-helm-scraper -f values-override.yaml
```

The chart bundles a `ConfigMap` with the default detection rules (`templates/configmap.yaml`), a `Secret` built from every key under `env:` (`templates/secret.yaml`, injected via `envFrom`), and RBAC (`ClusterRole`/`ClusterRoleBinding`) so the scraper can list resources across all namespaces. The CronJob schedule and job history limits are controlled by `cronjob.*` in `values.yaml` (default: every 3 hours).

## Configuration

All variables below are required — the process panics at startup if any is missing from the environment (or from `.env` in dev mode, when `APP_ENV` is unset).

| Variable | Example | Purpose |
|---|---|---|
| `APP_ENV` | `dev` | when unset, triggers `.env` file load |
| `API_URL` | `https://keepup.host/helm-cluster` | `PUT` endpoint receiving the cluster payload |
| `API_TOKEN` | `secret` | sent as the `x-api-token` header |
| `CLUSTER_NAME` | `prod-eu` | reported as `cluster_name`; falls back to `minikube` if unset at runtime |
| `TEAM` | `platform` | reported as `team`; empty string if unset at runtime |
| `RULES_FILE` | `./keepup-detection.yaml` | path to the detection rules YAML (defaults to `./keepup-detection.yaml`) |

## Detection rules

Rules live under the `docker:` key, either in `keepup-detection.yaml` (local dev) or inlined in the Helm chart's `ConfigMap` (`charts/keepup-helm-scraper/templates/configmap.yaml`) for cluster deployments — keep both in sync when adding a rule.

Each rule has:

- `applicationName` — canonical name reported as `chart_name`
- `detectionRegex` — matched against the full image string to identify the application
- `versionRegex` — extracts the raw version substring from the image string; a generic `major.minor(.patch)` regex then pulls the numbers out of it

```yaml
# f/e registry.gitlab.com/gitlab-org/gitlab-runner:alpine-v18.1.3
- applicationName: 'gitlab-runner'
  detectionRegex: '\/gitlab-runner:'
  versionRegex: ':([A-Za-z][A-Za-z0-9-]*-)?(v)?(\d+)\.(\d+)(\.(\d+))?((@sha)?.*)?$'
```

The `versionRegex` above tolerates an arbitrary alphanumeric prefix before the version (e.g. `alpine-`, `ubi-fips-`) as well as a plain `vX.Y.Z` or bare `X.Y.Z` tag, and ignores any trailing `@sha256:...` digest.

## Local development

Requires `.env` and a rules file (`keepup-detection.yaml`) in the working directory, plus a reachable Kubernetes API (via in-cluster config or a local proxy).

```bash
cd src && go run main.go
```

## Build

```bash
# binary
go build -o helm-scraper src/main.go

# Docker image
docker build -t ghcr.io/code-tool/keepup-helm-scraper:$(cat VERSION.txt) -f docker/Dockerfile .
```

Docker images are built and pushed to `ghcr.io/code-tool/keepup-helm-scraper` (multi-platform: `linux/amd64`, `linux/arm64`) on every tag push. The Helm chart (`charts/keepup-helm-scraper/`) is released via `helm/chart-releaser-action` on every push to `main` — chart version and app version are maintained independently in `Chart.yaml`.
