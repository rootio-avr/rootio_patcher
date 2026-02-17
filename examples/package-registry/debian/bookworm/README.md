# Debian Bookworm - Root.io Package Registry Configuration

This example demonstrates how to configure Debian to use the Root.io APT package registry.

## Registry Configuration

### In a Dockerfile

```dockerfile
FROM debian:bookworm-slim

# Install required dependencies
RUN apt-get update && apt-get install -y \
    gnupg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Add Root.io GPG key and repository
ARG ROOTIO_API_KEY
RUN echo "LS0tLS1CRUdJTiBQR1AgUFVCTElDIEtFWSBCTE9DSy0tLS0tCgptRE1FYVlIQ1dSWUpLd1lCQkFIYVJ3OEJBUWRBcDdXVHNLMTVrWTNmQ0pxOUNRVnlxODluRzFoNEw4OHZvVndqCnB0NGNXSjYwSkZKdmIzUXVhVzhnUVZCVUlGSmxjRzl6YVhSdmNua2dQR0Z3ZEVCeWIyOTBMbWx2UG9pVEJCTVcKQ2dBN0ZpRUUzSVVhWTlLRDFsTUhKYTdNZ09RM004RHd3c2tGQW1tQndsa0NHd01GQ3drSUJ3SUNJZ0lHRlFvSgpDQXNDQkJZQ0F3RUNIZ2NDRjRBQUNna1FnT1EzTThEd3dzbGY2d0QrSWxqSGRkVmFKM2xKYjBsSE0rZVFubWNvCnlmTTlpWis5cXI0SjBNYnZsNG9CQUtOL0pYZkJvR2JGYzgzM0ZmN1I5R3M5UXU2bm1EUVZlSDI4eHEwdDRwWU4KPWs3ZHMKLS0tLS1FTkQgUEdQIFBVQkxJQyBLRVkgQkxPQ0stLS0tLQo=" | \
    base64 -d | gpg --dearmor -o /etc/apt/keyrings/rootio.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/rootio.gpg] https://root:${ROOTIO_API_KEY}@pkg.root.io/debian/bookworm bookworm main" > /etc/apt/sources.list.d/rootio.list

# Update package index
RUN apt-get update

# Install packages with fallback to unaliased packages
# This loop checks for rootio-prefixed packages first, then falls back to standard packages
RUN for pkg in curl git vim bash libgit2-dev tini; do \
    if apt-cache show "rootio-$pkg" >/dev/null 2>&1; then \
      apt-get install -y --no-install-recommends "rootio-$pkg"; \
    else \
      apt-get install -y --no-install-recommends "$pkg"; \
    fi; \
    done && \
    rm -rf /var/lib/apt/lists/*
```

**Build with:**
```bash
DOCKER_BUILDKIT=1 docker build --secret id=rootio_api_key,env=ROOTIO_API_KEY -t your-image .
```

### On a Running System

```bash
# Install prerequisites
sudo apt-get update
sudo apt-get install -y gnupg ca-certificates

# Create keyrings directory if it doesn't exist
sudo mkdir -p /etc/apt/keyrings

# Install Root.io GPG key
echo "LS0tLS1CRUdJTiBQR1AgUFVCTElDIEtFWSBCTE9DSy0tLS0tCgptRE1FYVlIQ1dSWUpLd1lCQkFIYVJ3OEJBUWRBcDdXVHNLMTVrWTNmQ0pxOUNRVnlxODluRzFoNEw4OHZvVndqCnB0NGNXSjYwSkZKdmIzUXVhVzhnUVZCVUlGSmxjRzl6YVhSdmNua2dQR0Z3ZEVCeWIyOTBMbWx2UG9pVEJCTVcKQ2dBN0ZpRUUzSVVhWTlLRDFsTUhKYTdNZ09RM004RHd3c2tGQW1tQndsa0NHd01GQ3drSUJ3SUNJZ0lHRlFvSgpDQXNDQkJZQ0F3RUNIZ2NDRjRBQUNna1FnT1EzTThEd3dzbGY2d0QrSWxqSGRkVmFKM2xKYjBsSE0rZVFubWNvCnlmTTlpWis5cXI0SjBNYnZsNG9CQUtOL0pYZkJvR2JGYzgzM0ZmN1I5R3M5UXU2bm1EUVZlSDI4eHEwdDRwWU4KPWs3ZHMKLS0tLS1FTkQgUEdQIFBVQkxJQyBLRVkgQkxPQ0stLS0tLQo=" | \
    base64 -d | sudo gpg --dearmor -o /etc/apt/keyrings/rootio.gpg

# Add Root.io repository (replace YOUR_API_KEY with your actual key)
echo "deb [signed-by=/etc/apt/keyrings/rootio.gpg] https://root:YOUR_API_KEY@pkg.root.io/debian/bookworm bookworm main" | \
    sudo tee /etc/apt/sources.list.d/rootio.list

# Update package index
sudo apt-get update

# Install packages with fallback to unaliased packages
for pkg in curl git vim bash libgit2-dev tini; do
  if apt-cache show "rootio-$pkg" >/dev/null 2>&1; then
    sudo apt-get install -y --no-install-recommends "rootio-$pkg"
  else
    sudo apt-get install -y --no-install-recommends "$pkg"
  fi
done
```

## Configuration Details

### Repository URL Format

```
https://root:${ROOTIO_API_KEY}@pkg.root.io/debian/${RELEASE} ${RELEASE} main
```

- **Authentication**: HTTP Basic Auth with username `root` and your API key as password
- **Path**: `/debian/${RELEASE}` (e.g., `/debian/bookworm`)
- **Release**: Debian codename (e.g., `bullseye`, `bookworm`, `trixie`)
- **Component**: `main`

### GPG Key

The Base64-encoded public key is the Root.io APT signing key used to verify package signatures. It's stored in `/etc/apt/keyrings/rootio.gpg` following Debian best practices.

### Repository File Location

Configuration is stored in `/etc/apt/sources.list.d/rootio.list`, keeping it separate from the main sources list for easier management.

### Package Aliasing

Root.io uses package aliasing with the `rootio-` prefix for patched packages:
- Patched packages are named `rootio-<package>` (e.g., `rootio-curl`)
- Original packages remain available without the prefix (e.g., `curl`)
- Use a for loop to check for aliased packages and fall back to standard packages
- This pattern works in both Dockerfiles and on running systems

## Supported Debian Releases

- `bullseye` - Debian 11
- `bookworm` - Debian 12
- `trixie` - Debian 13 (testing)

Simply replace the release codename in the repository configuration.

## API Key

Get your API key from [Root.io](https://app.root.io).

**For local development:**
- Export as environment variable: `export ROOTIO_API_KEY=your_key`
- Store in `.env` file (if using direnv or docker-compose)

**For production:**
- Use Docker secrets, Kubernetes secrets, or your cloud provider's secrets manager
- Never commit API keys to version control
