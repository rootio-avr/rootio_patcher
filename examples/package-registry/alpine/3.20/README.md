# Alpine 3.20 - Root.io Package Registry Configuration

This example demonstrates how to configure Alpine Linux to use the Root.io package registry.

## Registry Configuration

### In a Dockerfile

```dockerfile
ARG ALPINE_VERSION=3.20
FROM alpine:${ALPINE_VERSION}

# Install the Alpine public key for package signature verification
RUN echo "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUlJQklqQU5CZ2txaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUFvcG1yUjR1em95WEFiTHFiRUE3cwpBNWQrVytnUzE0TzEwMlJHUjhxa0h0cDhSUEs2b3FvZkhJZzNYSWpnSVlzalQwYTk1VXNzbGFlL2FDWU1UeFBVCmhnSmxkTVU1SnlzZmYxT1BEQld4R2ZCOFJCL081cjA1cU9JLzJ3QlgwQjl5NjhhU0xQZi9UWEE3akY5STRKL3EKdjArWVF2c3owRWdqUEFjeVN2MlgvOHE2R2RoOHRkWG4rbTVBYjJoUU5YUE1TdTM3aXMwUVR3Uk9DSFV4ZXZLeAo0OUZ5UGkvQUR5UDB6bVFYaUtCQW52alN6YVNQNmZPQm1yYlNNY05wYnVPV2lwUUJoajRnVE5KOVNzaDZSQmdmCmJVUlFhdmlwWlAyL0RiNTNiLzJ5UTJJU21QMEo1cmdjWnkrdXoyZndTWmtHb3pUa0ErZDE1dHludlJRVFQwbGkKWXdJREFRQUIKLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg==" | \
    base64 -d > /etc/apk/keys/root@alpinelinux.org-67b85bd5.rsa.pub

# Add pkg.root.io as SECONDARY repository (append, don't replace)
ARG ROOTIO_API_KEY
RUN echo "https://root:${ROOTIO_API_KEY}@pkg.root.io/alpine/${ALPINE_VERSION}" >> /etc/apk/repositories

# Update package index
RUN apk update

# Install packages with fallback to unaliased packages
# This loop checks for rootio-prefixed packages first, then falls back to standard packages
RUN for pkg in curl git vim bash libgit2-dev tini; do \
    if apk search -e "rootio-$pkg" | grep -q "rootio-$pkg"; then \
      apk add --no-cache "rootio-$pkg"; \
    else \
      apk add --no-cache "$pkg"; \
    fi; \
    done
```

**Build with:**
```bash
DOCKER_BUILDKIT=1 docker build --secret id=rootio_api_key,env=ROOTIO_API_KEY -t your-image .
```

### On a Running System

```bash
# Install the signing key
echo "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUlJQklqQU5CZ2txaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUFvcG1yUjR1em95WEFiTHFiRUE3cwpBNWQrVytnUzE0TzEwMlJHUjhxa0h0cDhSUEs2b3FvZkhJZzNYSWpnSVlzalQwYTk1VXNzbGFlL2FDWU1UeFBVCmhnSmxkTVU1SnlzZmYxT1BEQld4R2ZCOFJCL081cjA1cU9JLzJ3QlgwQjl5NjhhU0xQZi9UWEE3akY5STRKL3EKdjArWVF2c3owRWdqUEFjeVN2MlgvOHE2R2RoOHRkWG4rbTVBYjJoUU5YUE1TdTM3aXMwUVR3Uk9DSFV4ZXZLeAo0OUZ5UGkvQUR5UDB6bVFYaUtCQW52alN6YVNQNmZPQm1yYlNNY05wYnVPV2lwUUJoajRnVE5KOVNzaDZSQmdmCmJVUlFhdmlwWlAyL0RiNTNiLzJ5UTJJU21QMEo1cmdjWnkrdXoyZndTWmtHb3pUa0ErZDE1dHludlJRVFQwbGkKWXdJREFRQUIKLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg==" | \
    base64 -d > /etc/apk/keys/root@alpinelinux.org-67b85bd5.rsa.pub

# Add Root.io repository (replace YOUR_API_KEY with your actual key)
echo "https://root:YOUR_API_KEY@pkg.root.io/alpine/3.20" >> /etc/apk/repositories

# Update package index
apk update

# Install packages with fallback to unaliased packages
for pkg in curl git vim bash libgit2-dev tini; do
  if apk search -e "rootio-$pkg" | grep -q "rootio-$pkg"; then
    apk add --no-cache "rootio-$pkg"
  else
    apk add --no-cache "$pkg"
  fi
done
```

## Configuration Details

### Repository URL Format

```
https://root:${ROOTIO_API_KEY}@pkg.root.io/alpine/${ALPINE_VERSION}
```

- **Authentication**: HTTP Basic Auth with username `root` and your API key as password
- **Path structure**: `/alpine/{version}` (e.g., `/alpine/3.20`, `/alpine/3.21`)

### Signing Key

The Base64-encoded public key is the official Alpine signing key (`root@alpinelinux.org-67b85bd5.rsa.pub`) used to verify package signatures from Root.io.

### Repository Priority

By **appending** the Root.io repository to `/etc/apk/repositories` (using `>>`), it acts as a secondary source:
- Alpine's official repositories are checked first
- Root.io packages are used when patches are available
- This prevents conflicts with official packages

### Package Aliasing

Root.io uses package aliasing with the `rootio-` prefix for patched packages:
- Patched packages are named `rootio-<package>` (e.g., `rootio-curl`)
- Original packages remain available without the prefix (e.g., `curl`)
- Use a for loop to check for aliased packages and fall back to standard packages
- This pattern works in both Dockerfiles and on running systems

## Supported Versions

- Alpine 3.18
- Alpine 3.19
- Alpine 3.20
- Alpine 3.21
- Alpine 3.22

Simply replace `${ALPINE_VERSION}` in the repository URL with your desired version.

## API Key

Get your API key from [Root.io](https://app.root.io).

**For local development:**
- Export as environment variable: `export ROOTIO_API_KEY=your_key`
- Store in `.env` file (if using direnv or docker-compose)

**For production:**
- Use Docker secrets, Kubernetes secrets, or your cloud provider's secrets manager
- Never commit API keys to version control
