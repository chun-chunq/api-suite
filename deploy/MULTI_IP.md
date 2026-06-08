# Multiple Hetzner Floating IPs — Setup Guide

You can add multiple Hetzner Floating IPs for two purposes:
1. **Different domain/IP per API** (e.g. `sanctions-api.yourdomain.com` on one IP, rest on another)
2. **IP rotation for scrapers** — each Chrome scraper uses a different outbound IP to avoid blocks

---

## Part 1: Basic — Two IPs, Two Domains

This is the simplest setup: point two floating IPs to the same server,
host different APIs on each.

### Hetzner side
1. Hetzner Console → Floating IPs → Create → assign to your server
2. Note the new IP (e.g. `5.6.7.8`)

### Server side — bind the new IP

```bash
# Check your network interface name
ip addr show
# Usually eth0 or ens3

# Add floating IP to interface (replace eth0 and IP):
ip addr add 5.6.7.8/32 dev eth0
```

Make it persistent (Debian/Ubuntu):
```bash
# /etc/network/interfaces.d/floating.conf
auto eth0:1
iface eth0:1 inet static
    address 5.6.7.8
    netmask 255.255.255.255
```

### Nginx — two server blocks
```nginx
# Primary IP — all APIs
server {
    listen 1.2.3.4:443 ssl http2;        # your main floating IP
    server_name api.yourdomain.com;
    # ... existing config ...
}

# Secondary IP — high-value compliance APIs only
server {
    listen 5.6.7.8:443 ssl http2;        # second floating IP
    server_name compliance.yourdomain.com;
    # Same SSL certs (or get a new cert for this subdomain)

    location /v1/sanctions/  { proxy_pass http://sanctions; }
    location /v1/lei/        { proxy_pass http://gleif; }
    location /v1/bafin/      { proxy_pass http://bafin; }
}
```

---

## Part 2: IP Rotation for Scrapers (Advanced)

Use this to make each Chrome scraper API use a different outbound IP.
This reduces the chance of getting blocked by BaFin/DPMA/insolvency portals.

### How it works
Linux lets you mark packets from specific processes and route them through a specific source IP.
We use iptables + policy routing.

### Setup script

```bash
#!/bin/bash
# /opt/setup-ip-routing.sh
# Run once after each reboot (or add to /etc/rc.local)

# Your floating IPs (fill in actual values)
IP1="1.2.3.4"   # main IP — insolvency-api, zvg-api
IP2="5.6.7.8"   # second IP — dpma-api
IP3="9.10.11.12" # third IP — bafin-api (if you have a third)

IFACE="eth0"

# Add IPs to interface
ip addr add ${IP2}/32 dev ${IFACE} 2>/dev/null || true
ip addr add ${IP3}/32 dev ${IFACE} 2>/dev/null || true

# Create routing tables for each IP
echo "201 ip2rt" >> /etc/iproute2/rt_tables 2>/dev/null || true
echo "202 ip3rt" >> /etc/iproute2/rt_tables 2>/dev/null || true

# Add routes
ip route add default via $(ip route | grep default | awk '{print $3}') src ${IP2} table ip2rt
ip route add default via $(ip route | grep default | awk '{print $3}') src ${IP3} table ip3rt

# Mark packets from docker containers by port
# dpma-api runs on port 8083 → route its outbound through IP2
iptables -t mangle -A OUTPUT -p tcp --sport 8083 -j MARK --set-mark 2
iptables -t mangle -A OUTPUT -p tcp --sport 8087 -j MARK --set-mark 3  # bafin-api

# Apply routing rules
ip rule add fwmark 2 table ip2rt
ip rule add fwmark 3 table ip3rt
```

```bash
chmod +x /opt/setup-ip-routing.sh
/opt/setup-ip-routing.sh

# Make persistent across reboots:
echo "@reboot root /opt/setup-ip-routing.sh" >> /etc/cron.d/ip-routing
```

### Update docker-compose for IP rotation

When the scraper makes outbound requests, Linux will route them through the assigned IP.
No code changes needed — the routing happens at the kernel level.

Optionally you can also set the container's source IP explicitly:
```yaml
# In docker-compose.yml, for dpma-api:
  dpma-api:
    sysctls:
      - net.ipv4.ip_nonlocal_bind=1
    # ... rest of config
```

---

## Part 3: Per-API Subdomains (Optional)

With multiple IPs, you can sell each API with its own dedicated subdomain and IP.
Enterprise customers prefer this (dedicated IP = no noisy neighbor risk).

```
api.yourdomain.com          → main floating IP (all APIs)
sanctions.yourdomain.com    → dedicated IP (compliance team SLA)
lei.yourdomain.com          → dedicated IP (fintech MiFID II)
```

DNS setup (in your domain registrar / Hetzner DNS):
```
api             A    1.2.3.4
sanctions       A    5.6.7.8
lei             A    5.6.7.8   (same IP, different subdomain is fine)
```

Then add SSL certs:
```bash
certbot certonly --nginx -d api.yourdomain.com -d sanctions.yourdomain.com -d lei.yourdomain.com
```

---

## Summary

| Goal | Effort | Benefit |
|------|--------|---------|
| Second domain/IP for compliance APIs | Low (30 min) | Better branding, separate SLA |
| IP rotation for scrapers | Medium (1h) | Reduced block risk from target sites |
| Per-API dedicated IPs for Enterprise | Medium | Premium offering, higher price |

**Recommendation:** Start with 1 extra floating IP (€3/mo).
Route `bafin-api` and `dpma-api` through it — they're most likely to get rate-limited by target sites.
