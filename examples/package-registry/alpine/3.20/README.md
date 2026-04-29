# Alpine 3.20 - Root.io Package Registry Configuration

This example demonstrates how to configure Alpine Linux to use the Root.io package registry.

## Registry Configuration

### In a Dockerfile

**Part 1: Configure Root.io repository**

```dockerfile
ARG ALPINE_VERSION=3.20
FROM alpine:${ALPINE_VERSION}

# Install signing key, configure Root.io repository
RUN --mount=type=secret,id=rootio_api_key \
    # Install Alpine public key for package signature verification
    echo "LS0tLS1CRUdJTiBSU0EgUFVCTElDIEtFWS0tLS0tCk1JSUNDZ0tDQWdFQXNvNzBvakxQRG1jdXVEV3ptcGRnRmZ1Q3Y1TVBsdXBVUUN3Z2QzUEVoSVhiYUFPU1F5a1AKOVFkQVdaeVF1UEduOGlST3dDa2JDMUl4bmxQZU9ldVZIdVhhU0dRYnRUN09McU9yaWJHb0ttRVI1dHRHZlVVMgpybXV0ekphTVczcjY4ZVNqVUJlVVdkYXRtVDFNNnhWWi8wUjRyRitiRkdleExzMTZ0ZHl6YldTWjJEbmJUNTd4ClZsQmFKUThjZ3ZyMmhWN2txZmk2VUllNlZUcnJQMkwvMmlmTlB3L2MyUGdJcnV1cXRjVDdaaUxERmdidnllWUMKWTkxWjlvNnVGb3RSTlF6eVphQTQwQkVEeisyQ3oyUkorOFl3NkJvYUYrTHRkMWVzbTh6UGE4b1c2TTNQaTY5cwpJRlYvR3Z5UzZ1NzcvdXhwN1VwTSsvaVMvZ0pONFFobWJuVTlDamsyeGs1M2ZSamVBQzY2bldodWhSTi9Wd0hRCmUwNVJ1TmliZGFHVzR5eVlmbGtSMVpVZFhsVVhDUkJFazF3UU9Ed0Fka1NuRmwrWlFWS2lVdy9iMDJneXY2YzMKWHp5emZlbXdMcm5zMDZaUEwzbTE2VFlrT3BkQ2NCb0tUQ1NNaTZ6SDdCb211dlpUaTUzSGVkV2NDV2JTcDFIVAp0VFpHMmxmdXJVK1AyQndEZmVJWGxXMWJ0TlMwUDVjSTczOVYzTS9CSW1IYzRCQzRhVWorSWw2dHdqTzZUNlYxCnAwU3BIakJESjBaVVJ2WDZmMDFGWENnMXJqdzZ1T2xrdExrZTNGQUNzUjl5U0VkZkpKTE1IMDlFU1dYbE9RMWoKNnVqZlRWVzdKekFwcDkrV0R2Yzd1U3lzc2dlS3BxY3JuTkd2SlhYbGQxTlcyNWRwSXp0ZXlMc0NBd0VBQVE9PQotLS0tLUVORCBSU0EgUFVCTElDIEtFWS0tLS0tCg==" | \
    base64 -d > /etc/apk/keys/root@alpinelinux.org.rsa.pub && \
    \
    # Add pkg.root.io as repository with authentication from secret
    echo "https://root:$(cat /run/secrets/rootio_api_key)@pkg.root.io/alpine/${ALPINE_VERSION}" >> /etc/apk/repositories && \
    \
    # Update package index
    apk update && \
    \
    # <... install packages here ...>
    \
    # Remove Root.io repository line with credentials from repositories file
    sed -i '/pkg\.root\.io/d' /etc/apk/repositories
```

**Part 2: Install packages with aliasing fallback**

```dockerfile
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
echo "LS0tLS1CRUdJTiBSU0EgUFVCTElDIEtFWS0tLS0tCk1JSUNDZ0tDQWdFQXNvNzBvakxQRG1jdXVEV3ptcGRnRmZ1Q3Y1TVBsdXBVUUN3Z2QzUEVoSVhiYUFPU1F5a1AKOVFkQVdaeVF1UEduOGlST3dDa2JDMUl4bmxQZU9ldVZIdVhhU0dRYnRUN09McU9yaWJHb0ttRVI1dHRHZlVVMgpybXV0ekphTVczcjY4ZVNqVUJlVVdkYXRtVDFNNnhWWi8wUjRyRitiRkdleExzMTZ0ZHl6YldTWjJEbmJUNTd4ClZsQmFKUThjZ3ZyMmhWN2txZmk2VUllNlZUcnJQMkwvMmlmTlB3L2MyUGdJcnV1cXRjVDdaaUxERmdidnllWUMKWTkxWjlvNnVGb3RSTlF6eVphQTQwQkVEeisyQ3oyUkorOFl3NkJvYUYrTHRkMWVzbTh6UGE4b1c2TTNQaTY5cwpJRlYvR3Z5UzZ1NzcvdXhwN1VwTSsvaVMvZ0pONFFobWJuVTlDamsyeGs1M2ZSamVBQzY2bldodWhSTi9Wd0hRCmUwNVJ1TmliZGFHVzR5eVlmbGtSMVpVZFhsVVhDUkJFazF3UU9Ed0Fka1NuRmwrWlFWS2lVdy9iMDJneXY2YzMKWHp5emZlbXdMcm5zMDZaUEwzbTE2VFlrT3BkQ2NCb0tUQ1NNaTZ6SDdCb211dlpUaTUzSGVkV2NDV2JTcDFIVAp0VFpHMmxmdXJVK1AyQndEZmVJWGxXMWJ0TlMwUDVjSTczOVYzTS9CSW1IYzRCQzRhVWorSWw2dHdqTzZUNlYxCnAwU3BIakJESjBaVVJ2WDZmMDFGWENnMXJqdzZ1T2xrdExrZTNGQUNzUjl5U0VkZkpKTE1IMDlFU1dYbE9RMWoKNnVqZlRWVzdKekFwcDkrV0R2Yzd1U3lzc2dlS3BxY3JuTkd2SlhYbGQxTlcyNWRwSXp0ZXlMc0NBd0VBQVE9PQotLS0tLUVORCBSU0EgUFVCTElDIEtFWS0tLS0tCg==" | \
    base64 -d > /etc/apk/keys/root@alpinelinux.org.rsa.pub

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

The Base64-encoded public key is the official Alpine signing key (`root@alpinelinux.org.rsa.pub`) used to verify package signatures from Root.io.

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
