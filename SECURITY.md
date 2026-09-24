# Security Policy

## Supported versions

Security fixes are applied on the latest `main` branch of this operator.

## Reporting a vulnerability

Report dependency or code vulnerabilities to **security@pathlyhq.com**.

For privacy or personal-data concerns, contact **privacy@pathlyhq.com**.

Please include the repository name (`pathly-k8s-operator`), affected version or commit, and steps to reproduce. Do not open a public issue for unfixed vulnerabilities.

## Credentials

- Store the Pathly API token in a Kubernetes Secret referenced by `PathlyConnection`.
- Never commit real `PATHLY_API_TOKEN` values.
- Create and rotate tokens in the [Pathly console](https://pathlyhq.com) — see [developers](https://pathlyhq.com/en/developers).

## HTTP scenarios only

This operator manages **HTTP** scenarios only. Browser journeys may embed credentials; those must not land in etcd. Manage browser scenarios in the [Pathly console](https://pathlyhq.com) or via the API with a short-lived token.
