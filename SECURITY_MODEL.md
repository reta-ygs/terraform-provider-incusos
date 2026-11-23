# Security threat model for terraform-provider-incusos

## System and scope

The `registry.terraform.io/reta-ygs/incusos` Terraform provider runs as a plugin on an operator workstation or automation runner. Terraform calls the plugin to read and change the IncusOS host OS. The plugin normally connects through the Incus HTTPS proxy under `/os/1.0`. It also supports directly addressing the IncusOS fallback listener. The IncusOS daemon enforces host-side authorization. The provider repository contains the client, resource mappings, CI, release workflow, and example Terraform configurations. This model covers the provider repository. IncusOS server internals, consumer Terraform backends and CI configuration, and organization-level GitHub settings are deployment dependencies outside this repository. See `README.md` and `main.go`.

## Components

| Component | Scope |
| --- | --- |
| Provider configuration | `main.go`, `internal/provider/provider.go`. Plugin entry point, provider schema, endpoint and certificate selection from HCL, environment, and local Incus configuration. |
| IncusOS API client | `internal/client/`. The outbound HTTP transport, TLS trust, API request encoding and response parsing. |
| Terraform resources and state | `internal/provider/resource_*.go`, `internal/provider/data_sources.go`, `internal/provider/helpers.go`, `docs/`, and `examples/`. Resource lifecycles, data sources, plans, refresh, state, and diagnostics. |
| Build and release | `.github/workflows/`, `.goreleaser.yml`, `go.mod`, `go.sum`, `GNUmakefile`, `renovate.json`, and `terraform-registry-manifest.json`. Pull request checks, tool and dependency intake, and distribution of the signed provider release. |

## Trust boundaries and reachability

- **Terraform to plugin and local filesystem.** The Terraform process supplies provider and resource configuration through the Plugin Framework. Provider settings resolve from HCL, `INCUSOS_*` environment variables, then local Incus remote configuration for missing connection fields when a lookup is performed. The runner's identity controls file permissions and the provider uses it to read configured PEM files and local Incus config. Access to a consumer repository, pipeline variables, runner files, and IncusOS credentials are distinct privileges. The provider has no user account or public application entry point. See `main.go` and `internal/provider/provider.go`.
- **Plugin to IncusOS and back.** The plugin sends JSON requests whose target and credentials come from the effective provider settings. Intentional mutations include network confirmation, storage lifecycle, host services, security configuration, updates, and application deployment. When HTTPS is used, network intermediaries and the remote host can influence connection behavior or response bytes subject to TLS authentication; when an HTTP endpoint is configured, the client does not restrict HTTP to local hosts, so remotely reachable HTTP endpoints expose management requests, responses, and secrets over plaintext without TLS. API responses feed resource refresh, data sources, and error diagnostics. See `internal/client/client.go`, `internal/provider/data_sources.go`, and `internal/provider/resource_system_network.go`.
- **Plugin to plans, state, and diagnostics.** The deployment controls who can read plan artifacts, state and backups, and CI logs. Some resource writes and refreshes carry sensitive values into Terraform state. A schema's `Sensitive` flag controls Terraform's standard display of the value. Backend storage, access control and retention belong to the deployment. See `internal/provider/resource_system_security.go`, `internal/provider/resource_system_network.go`, `internal/provider/resource_system_logging.go`, and `internal/provider/resource_services_*.go`.
- **Source and dependencies to CI and release.** Pull requests can trigger test and analysis workflows. Matching `v*` tag pushes trigger a release job that can write release contents and imports the configured GPG signing key. Terraform starts the plugin on the consumer's runner, where the plugin can interact with the runner's credentials and IncusOS. See `.github/workflows/{test,lint,codeql,release}.yml` and `.goreleaser.yml`.

## Assets and plausible attackers

| Asset | Security consequence |
| --- | --- |
| IncusOS client TLS identity and endpoint trust | Unauthorized host management or disclosure of management requests and responses. |
| Terraform plan/state, service credentials, and disk recovery material | Exposure of WireGuard and other service secrets, recovery passphrases, or private deployment details. |
| Managed hosts and local storage | Unintended changes to host access, trust anchors, updates, and attached services, or loss of storage data or availability. |
| Provider binaries and signing identity | A compromised release would run under downstream Terraform runner identities. |

- An untrusted contributor may control a pull request. Whether consumer CI lets such a contributor influence a Terraform run with host credentials is unverified.
- A network-positioned actor may influence DNS, a proxy, or traffic. Which paths they can reach depends on network topology, the configured endpoint and proxy, and the effective TLS settings. A compromised or incorrectly selected IncusOS host can control its own API responses.
- A runner peer or state or log reader may gain access to files or artifacts if granted by the operator or CI configuration. Reading state, writing runner configuration, and possessing an IncusOS client private key are different capabilities.
- A compromised dependency, CI component, or identity with release privileges could affect provider integrity. GitHub tag and signing protections depend on the repository settings.

## Existing controls

- The API client sets TLS 1.2 as its minimum, verifies the server by default, and supports a configured CA or direct fingerprint pinning for a server leaf certificate. It checks that client certificate and key are supplied together. There is an explicit verification override, and although HTTP is intended for local testing, the client accepts any HTTP endpoint without a locality check; using a remotely reachable HTTP endpoint sends management traffic in plaintext without TLS. See `internal/client/client.go`.
- Terraform schemas mark fields such as the inline client key, encryption recovery keys, and multiple network and storage service secrets as sensitive. The provider exposes network change confirmation as a separate API operation under the resource's `auto_confirm` setting. See `internal/provider/provider.go`, `internal/provider/resource_system_security.go`, and `internal/provider/resource_system_network.go`.
- GitHub workflows use pinned Action revisions and scoped job permissions. GoReleaser signs SHA-256 release checksums. The repository runs CodeQL, lint and dependency vulnerability checks and documents private advisory reporting. See `.github/workflows/`, `.goreleaser.yml`, and `SECURITY.md`.

## Deployment assumptions and open questions

Trusted operators or CI are the intended users. Credentials, Terraform state, host configuration changes, and release integrity are all in scope. This model assumes no deployment-specific attacker exclusions or accepted risks.

Whether less-trusted contributors can influence credentialed runs depends on the deployment. Production network exposure, credential scope, state backend access, and protections around release tags remain unverified.
