# Pathly Kubernetes Operator

**English** · [Français](README.fr.md) · [Español](README.es.md)


[![Powered by Pathly](https://img.shields.io/badge/Powered%20by-Pathly-0B5FFF?style=flat-square)](https://pathlyhq.com)
[![Website](https://img.shields.io/badge/Website-pathlyhq.com-111827?style=flat-square)](https://pathlyhq.com)
[![API docs](https://img.shields.io/badge/API-developers-2563eb?style=flat-square)](https://pathlyhq.com/en/developers)
[![Start free](https://img.shields.io/badge/Solo-start%20free-16a34a?style=flat-square)](https://pathlyhq.com/en/login?mode=signup)

> **Get started in one click.** Create a free account on [Pathly](https://pathlyhq.com) ([sign up](https://pathlyhq.com/en/login?mode=signup)), create an API key in the console, then export `PATHLY_API_TOKEN`. This project is the official bridge to [Pathly monitoring](https://pathlyhq.com) — real-browser and HTTP checks for checkout, login and availability, with data hosted in the EU. Full API reference: [pathlyhq.com/en/developers](https://pathlyhq.com/en/developers).

**Kubernetes-native synthetic monitoring for [Pathly](https://pathlyhq.com).** Manage Pathly scenarios, webhooks, maintenance windows and SLA targets as Custom Resources — without Crossplane.

> Docs: [Pathly](https://pathlyhq.com) · [Developers (EN)](https://pathlyhq.com/en/developers) · [Developers (ES)](https://pathlyhq.com/es/developers) · [Developers (FR)](https://pathlyhq.com/fr/developers)

## Why Pathly on Kubernetes?

[Pathly](https://pathlyhq.com) is a modern monitoring platform: HTTP and browser journeys, multi-region probes, error budgets and alerting. This **standalone operator** brings that estate into your cluster so GitOps (Argo CD, Flux) can own uptime the same way it owns Deployments.

| CRD | Pathly API | Docs |
|-----|------------|------|
| `PathlyConnection` | Secret ref for `PATHLY_API_TOKEN` | [Developers](https://pathlyhq.com/en/developers) |
| `PathlyScenario` | `/v1/scenarios` (HTTP only) | [Developers](https://pathlyhq.com/en/developers) |
| `PathlyWebhook` | `/v1/webhooks` | [Developers](https://pathlyhq.com/en/developers) |
| `PathlyMaintenanceWindow` | `/v1/maintenance-windows` | [Developers](https://pathlyhq.com/fr/developers) |
| `PathlySlaTarget` | `/v1/sla-targets` | [Developers](https://pathlyhq.com/en/developers) |

This is **not** a Crossplane provider. For Crossplane, see the separate Pathly Crossplane package. For Terraform / OpenTofu, use the [`pathlyhq/pathly`](https://registry.terraform.io/providers/pathlyhq/pathly) provider documented on [pathlyhq.com/en/developers](https://pathlyhq.com/en/developers).

## Security

- The API token lives in a Kubernetes **Secret**, referenced by `PathlyConnection`. Never put tokens in CR manifests committed to git.
- **HTTP scenarios only.** Browser journeys carry login credentials. This operator does not accept browser step ASTs so those secrets never land in etcd.
- Creates send an `Idempotency-Key`. Transient `429` / `5xx` honour `Retry-After` (capped).
- Least privilege: the operator only needs get/list/watch on Secrets and full access to Pathly CRDs.

Create a token in the [Pathly console](https://pathlyhq.com) with the scopes you need — see [developers](https://pathlyhq.com/en/developers).

## Install

```bash
kubectl apply -f config/crd/bases/
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/manager/deployment.yaml

# Token Secret + connection (edit examples/connection.yaml first)
kubectl apply -f examples/connection.yaml
kubectl apply -f examples/scenario.yaml
```

Run locally against a kubeconfig:

```bash
make build
./bin/manager
```

## Develop

```bash
make tidy
make test    # client 100% coverage target + high controller coverage
make build
```

Module: `github.com/pathlyhq/pathly-k8s-operator`

## Learn more about Pathly

- Product: [https://pathlyhq.com](https://pathlyhq.com)
- API & developer guides (EN): [https://pathlyhq.com/en/developers](https://pathlyhq.com/en/developers)
- API & developer guides (FR): [https://pathlyhq.com/fr/developers](https://pathlyhq.com/fr/developers)


## Related packages

| Package | Role |
|---|---|
| [pathly-crossplane](https://github.com/pathlyhq/pathly-crossplane) | Crossplane provider |
| [pathly-sdk-go](https://github.com/pathlyhq/pathly-sdk-go) | Go SDK |
| [pathly-terraform-provider](https://github.com/pathlyhq/pathly-terraform-provider) | Terraform / OpenTofu |
| [Pathly product](https://pathlyhq.com) | [Pathly monitoring](https://pathlyhq.com) |
## About Pathly

[Pathly](https://pathlyhq.com) is synthetic monitoring for agencies and e-commerce: replay the customer journey, catch broken checkouts before your clients call, and keep evidence (screenshot, step, runbook) ready for the invoice. Product: [pathlyhq.com](https://pathlyhq.com) · Developers: [pathlyhq.com/en/developers](https://pathlyhq.com/en/developers) · Status & pricing: [pathlyhq.com/en/pricing](https://pathlyhq.com/en/pricing).

## Author

| | |
|---|---|
| **Company** | Pathly |
| **Author** | Simon Raynaud / keyral |

See [AUTHORS](AUTHORS).

## License

Apache-2.0 — see [LICENSE](LICENSE).
