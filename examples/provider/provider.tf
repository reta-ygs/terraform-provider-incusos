terraform {
  required_providers {
    incusos = {
      source = "reta-ygs/incusos"
    }
  }
}

# Option 1 (Recommended for teams):
# Keep the provider block empty and inject INCUS_REMOTE=node-01 (uses ~/.config/incus or INCUS_CONF)
# or INCUSOS_ADDRESS / INCUSOS_CLIENT_CERT / INCUSOS_CLIENT_KEY / INCUSOS_SERVER_CERT in CI/CD.
provider "incusos" {}

# Option 2: Explicitly select an Incus CLI remote
# provider "incusos" {
#   remote = "node-01"
# }

# Option 3: Explicit HTTPS + mTLS parameters
# provider "incusos" {
#   address          = "https://192.0.2.10:8443"
#   path_prefix      = "/os"
#   client_cert_path = "~/.config/incus/client.crt"
#   client_key_path  = "~/.config/incus/client.key"
#   server_cert_path = "~/.config/incus/servercerts/node-01.crt"
# }
