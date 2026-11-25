# Terraform Provider for IncusOS

`terraform-provider-incusos` は、イミュータブルな Linux OS である **[IncusOS](https://github.com/lxc/incus-os)** (`incus-osd`) の OS レイヤー設定を Terraform で宣言的に管理するための Provider です。

## `terraform-provider-incus` との役割分担

- **[`lxc/terraform-provider-incus`](https://github.com/lxc/terraform-provider-incus)**: Incus デーモン (`/1.0/...`) 上のインスタンス・プロファイル・プロジェクト・ネットワーク・ストレージ等を管理
- **`terraform-provider-incusos`（本リポジトリ）**: `terraform-provider-incus` が管理しない **IncusOS ホスト OS 層 (`/os/1.0/...`)**（ホストネットワーク、ZFS プール/ボリューム、カーネルチューニング、ログ、電源管理、セキュリティ、OS アップデート、フォールバックリスナー、システムサービス、アプリケーション）を管理

API 型定義には `github.com/lxc/incus-os/incus-osd/api` を直接 import し、[Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework) で実装しています。

## 接続方式

IncusOS のリモート管理 API は **HTTPS + クライアント TLS 証明書認証 (mTLS)** でアクセスします（デフォルトは Incus HTTPS プロキシ経由の `/os/1.0/...`、または Fallback Listener 経由）。

接続設定は AWS Provider と同様に **HCL 明示指定 > `INCUSOS_*` 環境変数 > Incus CLI 設定 (`~/.config/incus` / `INCUS_CONF` + `INCUS_REMOTE`)** の優先順位で解決されます。

### チーム開発・ローカル運用（`INCUS_REMOTE` による注入）

Incus / IncusOS の公式 Go 実装 (`github.com/lxc/incus/v7/shared/cliconfig`) と同じ環境変数 `INCUS_REMOTE` / `INCUS_CONF` に対応しています。

```hcl
# .tf 側にはローカルパスや認証情報を書かない
provider "incusos" {}
```

```bash
# ローカルの ~/.config/incus からリモート名を指定して実行
INCUS_REMOTE=node-01 terraform plan
```

### CI/CD・明示指定

```hcl
provider "incusos" {
  address          = "https://192.0.2.10:8443"
  path_prefix      = "/os" # デフォルト
  client_cert_path = "~/.config/incus/client.crt"
  client_key_path  = "~/.config/incus/client.key"
  server_cert_path = "~/.config/incus/servercerts/node-01.crt"
}
```

対応する環境変数:
- リモート解決: `INCUSOS_REMOTE`, `INCUS_REMOTE`, `INCUS_CONF`, `INCUS_GLOBAL_CONF`
- 直接指定 / 上書き: `INCUSOS_ADDRESS`, `INCUSOS_PATH_PREFIX`, `INCUSOS_CLIENT_CERT`, `INCUSOS_CLIENT_KEY`, `INCUSOS_CLIENT_CERT_PATH`, `INCUSOS_CLIENT_KEY_PATH`, `INCUSOS_SERVER_CERT`, `INCUSOS_SERVER_CERT_PATH`, `INCUSOS_INSECURE_SKIP_VERIFY`, `INCUSOS_TIMEOUT`, `INCUSOS_NETWORK_CONFIRMATION_TIMEOUT`

## 提供リソース一覧

### System (`/1.0/system/*`)
| リソース | 説明 |
|---|---|
| `incusos_system_network` | ホストネットワーク設定（DNS, NTP/Timezone, Proxy, Interfaces, Bonds, VLANs, WireGuard, `auto_confirm`） |
| `incusos_system_storage` | ホストストレージの定期メンテナンススケジュール（`scrub_schedule`, `trim_schedule`） |
| `incusos_system_storage_pool` | ローカル ZFS ストレージプールの作成・更新・削除 |
| `incusos_system_storage_volume` | ストレージプール上の IncusOS 管理ボリュームの作成・削除 |
| `incusos_system_kernel` | カーネル設定（blacklist, console, CPU, memory/hugepages/zram, network sysctl, PCI passthrough/SR-IOV, ZFS ARC） |
| `incusos_system_logging` | リモート syslog / systemd-journal-upload 設定 |
| `incusos_system_power` | CPU スケーリングガバナー・EPP 設定 |
| `incusos_system_provider` | アップデート/構成プロバイダー設定（`images`, `operations-center` 等） |
| `incusos_system_security` | カスタム CA 証明書・ディスク暗号化リカバリキー設定 |
| `incusos_system_update` | OS アップデートポリシー（channel, check_frequency, auto_reboot, maintenance_windows） |
| `incusos_system_fallback_listener` | フォールバック HTTPS リスナー設定 |

### Applications (`/1.0/applications/*`)
| リソース | 説明 |
|---|---|
| `incusos_application` | IncusOS アプリケーションのインストール・バージョン固定・削除（`gpu-support`, `debug`, `incus` 等） |

### Services (`/1.0/services/*`)
| リソース | 説明 |
|---|---|
| `incusos_service_ceph` | Ceph クライアントサービス |
| `incusos_service_iscsi` | iSCSI イニシエーターサービス |
| `incusos_service_linstor` | LINSTOR サテライトサービス |
| `incusos_service_lvm` | 共有 LVM (lvmlockd/sanlock) サービス |
| `incusos_service_multipath` | Multipath サービス |
| `incusos_service_netbird` | NetBird サービス |
| `incusos_service_nvme` | NVMe-oF サービス |
| `incusos_service_ovn` | OVN サービス |
| `incusos_service_tailscale` | Tailscale サービス |
| `incusos_service_usbip` | USB/IP サービス |

## 提供データソース一覧

| データソース | 説明 |
|---|---|
| `incusos_server` | `GET /1.0` — ホスト名、Machine ID、OS バージョン、稼働時間 |
| `incusos_system_storage` | `GET /1.0/system/storage` — 物理ドライブ一覧、ルートパーティション使用量、プール一覧 |
| `incusos_system_security` | `GET /1.0/system/security` — Secure Boot / TPM / 暗号化ボリューム状態 |
| `incusos_system_network` | `GET /1.0/system/network` — ネットワークインターフェース実行時状態 |
| `incusos_system_resources` | `GET /1.0/system/resources` — ハードウェアリソース情報（JSON） |
| `incusos_applications` | `GET /1.0/applications` — インストール済みアプリケーション一覧 |
| `incusos_services` | `GET /1.0/services` — 利用可能サービス一覧 |

## ビルド・テスト

```bash
make build
make test
```
