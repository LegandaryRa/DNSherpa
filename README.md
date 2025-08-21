# DNSherpa

**Automatically manage DNS records for your Docker services and Proxmox VMs!**

When you deploy a Docker service with Traefik labels or manage VMs in Proxmox, DNSherpa automatically creates DNS records so your domains work instantly. No more manual DNS management!

## ✨ What It Does

🔄 **Monitors your Docker containers** and watches for Traefik labels  
🏷️ **Finds domain names** in your `Host()` rules  
🚀 **Discovers Proxmox VMs/containers** and creates DNS records from VM names
📝 **Creates DNS records** automatically in your DNS server with A and AAAA support

## 🎯 Perfect For

- **Home Labs**: Automatic DNS for your self-hosted services
- **Development**: No more editing `/etc/hosts` files  
- **Small Teams**: Set-and-forget DNS management
- **Docker Compose**: Works with all your existing services

## 📋 What You Need

- Docker services with Traefik labels (for Docker agent mode)
- Proxmox VE cluster with API tokens (for Proxmox agent mode)  
- An etcd server (for DNS storage)
- CoreDNS with etcd plugin (as your DNS server)

## 🚀 Quick Start

### 1. Set Up Your Configuration

Create a `docker-compose.yml`:

## Docker Agent Mode (monitors local Docker containers)

```yaml
services:
  dnsherpa-docker:
    image: ghcr.io/legandaryra/dnsherpa:latest
    restart: unless-stopped
    environment:
      # === Core Settings ===
      - AGENT_MODE=docker
      - ETCD_ENDPOINTS=192.168.1.10:2379,192.168.1.11:2379
      - DOMAIN=yourdomain.com  # Only needed if hostname not FQDN
      - LOG_LEVEL=info
      - LOG_FORMAT=text
      - ETCD_PREFIX=/skydns
      
      # === Docker Settings (optional) ===
      - DNS_TARGET=traefik.yourdomain.com  # Auto-detected if omitted
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /etc/hostname:/host/hostname:ro  # For auto-detection
```

## Proxmox Agent Mode (monitors Proxmox cluster)

```yaml
services:
  dnsherpa-proxmox:
    image: ghcr.io/legandaryra/dnsherpa:latest
    restart: unless-stopped
    environment:
      # === Core Settings ===
      - AGENT_MODE=proxmox
      - ETCD_ENDPOINTS=192.168.1.10:2379,192.168.1.11:2379
      - DOMAIN=yourdomain.com  # Only needed if VM names not FQDN
      - LOG_LEVEL=info
      - LOG_FORMAT=json  # JSON for production
      - ETCD_PREFIX=/skydns
      
      # === Proxmox Settings ===
      - PROXMOX_API_URL=https://pve.yourdomain.com:8006
      - PROXMOX_TOKEN_ID=dnsherpa@pve
      - PROXMOX_TOKEN_SECRET=your-token-secret
      - PROXMOX_VERIFY_SSL=false
      - PROXMOX_POLL_INTERVAL=30s
      - PROXMOX_INTERFACE=eth0
      - PROXMOX_MULTI_IPV4=first
```

## Hybrid Mode (monitors both Docker and Proxmox)

```yaml
services:
  dnsherpa-hybrid:
    image: ghcr.io/legandaryra/dnsherpa:latest
    restart: unless-stopped
    environment:
      # === Core Settings ===
      - AGENT_MODE=hybrid
      - ETCD_ENDPOINTS=192.168.1.10:2379,192.168.1.11:2379
      - DOMAIN=yourdomain.com  # Only needed if hostnames/VM names not FQDN
      - LOG_LEVEL=info
      - LOG_FORMAT=text
      - ETCD_PREFIX=/skydns
      
      # === Docker Settings (optional) ===
      - DNS_TARGET=traefik.yourdomain.com  # Auto-detected if omitted
      
      # === Proxmox Settings ===
      - PROXMOX_API_URL=https://pve.yourdomain.com:8006
      - PROXMOX_TOKEN_ID=dnsherpa@pve
      - PROXMOX_TOKEN_SECRET=your-token-secret
      - PROXMOX_VERIFY_SSL=false
      - PROXMOX_POLL_INTERVAL=30s
      - PROXMOX_INTERFACE=eth0
      - PROXMOX_MULTI_IPV4=first
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /etc/hostname:/host/hostname:ro  # For Docker auto-detection
```

### 2. Start the Service

```bash
docker-compose up -d
```

### 3. Deploy a Service with Traefik

```yaml
services:
  webapp:
    image: nginx
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.webapp.rule=Host(`webapp.yourdomain.com`)"
```

**That's it! 🎉** 

The DNS record for `webapp.yourdomain.com` is created automatically!

### 4. Configure Proxmox VMs (for Proxmox mode)

DNSherpa automatically creates DNS records for all running VMs/containers based on their names:

**VM Examples:**
- VM name: `web-server` → DNS: `web-server.yourdomain.com`
- VM name: `db.mydomain.com` → DNS: `db.mydomain.com` (already FQDN)

**VM Tag Options:**
```bash
# Skip DNS record creation
dnsherpa-skip

# Use specific IP addresses (highest priority)
dnsherpa-ip:192.168.1.100,2001:db8::1

# Use specific network interface
dnsherpa-interface:ens18
```

**Create API Token in Proxmox:**
1. Go to Datacenter → API Tokens
2. Add token: User `dnsherpa@pve`, Token ID `dnsherpa`
3. Uncheck "Privilege Separation"
4. Use the generated secret in `PROXMOX_TOKEN_SECRET`

## 🔧 Configuration Options

### Core Settings
| Setting | What It Does | Default | Example |
|---------|--------------|---------|---------|
| `AGENT_MODE` | Which services to monitor | `docker` | `docker`, `proxmox`, `hybrid` |
| `ETCD_ENDPOINTS` | Your etcd server addresses | `172.16.0.221:2379,172.16.0.222:2379` | `192.168.1.10:2379,192.168.1.11:2379` |
| `DOMAIN` | Your domain name (only needed when hostname/VM name is not FQDN) | None | `mydomain.com` |
| `LOG_LEVEL` | Logging verbosity level | `info` | `trace`, `debug`, `info`, `warn`, `error`, `fatal` |
| `LOG_FORMAT` | Log output format | `text` | `text`, `json` |
| `ETCD_PREFIX` | DNS storage path in etcd | `/skydns` | `/skydns` |
| `ETCD_TLS` | Enable TLS connection to etcd | `false` | `true` |
| `ETCD_CA_FILE` | Path to CA certificate file | None | `/certs/ca.pem` |
| `ETCD_CERT_FILE` | Path to client certificate file | None | `/certs/client.pem` |
| `ETCD_KEY_FILE` | Path to client private key file | None | `/certs/client-key.pem` |

### Docker Settings
| Setting | Description | Default | Example |
|---------|-------------|---------|---------|
| `DNS_TARGET` | Where domains should point. Can be hostname or IP address (IPv4/IPv6). Optional - auto-detected from hostname if not set. | Auto-detected from hostname | `traefik.mydomain.com`, `192.168.1.100`, `2001:db8::1` |

#### DNS Target Auto-Detection
When `DNS_TARGET` is not specified, it's automatically detected using this priority:
1. **Hostname file**: Read `/host/hostname` (requires mounting `/etc/hostname:/host/hostname:ro`)
2. **FQDN check**: If hostname contains dots, use as-is
3. **Domain append**: If hostname is not FQDN, append `DOMAIN` environment variable

### Proxmox Settings
| Setting | Description | Default | Example |
|---------|-------------|---------|---------|
| `PROXMOX_API_URL` | Proxmox API endpoint (without /api2/json) | None | `https://pve.domain.com:8006` |
| `PROXMOX_TOKEN_ID` | API token ID | None | `dnsherpa@pve` |
| `PROXMOX_TOKEN_SECRET` | API token secret | None | `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` |
| `PROXMOX_VERIFY_SSL` | Verify SSL certificates | `false` | `true` |
| `PROXMOX_POLL_INTERVAL` | How often to check for changes | `30s` | `30s`, `1m`, `2m` |
| `PROXMOX_INTERFACE` | Default network interface | `eth0` | `ens18`, `vmbr0` |
| `PROXMOX_MULTI_IPV4` | Multiple IPv4 strategy | `first` | `first`, `all` |

### DNS Record Settings
| Setting | Description | Value |
|---------|-------------|-------|
| `RecordTTL` | Time-to-live for DNS records | `300` seconds (fixed) |

## 📊 Record Types

**Docker Mode:**
- Uses `DNS_TARGET` setting (Docker Settings) to determine record type
- **Hostname** → CNAME record (`traefik.domain.com`)
- **IPv4 Address** → A record (`192.168.1.100`)  
- **IPv6 Address** → AAAA record (`2001:db8::1`)

**Proxmox Mode:**
- Creates A and AAAA records directly from VM IP addresses
- **IPv4 addresses** → A records (e.g., `/skydns/com/domain/vm-name/a1`)
- **IPv6 addresses** → AAAA records (e.g., `/skydns/com/domain/vm-name/aaaa1`)
- Supports multiple IP addresses per VM with CoreDNS-compatible key suffixes

## 🔍 Troubleshooting

### Container Won't Start?

Check the logs: `docker logs dnsherpa`

Common issues:
- **"Cannot read /host/hostname"**: Add `-v /etc/hostname:/host/hostname:ro` (needed for Docker mode auto-detection)
- **"DOMAIN not set"**: Add `DOMAIN=yourdomain.com` (Core Settings - only needed if hostname/VM name not FQDN)
- **"Cannot connect to etcd"**: Check your `ETCD_ENDPOINTS` (Core Settings)

### DNS Records Not Created?

1. Check container labels: `docker inspect <container>`
2. Verify etcd connection: `docker logs dnsherpa`
3. Test DNS resolution: `nslookup webapp.yourdomain.com`

## 💡 Examples

### Home Lab Setup (Docker Mode)
```yaml
environment:
  - AGENT_MODE=docker
  - ETCD_ENDPOINTS=192.168.1.10:2379
  - DNS_TARGET=192.168.1.100          # Point to your server IP
  - DOMAIN=homelab.local
```

### Production Setup (Docker Mode)
```yaml
environment:
  - AGENT_MODE=docker
  - ETCD_ENDPOINTS=etcd1:2379,etcd2:2379
  - DNS_TARGET=lb.company.com         # Point to load balancer
  - DOMAIN=company.com
```

### IPv6 Setup (Docker Mode)
```yaml
environment:
  - AGENT_MODE=docker
  - ETCD_ENDPOINTS=192.168.1.10:2379
  - DNS_TARGET=2001:db8::100          # Point to IPv6 address
  - DOMAIN=example.com
```

### Proxmox Setup
```yaml
environment:
  - AGENT_MODE=proxmox
  - ETCD_ENDPOINTS=192.168.1.10:2379
  - DOMAIN=homelab.local              # Only needed if VM names not FQDN
  - PROXMOX_API_URL=https://pve.homelab.local:8006
  - PROXMOX_TOKEN_ID=dnsherpa@pve
  - PROXMOX_TOKEN_SECRET=your-token-secret
```

## 🔐 TLS Security Example

For secure etcd connections (these are Core Settings, available in all modes):
```yaml
environment:
  - ETCD_TLS=true
  - ETCD_CA_FILE=/certs/ca.pem
  - ETCD_CERT_FILE=/certs/client.pem  
  - ETCD_KEY_FILE=/certs/client-key.pem
volumes:
  - ./certs:/certs:ro
```

## 🔧 CoreDNS Setup

Your CoreDNS needs this configuration:

```
yourdomain.com:53 {
    etcd {
        path /skydns
        endpoint 192.168.1.10:2379 192.168.1.11:2379
    }
}
```

## 📊 Monitoring

Check what's happening:
```bash
# View logs
docker logs -f dnsherpa

# Example output:
# Creating CNAME record: webapp.mydomain.com -> traefik.mydomain.com  
# Creating A record: api.mydomain.com -> 192.168.1.100
```

## 🛠️ Technical Details

- **Listens to**: Docker Events API
- **Stores DNS in**: etcd key-value store  
- **Compatible with**: CoreDNS etcd plugin
- **Supports**: A, AAAA, and CNAME records
- **Language**: Go 1.25
- **Container**: Multi-architecture (AMD64/ARM64)

## 📄 License

MIT License - Feel free to use and modify!

## 🤝 Contributing

Found a bug or want a feature? 
1. Open an issue
2. Submit a pull request  
3. Help others in discussions

---

**Made with ❤️ for the Docker community**
