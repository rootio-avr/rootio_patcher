# Root.io Patcher

> Automated security patching for Python packages with Root.io

[![Release](https://img.shields.io/github/v/release/rootio-avr/rootio_patcher)](https://github.com/rootio-avr/rootio_patcher/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/rootio-avr/rootio_patcher)](https://golang.org/dl/)
[![License](https://img.shields.io/github/license/rootio-avr/rootio_patcher)](LICENSE)

`rootio_patcher` is a command-line tool that automatically identifies and patches vulnerabilities in your Python dependencies using Root.io's security fixes. It analyzes your installed packages, queries the Root.io API for available patches, and applies them seamlessly.

---

## Features

- 🔍 **Automatic Vulnerability Detection** - Scans your Python environment for known vulnerabilities
- 🔧 **One-Command Patching** - Applies security fixes with a single command
- 🌍 **Cross-Platform** - Works on Linux, macOS, and Windows
- 🔒 **Secure by Default** - Dry-run mode enabled by default to preview changes
- 📦 **Alias Support** - Option to use Root.io aliased packages or direct patches
- 🚀 **Zero Dependencies** - Single binary with no runtime dependencies
- 🔬 **Detailed Reporting** - Clear output showing which vulnerabilities are fixed

---

## Quick Start

```bash
# 1. Set your Root.io API key
export ROOTIO_API_KEY="your-api-key-here"

# 2. Run in dry-run mode (no changes made)
rootio_patcher

# 3. Apply patches for real
DRY_RUN=false rootio_patcher
```

---

## Installation

### Option 1: Download Pre-built Binary (Recommended)

#### Linux (x86_64)
```bash
curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz
chmod +x rootio_patcher
sudo mv rootio_patcher /usr/local/bin/
```

#### macOS (Apple Silicon - M1/M2/M3)
```bash
curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_darwin_arm64.tar.gz | tar xz
chmod +x rootio_patcher
sudo mv rootio_patcher /usr/local/bin/
```

#### macOS (Intel)
```bash
curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_darwin_x86_64.tar.gz | tar xz
chmod +x rootio_patcher
sudo mv rootio_patcher /usr/local/bin/
```

#### Windows (PowerShell)
```powershell
# Download from GitHub releases
Invoke-WebRequest -Uri "https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_windows_x86_64.zip" -OutFile "rootio_patcher.zip"
Expand-Archive -Path rootio_patcher.zip -DestinationPath .
# Add to PATH or run ./rootio_patcher.exe
```

### Option 2: Build from Source

```bash
# Clone the repository
git clone https://github.com/rootio-avr/rootio_patcher.git
cd rootio_patcher

# Build
go build -o rootio_patcher ./cmd/rootio_patcher

# Install
sudo mv rootio_patcher /usr/local/bin/
```

### Verify Installation

```bash
rootio_patcher --help
```

---

## Configuration

`rootio_patcher` is configured entirely through environment variables:

### Required Configuration

| Variable | Description | Example |
|----------|-------------|---------|
| `ROOTIO_API_KEY` | Your Root.io API key (**required**) | `rootio_abc123...` |

### Optional Configuration

| Variable | Description | Default | Valid Values |
|----------|-------------|---------|--------------|
| `DRY_RUN` | Preview changes without applying them | `true` | `true`, `false` |
| `USE_ALIAS` | Use Root.io aliased packages instead of direct patches | `true` | `true`, `false` |
| `ROOTIO_API_URL` | Root.io API endpoint | `https://api.root.io` | Any URL |
| `ROOTIO_PKG_URL` | Root.io package repository URL | `https://pkg.root.io` | Any URL |
| `PYTHON_PATH` | Path to Python interpreter | `python` | `python`, `python3`, `/usr/bin/python3` |
| `LOG_LEVEL` | Logging verbosity | `info` | `debug`, `info`, `warn`, `error` |

### Environment Variable Details

#### `ROOTIO_API_KEY` (Required)

Your Root.io API key for authentication. See [How to Get a Root.io API Key](#how-to-get-a-rootio-api-key) below.

#### `DRY_RUN`

When set to `true`, `rootio_patcher` will analyze your packages and show what **would** be patched without making any changes. This is the default and recommended for first-time use.

Set to `false` to actually apply patches:
```bash
DRY_RUN=false rootio_patcher
```

#### `USE_ALIAS`

Root.io provides two types of patches:

- **Aliased Packages** (`USE_ALIAS=true`, default): Root.io maintains patched versions under a different package name (e.g., `rootio-django` instead of `django`). This allows for better tracking and rollback.

- **Direct Patches** (`USE_ALIAS=false`): Patches are applied directly to the original package name.

Most users should use the default aliased packages.

#### `PYTHON_PATH`

Specifies which Python interpreter to use. This is useful if you have multiple Python versions:

```bash
# Use Python 3.11 specifically
PYTHON_PATH=/usr/bin/python3.11 rootio_patcher

# Use a virtual environment's Python
PYTHON_PATH=./venv/bin/python rootio_patcher
```

#### `LOG_LEVEL`

Controls verbosity of output:

- `error`: Only show errors
- `warn`: Show warnings and errors
- `info` (default): Show general information
- `debug`: Show detailed debugging information

```bash
LOG_LEVEL=debug rootio_patcher
```

---

## How to Get a Root.io API Key

1. **Sign up for Root.io**
   - Visit [https://root.io](https://root.io)
   - Create an account or log in

2. **Navigate to API Settings**
   - Go to your dashboard
   - Click on **Settings** → **API Keys**

3. **Generate a New API Key**
   - Click **"Generate New API Key"**
   - Give it a descriptive name (e.g., "Production Patcher")
   - Copy the generated key (it will only be shown once!)

4. **Store Your API Key Securely**

   **Option A: Environment Variable (Temporary)**
   ```bash
   export ROOTIO_API_KEY="your-api-key-here"
   ```

   **Option B: Shell Profile (Permanent)**
   ```bash
   # Add to ~/.bashrc, ~/.zshrc, or ~/.profile
   echo 'export ROOTIO_API_KEY="your-api-key-here"' >> ~/.bashrc
   source ~/.bashrc
   ```

   **Option C: .env File (Project-specific)**
   ```bash
   # Create .env file in your project directory
   echo 'ROOTIO_API_KEY=your-api-key-here' > .env

   # Load before running (if using a tool like direnv)
   source .env
   rootio_patcher
   ```

   **Option D: CI/CD Secrets**

   For GitHub Actions:
   ```yaml
   - name: Patch vulnerabilities
     env:
       ROOTIO_API_KEY: ${{ secrets.ROOTIO_API_KEY }}
     run: rootio_patcher
   ```

⚠️ **Security Best Practices:**
- Never commit API keys to version control
- Use environment variables or secret management tools
- Rotate keys regularly
- Use different keys for different environments (dev/staging/prod)

---

## Usage Examples

### Basic Usage (Dry Run)

Check what patches are available without making changes:

```bash
export ROOTIO_API_KEY="your-api-key"
rootio_patcher
```

**Output:**
```
Collecting installed packages...
Analyzing packages for vulnerabilities...

DRY-RUN MODE: No changes will be made

The following packages can be patched:

  Package: django 4.2.0 → 4.2.1 (rootio-django)
    Fixes: CVE-2023-12345, CVE-2023-67890

  Package: requests 2.28.0 → 2.28.2 (rootio-requests)
    Fixes: CVE-2023-11111

Run with DRY_RUN=false to apply these patches.
```

### Apply Patches

Actually install the security fixes:

```bash
export ROOTIO_API_KEY="your-api-key"
DRY_RUN=false rootio_patcher
```

**Output:**
```
Applying 2 patches...

[1/2] Patching django (4.2.0 → 4.2.1)...
  ✓ Successfully patched django

[2/2] Patching requests (2.28.0 → 2.28.2)...
  ✓ Successfully patched requests

✓ Successfully patched 2 packages!
```

### Use Direct Patches (No Aliases)

Install patches using original package names:

```bash
export ROOTIO_API_KEY="your-api-key"
DRY_RUN=false USE_ALIAS=false rootio_patcher
```

### Target Specific Python Environment

Patch a specific virtual environment:

```bash
export ROOTIO_API_KEY="your-api-key"
PYTHON_PATH=./venv/bin/python DRY_RUN=false rootio_patcher
```

### Debug Mode

Get detailed information about what's happening:

```bash
export ROOTIO_API_KEY="your-api-key"
LOG_LEVEL=debug rootio_patcher
```

### CI/CD Integration (GitHub Actions)

```yaml
name: Security Patching

on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly on Sunday
  workflow_dispatch:

jobs:
  patch:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.11'

      - name: Install dependencies
        run: pip install -r requirements.txt

      - name: Download rootio_patcher
        run: |
          curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz
          chmod +x rootio_patcher

      - name: Patch vulnerabilities
        env:
          ROOTIO_API_KEY: ${{ secrets.ROOTIO_API_KEY }}
          DRY_RUN: false
        run: ./rootio_patcher
```

### Docker Integration

```dockerfile
FROM python:3.11-slim

# Install your application
COPY requirements.txt .
RUN pip install -r requirements.txt

# Download and run rootio_patcher
RUN curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz && \
    chmod +x rootio_patcher && \
    mv rootio_patcher /usr/local/bin/

# Patch vulnerabilities during build
ARG ROOTIO_API_KEY
ENV ROOTIO_API_KEY=${ROOTIO_API_KEY}
RUN DRY_RUN=false rootio_patcher

# Your application code
COPY . .
CMD ["python", "app.py"]
```

---

## Troubleshooting

### "Failed to load configuration: env: required environment variable 'ROOTIO_API_KEY' is not set"

**Solution:** Set your Root.io API key:
```bash
export ROOTIO_API_KEY="your-api-key"
```

### "API returned status 401: Unauthorized"

**Solution:** Your API key is invalid or expired. Generate a new key from the Root.io dashboard.

### "API returned status 403: Forbidden"

**Solution:** Your API key doesn't have permission to access the remediation API. Contact Root.io support.

### "failed to collect packages: exec: 'python': executable file not found"

**Solution:** Python is not in your PATH. Specify the full path:
```bash
PYTHON_PATH=/usr/bin/python3 rootio_patcher
```

Or install Python if it's missing.

### "No patches needed - all packages are up to date!"

This means your packages are already secure! No action needed. ✓

### Patches fail to install

**Check pip permissions:**
```bash
# If you get permission errors, you might need to use a virtual environment
python3 -m venv venv
source venv/bin/activate
DRY_RUN=false rootio_patcher
```

**Or use pip's --user flag** by setting:
```bash
# This is not currently supported, but you can manually patch with pip
pip install --user <package>
```

---

## How It Works

1. **Discovery**: Scans your Python environment using `pip list` to identify installed packages
2. **Analysis**: Sends package list to Root.io API to check for known vulnerabilities
3. **Reporting**: Displays available patches with CVE information
4. **Patching**: (If `DRY_RUN=false`) Uses `pip install` to apply security fixes
5. **Verification**: Confirms successful installation

---

## Security Considerations

- `rootio_patcher` requires network access to:
  - `api.root.io` - For vulnerability analysis
  - `pkg.root.io` - For downloading patched packages

- API keys are transmitted securely over HTTPS using Basic Auth

- The tool runs `pip install` commands to apply patches. Ensure your Python environment has appropriate permissions.

- By default, `DRY_RUN=true` prevents any changes. Review the dry-run output before applying patches.

---

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Building from Source

```bash
# Clone the repo
git clone https://github.com/rootio-avr/rootio_patcher.git
cd rootio_patcher

# Install dependencies
go mod download

# Build
go build -o rootio_patcher ./cmd/rootio_patcher

# Run tests
go test ./...
```

### Release Process

Releases are automated via GitHub Actions:

```bash
git tag v1.0.0
git push origin v1.0.0
```

GoReleaser will automatically build binaries for all platforms and create a GitHub release.

---

## License

[MIT License](LICENSE)

---

## Support

- **Documentation**: [https://docs.root.io](https://docs.root.io)
- **Issues**: [GitHub Issues](https://github.com/rootio-avr/rootio_patcher/issues)
- **Email**: support@root.io

---

## Related Projects

- [Root.io Platform](https://root.io) - Comprehensive container security and vulnerability management
- [Root.io Python SDK](https://github.com/rootio-avr/python-sdk) - Python library for Root.io API

---

**Made with ❤️ by [Root.io](https://root.io)**
