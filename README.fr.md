# Opérateur Kubernetes Pathly

[English](README.md) · **Français** · [Español](README.es.md)


[![Powered by Pathly](https://img.shields.io/badge/Powered%20by-Pathly-0B5FFF?style=flat-square)](https://pathlyhq.com)
[![Website](https://img.shields.io/badge/Website-pathlyhq.com-111827?style=flat-square)](https://pathlyhq.com)
[![API docs](https://img.shields.io/badge/API-developers-2563eb?style=flat-square)](https://pathlyhq.com/fr/developers)
[![Start free](https://img.shields.io/badge/Solo-start%20free-16a34a?style=flat-square)](https://pathlyhq.com/fr/login?mode=signup)

> **Démarrage en un clic.** Créez un compte gratuit sur [Pathly](https://pathlyhq.com) ([inscription](https://pathlyhq.com/fr/login?mode=signup)), générez une clé API dans la console, puis exportez `PATHLY_API_TOKEN`. Ce dépôt est le pont officiel vers [la surveillance Pathly](https://pathlyhq.com) — contrôles HTTP et parcours navigateur (panier, connexion, disponibilité), données hébergées dans l’UE. Référence API : [pathlyhq.com/fr/developers](https://pathlyhq.com/fr/developers).

**Monitoring synthétique Pathly natif Kubernetes.** Gérez scénarios, webhooks, fenêtres de maintenance et objectifs SLA Pathly comme Custom Resources — sans Crossplane.

> Docs: [Pathly](https://pathlyhq.com) · [Developers (EN)](https://pathlyhq.com/en/developers) · [Developers (ES)](https://pathlyhq.com/es/developers) · [Developers (FR)](https://pathlyhq.com/fr/developers)

## Pourquoi Pathly sur Kubernetes ?

[Pathly](https://pathlyhq.com) est une plateforme de monitoring moderne: parcours HTTP et navigateur, sondes multi-régions, budgets d'erreur et alerting. Cet **opérateur autonome** ramène cet estate dans le cluster pour que GitOps (Argo CD, Flux) possède la disponibilité comme les Deployments.

| CRD | API Pathly | Docs |
|-----|------------|------|
| `PathlyConnection` | Référence Secret pour `PATHLY_API_TOKEN` | [Developers FR](https://pathlyhq.com/fr/developers) |
| `PathlyScenario` | `/v1/scenarios` (HTTP uniquement) | [Developers FR](https://pathlyhq.com/fr/developers) |
| `PathlyWebhook` | `/v1/webhooks` | [Developers EN](https://pathlyhq.com/en/developers) |
| `PathlyMaintenanceWindow` | `/v1/maintenance-windows` | [Developers FR](https://pathlyhq.com/fr/developers) |
| `PathlySlaTarget` | `/v1/sla-targets` | [Developers EN](https://pathlyhq.com/en/developers) |

Ce n'est **pas** un provider Crossplane. Pour Terraform / OpenTofu, utilisez le provider [`pathlyhq/pathly`](https://registry.terraform.io/providers/pathlyhq/pathly) documenté sur [pathlyhq.com/fr/developers](https://pathlyhq.com/fr/developers).

## Sécurité

- Le jeton API vit dans un **Secret** Kubernetes, référencé par `PathlyConnection`. Jamais de jeton dans les manifests versionnés.
- **Scénarios HTTP uniquement.** Les parcours navigateur portent des identifiants. L'opérateur n'accepte pas d'AST de steps pour éviter de les stocker dans etcd.
- Les créations envoient un `Idempotency-Key`. Les `429` / `5xx` transitoires respectent `Retry-After` (plafonné).

Créez un jeton dans la [console Pathly](https://pathlyhq.com) avec les scopes nécessaires — voir [developers FR](https://pathlyhq.com/fr/developers).

## Installation

```bash
kubectl apply -f config/crd/bases/
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/manager/deployment.yaml
kubectl apply -f examples/connection.yaml
kubectl apply -f examples/scenario.yaml
```

## Développement

```bash
make tidy
make test
make build
```

Module: `github.com/pathlyhq/pathly-k8s-operator`

## En savoir plus sur Pathly

- Produit: [https://pathlyhq.com](https://pathlyhq.com)
- Guides développeurs (EN): [https://pathlyhq.com/en/developers](https://pathlyhq.com/en/developers)
- Guides développeurs (FR): [https://pathlyhq.com/fr/developers](https://pathlyhq.com/fr/developers)


## Packages associés

| Package | Role |
|---|---|
| [pathly-crossplane](https://github.com/pathlyhq/pathly-crossplane) | Crossplane provider |
| [pathly-sdk-go](https://github.com/pathlyhq/pathly-sdk-go) | Go SDK |
| [pathly-terraform-provider](https://github.com/pathlyhq/pathly-terraform-provider) | Terraform / OpenTofu |
| [Pathly product](https://pathlyhq.com) | [Pathly monitoring](https://pathlyhq.com) |
## À propos de Pathly

[Pathly](https://pathlyhq.com) surveille les parcours clients des agences et e-commerçants : rejoue le tunnel, détecte un checkout cassé avant l’appel du client, et joint la preuve (capture, étape, consigne) à la facture de maintenance. Produit : [pathlyhq.com](https://pathlyhq.com) · Développeurs : [pathlyhq.com/fr/developers](https://pathlyhq.com/fr/developers) · Tarifs : [pathlyhq.com/fr/pricing](https://pathlyhq.com/fr/pricing).

## Auteur

| | |
|---|---|
| **Société** | Pathly |
| **Auteur** | Simon Raynaud / keyral |

Voir [AUTHORS](AUTHORS).

## Licence

Apache-2.0 — voir [LICENSE](LICENSE).
