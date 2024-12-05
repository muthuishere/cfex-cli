# Cloudflare Tunnel CLI Tool (cftun)

A command-line tool for quickly creating and managing Cloudflare Tunnels. Create instant tunnels to expose your local services with HTTPS using Cloudflare.

## Platform Support

This tool works on:
- Linux
- macOS
- Windows (only with WSL - Windows Subsystem for Linux)

### Windows Users Important Note
If you're on Windows, you must use WSL (Windows Subsystem for Linux) to run this tool.

To set up WSL:
1. Open PowerShell as Administrator and run:
   ```powershell
   wsl --install
   ```
2. Restart your computer
3. Follow the Linux installation instructions within your WSL environment


## Prerequisites

- Cloudflare account with a registered domain
- `cloudflared` installed
- `jq` installed
- Cloudflare API token with Zone permissions

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

## Setup Cloudflare API Token

1. Log in to your [Cloudflare Dashboard](https://dash.cloudflare.com)
2. Click on your profile icon and select "My Profile"
3. Navigate to "API Tokens" in the left sidebar
4. Click "Create Token"
5. Select "Create Custom Token"
6. Set the following permissions:
   - Zone - DNS - Edit
   - Zone - Zone - Read
7. Under "Zone Resources", select:
   - Include - All Zones (or specific zones you want to manage)
8. Give your token a name (e.g., "CFTUN CLI Token")
9. Click "Continue to summary" and then "Create Token"
10. Copy the generated token

## Configuration

Set your Cloudflare API token as an environment variable:

```bash
export CLOUDFLARE_API_KEY='your-api-token'
```

For permanent configuration, add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):

```bash
echo 'export CLOUDFLARE_API_KEY="your-api-token"' >> ~/.bashrc
source ~/.bashrc
```

## Usage

### Basic Command
```bash
cftun <full_domain> <local_port>
```

### Example
```bash
# Expose local port 3000 at mydomain.example.com
cftun mydomain.example.com 3000
```

### Common Use Cases

1. Expose a React development server:
```bash
cftun dev.yourdomain.com 3000
```

2. Share a local API:
```bash
cftun api.yourdomain.com 8080
```

3. Test webhooks locally:
```bash
cftun webhook.yourdomain.com 4000
```

The tool will:
1. Create a new tunnel
2. Configure DNS records
3. Start the tunnel
4. Clean up resources when stopped (Ctrl+C)

## Troubleshooting

### Common Issues

1. **"Error: cloudflared is not installed"**
   - Install cloudflared following [official instructions](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation)

2. **"Authentication failed"**
   - Verify your CLOUDFLARE_API_KEY is set correctly
   - Ensure the API token has correct permissions
   - Run `cloudflared tunnel login` if needed

3. **"DNS record already exists"**
   - The tool will automatically clean up existing records
   - If persists, manually delete the DNS record from Cloudflare dashboard

4. **"Port already in use"**
   - Ensure the local port is not being used by another application
   - Try a different port number

## Security Considerations

- Keep your API token secure and never commit it to version control
- Use separate API tokens for different environments/purposes
- Regularly rotate your API tokens
- Always use the minimum required permissions for API tokens

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT

## Support

- Create an [issue](https://github.com/muthuishere/cftun-cli/issues) for bug reports
- Star the repository if you find it useful
- Follow [@muthuishere](https://twitter.com/muthuishere) for updates
