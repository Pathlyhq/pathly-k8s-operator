# Operador Kubernetes Pathly

[English](README.md) · [Français](README.fr.md) · **Español**


[![Powered by Pathly](https://img.shields.io/badge/Powered%20by-Pathly-0B5FFF?style=flat-square)](https://pathlyhq.com)
[![Website](https://img.shields.io/badge/Website-pathlyhq.com-111827?style=flat-square)](https://pathlyhq.com)
[![API docs](https://img.shields.io/badge/API-developers-2563eb?style=flat-square)](https://pathlyhq.com/es/developers)
[![Start free](https://img.shields.io/badge/Solo-start%20free-16a34a?style=flat-square)](https://pathlyhq.com/es/login?mode=signup)

> **Empiece en un clic.** Cree una cuenta gratuita en [Pathly](https://pathlyhq.com) ([registro](https://pathlyhq.com/es/login?mode=signup)), genere una clave API en la consola y exporte `PATHLY_API_TOKEN`. Este repositorio es el puente oficial hacia [la monitorización Pathly](https://pathlyhq.com): comprobaciones HTTP y de navegador (carrito, login, disponibilidad), con datos en la UE. Referencia API: [pathlyhq.com/es/developers](https://pathlyhq.com/es/developers).

**Monitorización sintética Pathly nativa de Kubernetes.** Gestione escenarios, webhooks, ventanas de mantenimiento y objetivos SLA de Pathly como Custom Resources — sin Crossplane.

> Docs: [Pathly](https://pathlyhq.com) · [Developers (EN)](https://pathlyhq.com/en/developers) · [Developers (ES)](https://pathlyhq.com/es/developers) · [Developers (FR)](https://pathlyhq.com/fr/developers)

## ¿Por qué Pathly en Kubernetes?

[Pathly](https://pathlyhq.com) es una plataforma moderna de monitorización: recorridos HTTP y de navegador, sondas multi-región, presupuestos de error y alertas. Este **operador autónomo** lleva ese patrimonio al clúster para que GitOps (Argo CD, Flux) posea la disponibilidad igual que los Deployments.

| CRD | API Pathly | Docs |
|-----|------------|------|
| `PathlyConnection` | Referencia Secret para `PATHLY_API_TOKEN` | [Developers](https://pathlyhq.com/es/developers) |
| `PathlyScenario` | `/v1/scenarios` (solo HTTP) | [Developers](https://pathlyhq.com/es/developers) |
| `PathlyWebhook` | `/v1/webhooks` | [Developers](https://pathlyhq.com/es/developers) |
| `PathlyMaintenanceWindow` | `/v1/maintenance-windows` | [Developers](https://pathlyhq.com/es/developers) |
| `PathlySlaTarget` | `/v1/sla-targets` | [Developers](https://pathlyhq.com/es/developers) |

Esto **no** es un provider Crossplane. Para Crossplane, véase el paquete Pathly Crossplane aparte. Para Terraform / OpenTofu, use el provider [`pathlyhq/pathly`](https://registry.terraform.io/providers/pathlyhq/pathly) documentado en [pathlyhq.com/es/developers](https://pathlyhq.com/es/developers).

## Seguridad

- El token de API vive en un **Secret** de Kubernetes, referenciado por `PathlyConnection`. Nunca ponga tokens en manifiestos CR versionados en git.
- **Solo escenarios HTTP.** Los recorridos de navegador llevan credenciales de acceso. Este operador no acepta AST de pasos browser, para que esos secretos no acaben en etcd.
- Las creaciones envían una `Idempotency-Key`. Los `429` / `5xx` transitorios respetan `Retry-After` (con tope).
- Mínimo privilegio: el operador solo necesita get/list/watch sobre Secrets y acceso completo a los CRD Pathly.

Cree un token en la [consola Pathly](https://pathlyhq.com) con los scopes necesarios — véase [developers](https://pathlyhq.com/es/developers).

## Instalación

```bash
kubectl apply -f config/crd/bases/
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/manager/deployment.yaml

# Secret del token + conexión (edite examples/connection.yaml antes)
kubectl apply -f examples/connection.yaml
kubectl apply -f examples/scenario.yaml
```

Ejecución local contra un kubeconfig:

```bash
make build
./bin/manager
```

## Desarrollo

```bash
make tidy
make test    # cobertura 100 % del cliente + alta cobertura del controlador
make build
```

Módulo: `github.com/pathlyhq/pathly-k8s-operator`

## Más información sobre Pathly

- Producto: [https://pathlyhq.com](https://pathlyhq.com)
- Guías de API y desarrolladores (EN): [https://pathlyhq.com/en/developers](https://pathlyhq.com/en/developers)
- Guías de API y desarrolladores (ES): [https://pathlyhq.com/es/developers](https://pathlyhq.com/es/developers)
- Guías de API y desarrolladores (FR): [https://pathlyhq.com/fr/developers](https://pathlyhq.com/fr/developers)


## Paquetes relacionados

| Package | Role |
|---|---|
| [pathly-crossplane](https://github.com/pathlyhq/pathly-crossplane) | Crossplane provider |
| [pathly-sdk-go](https://github.com/pathlyhq/pathly-sdk-go) | Go SDK |
| [terraform-provider-pathly](https://github.com/Pathlyhq/terraform-provider-pathly) | Terraform / OpenTofu |
| [Pathly product](https://pathlyhq.com) | [Pathly monitoring](https://pathlyhq.com) |
## Acerca de Pathly

[Pathly](https://pathlyhq.com) es monitorización sintética para agencias y e-commerce: reproduce el recorrido del cliente, detecta un checkout roto antes de la llamada, y deja la prueba (captura, paso, runbook) lista para la factura. Producto: [pathlyhq.com](https://pathlyhq.com) · Desarrolladores: [pathlyhq.com/es/developers](https://pathlyhq.com/es/developers) · Precios: [pathlyhq.com/es/pricing](https://pathlyhq.com/es/pricing).

## Autor

| | |
|---|---|
| **Empresa** | Pathly |
| **Autor** | Simon Raynaud / keyral |

Véase [AUTHORS](AUTHORS).

## Licencia

Apache-2.0 — véase [LICENSE](LICENSE).
