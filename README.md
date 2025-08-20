# DNSherpa

**Automatically manage DNS records for your Docker services!**

When you deploy a Docker service with Traefik labels, this tool automatically creates DNS records so your domains work instantly. No more manual DNS management!

## ✨ What It Does

🔄 **Monitors your Docker containers** and watches for Traefik labels  
🏷️ **Finds domain names** in your `Host()` rules  
📝 **Creates DNS records** automatically in your DNS server  

## 🎯 Perfect For

- **Home Labs**: Automatic DNS for your self-hosted services
- **Development**: No more editing `/etc/hosts` files  
- **Small Teams**: Set-and-forget DNS management
- **Docker Compose**: Works with all your existing services

## 📋 What You Need

- Docker services with Traefik labels
- An etcd server (for DNS storage)
- CoreDNS (as your DNS server)

## 🚀 Quick Start

### 1. Set Up Your Configuration

Create a `docker-compose.yml`:

```yaml
services:
  dnsherpa:
    image: ghcr.io/legandaryra/dnsherpa:latest
    restart: unless-stopped
    environment:
      # === Core Settings ===
      # Where is your etcd server?
      - ETCD_ENDPOINTS=192.168.1.10:2379,192.168.1.11:2379
      
      # Where should domains point? (Pick one - or omit to auto-detect)
      - DNS_TARGET=traefik.yourdomain.com     # Use hostname (CNAME)
      # - DNS_TARGET=192.168.1.100             # Use IPv4 (A record)  
      # - DNS_TARGET=2001:db8::1               # Use IPv6 (AAAA record)
      
      # What's your domain? (only needed if hostname is not FQDN)
      - DOMAIN=yourdomain.com
      
      # === etcd Settings ===
      # DNS storage path in etcd (default: /skydns)
      - ETCD_PREFIX=/skydns
      
      # === TLS Configuration (optional) ===
      # Enable TLS connection to etcd
      # - ETCD_TLS=true
      # - ETCD_CA_FILE=/certs/ca.pem
      # - ETCD_CERT_FILE=/certs/client.pem
      # - ETCD_KEY_FILE=/certs/client-key.pem
    volumes:
      # Required volumes
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /etc/hostname:/host/hostname:ro
      
      # Optional: TLS certificates (if using ETCD_TLS=true)
      # - ./certs:/certs:ro
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

## 🔧 Configuration Options

### Core Settings
| Setting | What It Does | Default | Example |
|---------|--------------|---------|---------|
| `ETCD_ENDPOINTS` | Your etcd server addresses | `172.16.0.221:2379,172.16.0.222:2379` | `192.168.1.10:2379,192.168.1.11:2379` |
| `DNS_TARGET` | Where domains should point | Auto-detected from hostname | `traefik.mydomain.com` |
| `DOMAIN` | Your domain name (used if hostname not FQDN) | None | `mydomain.com` |

### etcd Settings
| Setting | Description | Default | Example |
|---------|-------------|---------|---------|
| `ETCD_PREFIX` | DNS storage path in etcd | `/skydns` | `/skydns` |

### TLS Configuration
| Setting | Description | Default | Example |
|---------|-------------|---------|---------|
| `ETCD_TLS` | Enable TLS connection to etcd | `false` | `true` |
| `ETCD_CA_FILE` | Path to CA certificate file | None | `/certs/ca.pem` |
| `ETCD_CERT_FILE` | Path to client certificate file | None | `/certs/client.pem` |
| `ETCD_KEY_FILE` | Path to client private key file | None | `/certs/client-key.pem` |

### DNS Target Detection
The `DNS_TARGET` is automatically detected using this priority:
1. **Environment variable**: If `DNS_TARGET` is set, use it directly
2. **Hostname file**: Read `/host/hostname` (requires mounting `/etc/hostname:/host/hostname:ro`)
3. **FQDN check**: If hostname contains dots, use as-is
4. **Domain append**: If hostname is not FQDN, append `DOMAIN` environment variable

### DNS Record Settings
| Setting | Description | Value |
|---------|-------------|-------|
| `RecordTTL` | Time-to-live for DNS records | `300` seconds (fixed) |

## 📊 Record Types

The tool automatically picks the right DNS record type:

- **Hostname** → CNAME record (`traefik.domain.com`)
- **IPv4 Address** → A record (`192.168.1.100`)  
- **IPv6 Address** → AAAA record (`2001:db8::1`)

## 🔍 Troubleshooting

### Container Won't Start?

Check the logs: `docker logs traefik-dns-automator`

Common issues:
- **"Cannot read /host/hostname"**: Add `-v /etc/hostname:/host/hostname:ro`
- **"DOMAIN not set"**: Add `DOMAIN=yourdomain.com` 
- **"Cannot connect to etcd"**: Check your `ETCD_ENDPOINTS`

### DNS Records Not Created?

1. Check container labels: `docker inspect <container>`
2. Verify etcd connection: `docker logs traefik-dns-automator`
3. Test DNS resolution: `nslookup webapp.yourdomain.com`

## 💡 Examples

### Home Lab Setup
```yaml
environment:
  - DNS_TARGET=192.168.1.100          # Point to your server IP
  - DOMAIN=homelab.local
```

### Production Setup  
```yaml
environment:
  - DNS_TARGET=lb.company.com         # Point to load balancer
  - DOMAIN=company.com
```

### IPv6 Setup
```yaml
environment:
  - DNS_TARGET=2001:db8::100          # Point to IPv6 address
  - DOMAIN=example.com
```

## 🔐 TLS Security Example

For secure etcd connections:
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
docker logs -f traefik-dns-automator

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
