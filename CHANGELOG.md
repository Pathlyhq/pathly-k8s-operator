# Changelog

All notable changes to the Pathly Kubernetes Operator are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-03-23

### Added

- Standalone Kubernetes operator for [Pathly](https://pathlyhq.com) monitoring (not Crossplane).
- CRDs: `PathlyConnection`, `PathlyScenario`, `PathlyWebhook`, `PathlyMaintenanceWindow`, `PathlySlaTarget`.
- Controllers that reconcile against the Pathly public API (`https://api.pathlyhq.com`).
- API token resolved from a Kubernetes Secret via `PathlyConnection` (never from CR specs).
- Examples, Makefile, GitLab CI, README EN/FR.

See also: [Pathly developers (EN)](https://pathlyhq.com/en/developers) · [Pathly developers (FR)](https://pathlyhq.com/fr/developers).
