terraform {
  required_providers {
    incusos = {
      source = "reta-ygs/incusos"
    }
  }
}

provider "incusos" {
  address          = var.incusos_address
  client_cert_path = var.client_cert_path
  client_key_path  = var.client_key_path
  server_cert_path = var.server_cert_path
}

variable "incusos_address" {
  type = string
}

variable "client_cert_path" {
  type    = string
  default = "~/.config/incus/client.crt"
}

variable "client_key_path" {
  type    = string
  default = "~/.config/incus/client.key"
}

variable "server_cert_path" {
  type = string
}

# Read host hardware & status
data "incusos_server" "this" {}
data "incusos_system_storage" "this" {}
data "incusos_system_security" "this" {}

# Configure OS-level networking (DNS, NTP, interfaces)
resource "incusos_system_network" "this" {
  confirmation_timeout = "60s"
  auto_confirm         = true

  dns = {
    hostname    = "node-01"
    domain      = "example.internal"
    nameservers = ["1.1.1.1", "8.8.8.8"]
  }

  time = {
    timezone    = "Asia/Tokyo"
    ntp_servers = ["ntp.nict.jp"]
  }

  interfaces = [
    {
      name      = "enp1s0"
      hwaddr    = "00:11:22:33:44:55"
      addresses = ["192.0.2.10/24"]
      roles     = ["management", "instances"]
      mtu       = 1500
    }
  ]
}

# Configure OS-level ZFS pool and volume
resource "incusos_system_storage" "schedules" {
  scrub_schedule = "0 3 * * 0"
  trim_schedule  = "0 4 * * 0"
}

resource "incusos_system_storage_pool" "local" {
  name    = "local"
  type    = "zfs-raid1"
  devices = [
    "/dev/disk/by-id/nvme-disk-1",
    "/dev/disk/by-id/nvme-disk-2",
  ]
}

resource "incusos_system_storage_volume" "incus" {
  pool = incusos_system_storage_pool.local.name
  name = "incus-default"
  use  = "incus"
}

# Configure kernel tuning & OS updates
resource "incusos_system_kernel" "this" {
  network = {
    tcp_congestion_algorithm = "bbr"
    tcp_mtu_probing          = true
  }
  zfs = {
    arc_max_size = "16GiB"
  }
}

resource "incusos_system_update" "this" {
  channel         = "stable"
  check_frequency = "6h"
  auto_reboot     = false
  maintenance_windows = [
    {
      start_day_of_week = "Sunday"
      start_hour        = 2
      start_minute      = 0
      end_day_of_week   = "Sunday"
      end_hour          = 5
      end_minute        = 0
    }
  ]
}

# Enable optional IncusOS services
resource "incusos_service_tailscale" "this" {
  enabled       = true
  accept_routes = true
  auth_key      = "tskey-auth-replace-me"
}

# Install optional IncusOS applications
resource "incusos_application" "gpu_support" {
  name                 = "gpu-support"
  pin_current_versions = false
}

output "os_version" {
  value = data.incusos_server.this.os_version
}

output "tpm_status" {
  value = data.incusos_system_security.this.tpm_status
}
