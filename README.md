# Cloudflare Tunnel CLI Tool (cftun)

A command-line tool for quickly creating and managing Cloudflare Tunnels.

## Prerequisites

- Cloudflare account
- `cloudflared` installed
- `jq` installed
- Cloudflare API token with appropriate permissions

## Installation

### Using curl (recommended)
```bash
curl -sSL https://raw.githubusercontent.com/muthuishere/cftun-cli/main/install.sh | bash
```

### Manual Installation
```bash
git clone https://github.com/muthuishere/cftun-cli.git
cd cftun-cli
chmod +x install.sh
./install.sh
```

## Usage
```bash
cftun <full_domain> <local_port>
```

Example:
```bash
cftun mydomain.example.com 3000
```

## Configuration

Set your Cloudflare API token as an environment variable:
```bash
export CLOUDFLARE_API_KEY='your-api-token'
```

## License

MIT
