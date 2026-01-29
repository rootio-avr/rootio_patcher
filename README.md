# Root.io Patcher

> Automated security patching for Python, npm, and Maven packages with Root.io

[![Release](https://img.shields.io/github/v/release/rootio-avr/rootio_patcher)](https://github.com/rootio-avr/rootio_patcher/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/rootio-avr/rootio_patcher)](https://golang.org/dl/)
[![License](https://img.shields.io/github/license/rootio-avr/rootio_patcher)](LICENSE)

`rootio_patcher` is a command-line tool that automatically identifies and patches vulnerabilities in your dependencies using Root.io's security fixes. It supports Python (pip), JavaScript (npm/yarn/pnpm), and Java (Maven) ecosystems, providing comprehensive security remediation across your entire stack.

---

## Features

- 🔍 **Multi-Ecosystem Support** - Python (pip), JavaScript (npm/yarn/pnpm), and Java (Maven)
- 🔧 **One-Command Patching** - Applies security fixes with a single command
- 🌍 **Cross-Platform** - Works on Linux, macOS, and Windows
- 🔒 **Secure by Default** - Dry-run mode enabled by default to preview changes
- 📦 **Smart Patching Strategies** - Post-install for pip, pre-install for npm/Maven
- 🚀 **Zero Dependencies** - Single binary with no runtime dependencies
- 🔬 **Detailed Reporting** - Clear output showing which vulnerabilities are fixed

---

## Quick Start

```bash
# 1. Set your Root.io API key
export ROOTIO_API_KEY="your-api-key-here"

# 2. Choose your package manager and run in dry-run mode (preview only)

# For Python packages
rootio_patcher pip remediate

# For npm/yarn/pnpm packages
rootio_patcher npm remediate --package-manager=npm

# For Maven packages
rootio_patcher maven remediate

# 3. Apply patches for real
rootio_patcher pip remediate --dry-run=false
rootio_patcher npm remediate --dry-run=false
rootio_patcher maven remediate --dry-run=false
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

`rootio_patcher` uses a combination of environment variables and command-line flags:

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `ROOTIO_API_KEY` | Your Root.io API key | - | **Yes** |
| `ROOTIO_API_URL` | Root.io API endpoint | `https://api.root.io` | No |
| `ROOTIO_PKG_URL` | Root.io package repository URL | `https://pkg.root.io` | No |
| `LOG_LEVEL` | Logging verbosity (`debug`, `info`, `warn`, `error`) | `info` | No |

### CLI Commands and Flags

`rootio_patcher` uses subcommands for each package manager:

#### Python/pip

```bash
rootio_patcher pip remediate [FLAGS]
```

**Flags:**
- `--python-path` - Path to Python interpreter (default: `python`)
- `--dry-run` - Preview changes without applying (default: `true`)
- `--use-alias` - Use Root.io aliased packages (default: `true`)

**How it works:** Post-install patching - scans installed packages and reinstalls them with Root.io patches

#### npm/yarn/pnpm

```bash
rootio_patcher npm remediate [FLAGS]
```

**Flags:**
- `--package-manager` - Package manager to use: `npm`, `yarn`, or `pnpm` (default: `npm`)
- `--dry-run` - Preview changes without applying (default: `true`)

**How it works:** Pre-install patching - updates `package.json` with overrides/resolutions. After running, execute `npm install` to apply patches.

#### Maven

```bash
rootio_patcher maven remediate [FLAGS]
```

**Flags:**
- `--file` - Path to pom.xml (default: `pom.xml`)
- `--dry-run` - Preview changes without applying (default: `true`)

**How it works:** Pre-install patching - updates version numbers in `pom.xml`. After running, execute `mvn clean install` to apply patches.

### Configuration Details

#### `ROOTIO_API_KEY` (Required)

Your Root.io API key for authentication. See [How to Get a Root.io API Key](#how-to-get-a-rootio-api-key) below.

```bash
export ROOTIO_API_KEY="sk_your-api-key-here"
```

#### `--dry-run` Flag

When set to `true` (default), `rootio_patcher` will analyze your packages and show what **would** be changed without making any modifications. This is recommended for first-time use.

Set to `false` to actually apply patches:
```bash
rootio_patcher pip remediate --dry-run=false
```

#### `--use-alias` Flag (pip only)

Root.io provides two types of patches for Python packages:

- **Aliased Packages** (`--use-alias=true`, default): Root.io maintains patched versions under a different package name (e.g., `rootio-django` instead of `django`). This allows for better tracking and rollback.

- **Direct Patches** (`--use-alias=false`): Patches are applied directly to the original package name.

Most users should use the default aliased packages.

#### `--python-path` Flag (pip only)

Specifies which Python interpreter to use. This is useful if you have multiple Python versions:

```bash
# Use Python 3.11 specifically
rootio_patcher pip remediate --python-path=/usr/bin/python3.11

# Use a virtual environment's Python
rootio_patcher pip remediate --python-path=./venv/bin/python
```

#### `LOG_LEVEL` Environment Variable

Controls verbosity of output:

- `error`: Only show errors
- `warn`: Show warnings and errors
- `info` (default): Show general information
- `debug`: Show detailed debugging information

```bash
LOG_LEVEL=debug rootio_patcher pip remediate
```

---

## How to Get a Root.io API Key

1. **Sign up for Root.io**
   - Visit [https://app.root.io](https://app.root.io)
   - Create an account or log in

2. **Navigate to API Settings**
   - Go to your dashboard
   - Click on **Settings** → **Token Management**

3. **Generate a New API Key**
   - Click **"Generate API Token"**
   - It's in your clipboard!

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

### Python (pip) Examples

#### Basic Usage (Dry Run)

Check what patches are available without making changes:

```bash
export ROOTIO_API_KEY="your-api-key"
rootio_patcher pip remediate
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

Run with --dry-run=false to apply these patches.
```

#### Apply Patches

Actually install the security fixes:

```bash
export ROOTIO_API_KEY="your-api-key"
rootio_patcher pip remediate --dry-run=false
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

#### Use Direct Patches (No Aliases)

Install patches using original package names:

```bash
rootio_patcher pip remediate --dry-run=false --use-alias=false
```

#### Target Specific Python Environment

Patch a specific virtual environment:

```bash
rootio_patcher pip remediate --python-path=./venv/bin/python --dry-run=false
```

### npm/yarn/pnpm Examples

#### npm with Dry Run

Preview npm package patches:

```bash
export ROOTIO_API_KEY="your-api-key"
rootio_patcher npm remediate --package-manager=npm
```

**Output:**
```
=== DRY-RUN MODE ===
The following overrides would be added to package.json:

1. Package: express
   Current version: 4.17.1
   Aliased package: npm:@rootio/express@4.17.3
   CVEs Fixed: [CVE-2024-12345]

2. Package: lodash
   Current version: 4.17.20
   Aliased package: npm:@rootio/lodash@4.17.21
   CVEs Fixed: [CVE-2024-67890]

These will be added to package.json under "overrides" field

To apply these patches, run with --dry-run=false
Then run: npm install
```

#### Apply npm Patches

```bash
rootio_patcher npm remediate --package-manager=npm --dry-run=false
```

**Output:**
```
Applying 2 patches to package.json...

  - express: 4.17.1 → @rootio/express@4.17.3
  - lodash: 4.17.20 → @rootio/lodash@4.17.21

✓ Successfully updated package.json with 2 overrides!

Next steps:
  1. Review the changes in package.json
  2. Run: npm install
  3. Test your application
```

#### yarn Support

```bash
rootio_patcher npm remediate --package-manager=yarn --dry-run=false
# Then run: yarn install
```

#### pnpm Support

```bash
rootio_patcher npm remediate --package-manager=pnpm --dry-run=false
# Then run: pnpm install
```

### Maven Examples

#### Basic Usage (Dry Run)

Preview Maven patches:

```bash
export ROOTIO_API_KEY="your-api-key"
rootio_patcher maven remediate
```

**Output:**
```
=== DRY-RUN MODE ===
The following packages in pom.xml would be updated:

1. Package: org.springframework:spring-core
   Current version: 5.3.20
   Patched version: 5.3.23
   CVEs Fixed: [CVE-2024-11111, CVE-2024-22222]

2. Package: com.fasterxml.jackson.core:jackson-databind
   Current version: 2.13.0
   Patched version: 2.13.4
   CVEs Fixed: [CVE-2024-33333]

To apply these patches:
  1. Run: rootio_patcher maven remediate --dry-run=false
  2. Then run: mvn clean install
```

#### Apply Maven Patches

```bash
rootio_patcher maven remediate --dry-run=false
```

**Output:**
```
Applying 2 patches to pom.xml...

  - org.springframework:spring-core: 5.3.20 → 5.3.23
  - com.fasterxml.jackson.core:jackson-databind: 2.13.0 → 2.13.4

✓ Successfully updated pom.xml with 2 patches!

Next steps:
  1. Review the changes in your pom.xml
  2. Run: mvn clean install
  3. Test your application
```

#### Custom pom.xml Path

```bash
rootio_patcher maven remediate --file=./submodule/pom.xml --dry-run=false
```

### Debug Mode

Get detailed information about what's happening for any package manager:

```bash
export LOG_LEVEL=debug
rootio_patcher pip remediate
rootio_patcher npm remediate
rootio_patcher maven remediate
```

### CI/CD Integration (GitHub Actions)

#### Python Project

```yaml
name: Security Patching - Python

on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly on Sunday
  workflow_dispatch:

jobs:
  patch-python:
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
        run: ./rootio_patcher pip remediate --dry-run=false
```

#### npm/Node.js Project

```yaml
name: Security Patching - npm

on:
  schedule:
    - cron: '0 0 * * 0'
  workflow_dispatch:

jobs:
  patch-npm:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Download rootio_patcher
        run: |
          curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz
          chmod +x rootio_patcher

      - name: Patch vulnerabilities
        env:
          ROOTIO_API_KEY: ${{ secrets.ROOTIO_API_KEY }}
        run: |
          ./rootio_patcher npm remediate --package-manager=npm --dry-run=false
          npm install

      - name: Commit changes
        run: |
          git config user.name "Root.io Patcher"
          git config user.email "bot@root.io"
          git add package.json package-lock.json
          git commit -m "chore: apply Root.io security patches" || echo "No changes"
          git push
```

#### Maven Project

```yaml
name: Security Patching - Maven

on:
  schedule:
    - cron: '0 0 * * 0'
  workflow_dispatch:

jobs:
  patch-maven:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up JDK
        uses: actions/setup-java@v4
        with:
          distribution: 'temurin'
          java-version: '17'

      - name: Download rootio_patcher
        run: |
          curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz
          chmod +x rootio_patcher

      - name: Patch vulnerabilities
        env:
          ROOTIO_API_KEY: ${{ secrets.ROOTIO_API_KEY }}
        run: |
          ./rootio_patcher maven remediate --dry-run=false
          mvn clean install -DskipTests

      - name: Commit changes
        run: |
          git config user.name "Root.io Patcher"
          git config user.email "bot@root.io"
          git add pom.xml
          git commit -m "chore: apply Root.io security patches" || echo "No changes"
          git push
```

### Docker Integration

#### Python Dockerfile

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
RUN rootio_patcher pip remediate --dry-run=false

# Your application code
COPY . .
CMD ["python", "app.py"]
```

#### Node.js Dockerfile

```dockerfile
FROM node:20-slim

# Copy package files
WORKDIR /app
COPY package*.json ./

# Download rootio_patcher
RUN curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz && \
    chmod +x rootio_patcher && \
    mv rootio_patcher /usr/local/bin/

# Patch vulnerabilities
ARG ROOTIO_API_KEY
ENV ROOTIO_API_KEY=${ROOTIO_API_KEY}
RUN rootio_patcher npm remediate --package-manager=npm --dry-run=false && \
    npm install

# Your application code
COPY . .
CMD ["node", "index.js"]
```

#### Maven Dockerfile

```dockerfile
FROM maven:3.9-eclipse-temurin-17

# Copy pom.xml
WORKDIR /app
COPY pom.xml .

# Download rootio_patcher
RUN curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz && \
    chmod +x rootio_patcher && \
    mv rootio_patcher /usr/local/bin/

# Patch vulnerabilities
ARG ROOTIO_API_KEY
ENV ROOTIO_API_KEY=${ROOTIO_API_KEY}
RUN rootio_patcher maven remediate --dry-run=false && \
    mvn dependency:go-offline

# Your application code
COPY src ./src
RUN mvn package -DskipTests

CMD ["java", "-jar", "target/app.jar"]
```

---

## Troubleshooting

### General Issues

#### "Failed to load configuration: env: required environment variable 'ROOTIO_API_KEY' is not set"

**Solution:** Set your Root.io API key:
```bash
export ROOTIO_API_KEY="your-api-key"
```

#### "API returned status 401: Unauthorized"

**Solution:** Your API key is invalid or expired. Generate a new key from the Root.io dashboard.

#### "API returned status 403: Forbidden"

**Solution:** Your API key doesn't have permission to access the remediation API. Contact Root.io support.

#### "No patches needed - all packages are up to date!"

This means your packages are already secure! No action needed. ✓

### Python (pip) Issues

#### "failed to collect packages: exec: 'python': executable file not found"

**Solution:** Python is not in your PATH. Specify the full path:
```bash
rootio_patcher pip remediate --python-path=/usr/bin/python3
```

Or install Python if it's missing.

#### Patches fail to install

**Check pip permissions:**
```bash
# If you get permission errors, use a virtual environment
python3 -m venv venv
source venv/bin/activate
rootio_patcher pip remediate --dry-run=false
```

### npm/yarn/pnpm Issues

#### "lock file not found: package-lock.json"

**Solution:** Make sure you're running the command from your project root, or the lock file doesn't exist yet:
```bash
# For npm - generate lock file first
npm install

# Then run patcher
rootio_patcher npm remediate
```

#### "package.json not found in current directory"

**Solution:** Navigate to your project root where package.json exists, or create one:
```bash
npm init -y
```

#### Changes not taking effect after patching

**Solution:** You must run your package manager's install command after patching:
```bash
# After running rootio_patcher
npm install    # for npm
yarn install   # for yarn
pnpm install   # for pnpm
```

### Maven Issues

#### "file not found: pom.xml"

**Solution:** Make sure you're in your Maven project root, or specify the path:
```bash
rootio_patcher maven remediate --file=./path/to/pom.xml
```

#### Changes not taking effect after patching

**Solution:** You must rebuild your project after patching:
```bash
mvn clean install
```

#### Invalid XML after patching

**Solution:** If the pom.xml becomes invalid, restore from git and report the issue:
```bash
git checkout pom.xml
```

Then contact Root.io support with the package details that caused the issue.

---

## How It Works

### Python (pip) - Post-Install Patching

1. **Discovery**: Scans your Python environment using `pip list` to identify installed packages
2. **Analysis**: Sends package list to Root.io API to check for known vulnerabilities
3. **Reporting**: Displays available patches with CVE information
4. **Patching**: Uses `pip install` to reinstall packages with Root.io patched versions
5. **Verification**: Confirms successful installation

### npm/yarn/pnpm - Pre-Install Patching

1. **Discovery**: Parses lock file (`package-lock.json`, `yarn.lock`, or `pnpm-lock.yaml`) to identify dependencies
2. **Analysis**: Sends package list to Root.io API to check for known vulnerabilities
3. **Reporting**: Displays available patches with CVE information
4. **Patching**: Updates `package.json` with overrides/resolutions pointing to Root.io aliased packages
5. **Installation**: User runs `npm/yarn/pnpm install` to apply the overrides

### Maven - Pre-Install Patching

1. **Discovery**: Parses `pom.xml` to identify dependencies
2. **Analysis**: Sends package list to Root.io API to check for known vulnerabilities
3. **Reporting**: Displays available patches with CVE information
4. **Patching**: Updates version numbers directly in `pom.xml`
5. **Installation**: User runs `mvn clean install` to download patched versions

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

[Apache License 2.0](LICENSE)

---

## Support

- **Documentation**: [https://docs.root.io](https://docs.root.io)
- **Issues**: [GitHub Issues](https://github.com/rootio-avr/rootio_patcher/issues)
- **Email**: support@root.io

---

## Related Projects

- [Root.io Platform](https://root.io) - Comprehensive container security and vulnerability management

---

**Made with ❤️ by [Root.io](https://root.io)**
