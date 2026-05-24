# Root.io Patcher

> Automated security patching for Python, npm, Maven, Go, NuGet, and Composer packages with Root.io

[![Release](https://img.shields.io/github/v/release/rootio-avr/rootio_patcher)](https://github.com/rootio-avr/rootio_patcher/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/rootio-avr/rootio_patcher)](https://golang.org/dl/)
[![License](https://img.shields.io/github/license/rootio-avr/rootio_patcher)](LICENSE)

`rootio_patcher` is a command-line tool that automatically identifies and patches vulnerabilities in your dependencies using Root.io's security fixes. It supports Python (pip), JavaScript (npm/yarn/pnpm), Java (Maven), Go (modules), .NET (NuGet), and PHP (Composer) ecosystems, providing comprehensive security remediation across your entire stack.

---

## Features

- 🔍 **Multi-Ecosystem Support** - Python (pip), JavaScript (npm/yarn/pnpm), Java (Maven), Go (modules), .NET (NuGet), and PHP (Composer)
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

# For Go modules
rootio_patcher go remediate

# For Composer (PHP) packages
rootio_patcher composer remediate

# 3. Apply patches for real
rootio_patcher pip remediate --dry-run=false
rootio_patcher npm remediate --dry-run=false
rootio_patcher maven remediate --dry-run=false
rootio_patcher go remediate --dry-run=false
rootio_patcher composer remediate --dry-run=false
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
| `ROOTIO_PIP_INDEX_URL` | Override the pip `--index-url` for Python installs (full URL, bypasses `ROOTIO_PKG_URL` construction) | - | No |
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
- `--directory`, `-C` - Project directory containing the lock file and `package.json` (default: `.` — current directory)
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

#### Go modules

```bash
rootio_patcher go remediate [FLAGS]
```

**Flags:**
- `--go-mod` - Path to go.mod (default: `go.mod`)
- `--dry-run` - Preview changes without applying (default: `true`)

**How it works:** Pre-build patching - adds `replace` directives to `go.mod` pointing to Root.io patched module aliases. The tool automatically runs `go mod tidy` after patching, and `go mod vendor` if a vendor directory is present. After running, execute `go build ./...` to build with patched modules.

> **Note:** Only modules with pinned semver versions (e.g. `v1.2.3`) are analyzed. Modules using pseudo-versions are skipped.

#### Composer (PHP)

```bash
rootio_patcher composer remediate [FLAGS]
```

**Flags:**
- `--file` - Path to composer.json (default: `composer.json`)
- `--dry-run` - Preview changes without applying (default: `true`)
- `--use-alias` - Use Root.io aliased packages (default: `false`)

**How it works:** Pre-install patching — reads exact resolved versions from `composer.lock`, updates `composer.json` with patched version constraints (pinning transitive deps as needed), adds the Root.io Composer repository entry, then automatically runs `composer update --with-dependencies <affected-packages>` to download the patched versions and update the lock file. The `vendor/` directory is fully populated and ready to deploy after the patcher runs.

> **Note:** `composer.lock` must exist before running the patcher. If it is missing, run `composer install` first. Platform requirements (`php`, `ext-*`) are not patched as they are managed by the OS, not Composer.

> **CI/CD:** After patching, subsequent `composer install` runs in CI/CD require `COMPOSER_AUTH` to be set so Composer can authenticate with `pkg.root.io` to fetch patched packages. See the [Composer CI/CD example](#composer-php-project) below.

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

#### `--use-alias` Flag (pip and Composer)

Root.io provides two types of patches:

- **Direct Patches** (`--use-alias=false`): Patches are applied using the original package name with a new patched version. This is the default for Composer.

- **Aliased Packages** (`--use-alias=true`): Root.io maintains patched versions under a different package name (e.g., `rootio-django` instead of `django` for pip, or `rootio/vendor-pkg` instead of `vendor/pkg` for Composer). This is the default for pip.

For Composer, when using aliases, the original package entry in `require`/`require-dev` is replaced with the aliased name.

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

#### Run Against a Project in Another Directory

Use `--directory` (or `-C`) to point at a project outside the current working directory:

```bash
rootio_patcher npm remediate --package-manager=npm --directory=/path/to/project --dry-run=false
# Then run: npm install inside that directory

# Relative paths work too
rootio_patcher npm remediate --package-manager=yarn -C ../frontend --dry-run=false
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

### Go Module Examples

#### Basic Usage (Dry Run)

Preview Go module patches:

```bash
export ROOTIO_API_KEY="your-api-key"
rootio_patcher go remediate
```

**Output:**
```
=== DRY-RUN MODE ===
The following replace directives would be added to go.mod:

1. replace golang.org/x/net v0.17.0 => rootio/golang.org/x/net v0.17.1
   CVEs Fixed: [CVE-2024-12345]

2. replace github.com/golang-jwt/jwt/v4 v4.5.0 => rootio/github.com/golang-jwt/jwt/v4 v4.5.1
   CVEs Fixed: [CVE-2024-67890]

To apply these patches:
  1. Run: rootio_patcher go remediate --dry-run=false
  2. Then run: go build ./...
```

#### Apply Go Module Patches

```bash
rootio_patcher go remediate --dry-run=false
```

**Output:**
```
Applying 2 patch(es) to go.mod...

  - replace golang.org/x/net v0.17.0 => rootio/golang.org/x/net v0.17.1
  - replace github.com/golang-jwt/jwt/v4 v4.5.0 => rootio/github.com/golang-jwt/jwt/v4 v4.5.1

✓ Successfully patched go.mod with 2 replace directive(s)!

Next steps:
  1. Review the changes in your go.mod
  2. Run: go build ./...
  3. Test your application
```

#### Custom go.mod Path

```bash
rootio_patcher go remediate --go-mod=./submodule/go.mod --dry-run=false
```

### Composer (PHP) Examples

#### Basic Usage (Dry Run)

Preview Composer package patches:

```bash
export ROOTIO_API_KEY="your-api-key"
rootio_patcher composer remediate
```

**Output:**
```
=== DRY-RUN MODE ===
The following packages in composer.json would be updated:

1. Package: vendor/package
   Current version: 2.1.0
   Patched version: 2.1.4
   CVEs Fixed: [CVE-2024-12345]

To apply these patches:
  Run: rootio_patcher composer remediate --dry-run=false
  Then ensure COMPOSER_AUTH is configured in your CI/CD environment
```

#### Apply Composer Patches

```bash
rootio_patcher composer remediate --dry-run=false
```

**Output:**
```
Applying 1 patch(es) to composer.json...

  - vendor/package: 2.1.0 → vendor/package@2.1.4

✓ Successfully patched composer.json with 1 update(s)!

Next steps:
  1. Review the changes in composer.json and composer.lock
  2. Ensure COMPOSER_AUTH is configured in your CI/CD environment for future installs
  3. Test and deploy your application
```

#### Custom composer.json Path

```bash
rootio_patcher composer remediate --file=./subproject/composer.json --dry-run=false
```

#### Use Aliased Packages

```bash
rootio_patcher composer remediate --dry-run=false --use-alias=true
```

### Java (Maven) Support - Beta Status

⚠️ **Beta Notice**: Java support is currently in beta.

**Current Support:**
- ✅ Maven projects (`pom.xml`)
- ❌ Gradle projects (coming soon)

**How It Works:**

The Maven patcher implements a comprehensive strategy to eliminate duplicate dependencies:

1. **Direct dependencies**: Updates vulnerable packages to use Root.io aliased versions (e.g., `io.netty:netty-codec-http2` → `io.root.io.netty:netty-codec-http2`)

2. **Transitive dependencies**: Explicitly adds Root.io patched versions for packages that are transitively included

3. **Exclusions**: Adds `<exclusions>` to all dependencies that are NOT being patched, preventing them from transitively pulling in vulnerable versions

**Example:**
```xml
<dependency>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-webflux</artifactId>
    <version>2.7.0</version>
    <exclusions>
        <!-- Prevents spring from pulling in vulnerable netty -->
        <exclusion>
            <groupId>io.netty</groupId>
            <artifactId>netty-codec-http2</artifactId>
        </exclusion>
    </exclusions>
</dependency>
```

This strategy ensures that only Root.io patched versions are used throughout your dependency tree, eliminating duplicate class warnings.

### Debug Mode

Get detailed information about what's happening for any package manager:

```bash
export LOG_LEVEL=debug
rootio_patcher pip remediate
rootio_patcher npm remediate
rootio_patcher maven remediate
rootio_patcher go remediate
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

#### Go Project

```yaml
name: Security Patching - Go

on:
  schedule:
    - cron: '0 0 * * 0'
  workflow_dispatch:

jobs:
  patch-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Download rootio_patcher
        run: |
          curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz
          chmod +x rootio_patcher

      - name: Patch vulnerabilities
        env:
          ROOTIO_API_KEY: ${{ secrets.ROOTIO_API_KEY }}
        run: |
          ./rootio_patcher go remediate --dry-run=false
          go build ./...

      - name: Commit changes
        run: |
          git config user.name "Root.io Patcher"
          git config user.email "bot@root.io"
          git add go.mod go.sum
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

#### Composer (PHP) Project

```yaml
name: Security Patching - Composer

on:
  schedule:
    - cron: '0 0 * * 0'
  workflow_dispatch:

jobs:
  patch-composer:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up PHP
        uses: shivammathur/setup-php@v2
        with:
          php-version: '8.2'

      - name: Install dependencies
        run: composer install --no-interaction

      - name: Download rootio_patcher
        run: |
          curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz
          chmod +x rootio_patcher

      - name: Patch vulnerabilities
        env:
          ROOTIO_API_KEY: ${{ secrets.ROOTIO_API_KEY }}
        run: ./rootio_patcher composer remediate --dry-run=false

      - name: Commit changes
        run: |
          git config user.name "Root.io Patcher"
          git config user.email "bot@root.io"
          git add composer.json composer.lock
          git commit -m "chore: apply Root.io security patches" || echo "No changes"
          git push
```

> **Note:** Add `COMPOSER_AUTH` to your CI/CD secrets for subsequent `composer install` runs that need to fetch patched packages from `pkg.root.io`:
> ```yaml
> env:
>   COMPOSER_AUTH: '{"http-basic":{"pkg.root.io":{"username":"","password":"${{ secrets.ROOTIO_API_KEY }}"}}}'
> ```

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

#### Go Dockerfile

```dockerfile
FROM golang:1.22-bookworm

WORKDIR /app

# Copy module files
COPY go.mod go.sum ./

# Download rootio_patcher
RUN curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz && \
    chmod +x rootio_patcher && \
    mv rootio_patcher /usr/local/bin/

# Patch vulnerabilities
ARG ROOTIO_API_KEY
ENV ROOTIO_API_KEY=${ROOTIO_API_KEY}
RUN rootio_patcher go remediate --dry-run=false

# Download dependencies and build
COPY . .
RUN go build -o app ./...

CMD ["./app"]
```

#### Composer (PHP) Dockerfile

```dockerfile
FROM php:8.2-cli

# Install Composer
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer

WORKDIR /app
COPY composer.json composer.lock ./

# Download rootio_patcher
RUN curl -sL https://github.com/rootio-avr/rootio_patcher/releases/latest/download/rootio_patcher_linux_x86_64.tar.gz | tar xz && \
    chmod +x rootio_patcher && \
    mv rootio_patcher /usr/local/bin/

# Install dependencies and patch vulnerabilities
ARG ROOTIO_API_KEY
ENV ROOTIO_API_KEY=${ROOTIO_API_KEY}
RUN composer install --no-interaction && \
    rootio_patcher composer remediate --dry-run=false

# Your application code
COPY . .
CMD ["php", "index.php"]
```

> Subsequent `composer install` steps (e.g. in a multi-stage build) require `COMPOSER_AUTH` to be set so Composer can authenticate with `pkg.root.io`.

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

## Vulnerability Gate

Use `--dry-run` (the default) as a non-destructive check in CI pipelines. The tool exits with a specific code depending on the outcome:

| Exit code | Meaning |
|-----------|---------|
| `0` | No patches needed — all packages are up to date |
| `1` | Error (bad config, API failure, unexpected panic) |
| `2` | Patches are available — action required |

This lets you fail a pipeline, open a ticket, or send an alert purely based on whether vulnerabilities exist, without touching any files.

### Shell

```bash
rootio_patcher npm remediate  # --dry-run=true is the default
case $? in
  0) echo "All packages are up to date." ;;
  2) echo "Patches available — please remediate." ; exit 1 ;;
  *) echo "Unexpected error." ; exit 1 ;;
esac
```

### CI

```bash
rootio_patcher npm remediate
EXIT_CODE=$?
if [ $EXIT_CODE -eq 2 ]; then
  echo "Security patches are available. Run with --dry-run=false to apply them."
  exit 1
elif [ $EXIT_CODE -ne 0 ]; then
  echo "rootio_patcher encountered an error."
  exit 1
fi
```

---

## GitHub Action

A reusable composite action is included in this repository. It wraps the vulnerability gate above — runs `rootio_patcher` in dry-run mode and **fails the job** if patches are available. No files are ever modified.

```yaml
- uses: rootio-avr/rootio_patcher/.github/actions/rootio-patch@main
  with:
    api-key: ${{ secrets.ROOTIO_API_KEY }}
    ecosystem: npm   # pip | npm | maven | go | nuget | composer
```

### Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `api-key` | Yes | — | Root.io API key |
| `ecosystem` | Yes | — | `pip`, `npm`, `maven`, `go`, `nuget`, or `composer` |
| `working-directory` | No | `.` | Directory inside the repo where the tool should run (e.g. `services/api`) |
| `package-manager` | No | `npm` | *(npm)* `npm`, `yarn`, or `pnpm` |
| `directory` | No | `.` | *(npm)* Project directory containing the lock file |
| `python-path` | No | `python` | *(pip)* Path to Python interpreter |
| `use-alias` | No | `true` | *(pip)* / *(composer)* Use Root.io aliased packages |
| `file` | No | `pom.xml` | *(maven)* Path to pom.xml; *(composer)* Path to composer.json |

Advanced settings (`ROOTIO_API_URL`, `ROOTIO_PKG_URL`, `ROOTIO_PIP_INDEX_URL`, `LOG_LEVEL`) are not inputs — pass them as environment variables on the calling step instead:

```yaml
- uses: rootio-avr/rootio_patcher/.github/actions/rootio-patch@main
  env:
    ROOTIO_PIP_INDEX_URL: https://my-private-index.example.com/simple/
  with:
    api-key: ${{ secrets.ROOTIO_API_KEY }}
    ecosystem: pip
```

### Outputs

| Output | Description |
|--------|-------------|
| `patches-available` | `"true"` if patches were found, `"false"` if everything is up to date |

### Examples

#### Block a PR if vulnerabilities exist (npm)

```yaml
name: Security Check

on: [pull_request]

jobs:
  vuln-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: rootio-avr/rootio_patcher/.github/actions/rootio-patch@main
        with:
          api-key: ${{ secrets.ROOTIO_API_KEY }}
          ecosystem: npm
          package-manager: npm
```

#### Python project

```yaml
      - uses: actions/setup-python@v5
        with:
          python-version: '3.11'
      - run: pip install -r requirements.txt
      - uses: rootio-avr/rootio_patcher/.github/actions/rootio-patch@main
        with:
          api-key: ${{ secrets.ROOTIO_API_KEY }}
          ecosystem: pip
```

#### Maven project

```yaml
      - uses: rootio-avr/rootio_patcher/.github/actions/rootio-patch@main
        with:
          api-key: ${{ secrets.ROOTIO_API_KEY }}
          ecosystem: maven
          file: path/to/pom.xml
```

#### Warn without blocking (using the output)

```yaml
      - uses: rootio-avr/rootio_patcher/.github/actions/rootio-patch@main
        id: vuln-check
        continue-on-error: true
        with:
          api-key: ${{ secrets.ROOTIO_API_KEY }}
          ecosystem: npm
      - if: steps.vuln-check.outputs.patches-available == 'true'
        run: echo "Patches available — consider remediating soon."
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

**Solution:** Either run from your project root, use `--directory` to point at it, or generate the lock file first:
```bash
# Run from the project directory
cd /path/to/project && rootio_patcher npm remediate

# Or use --directory / -C
rootio_patcher npm remediate --directory=/path/to/project

# If the lock file doesn't exist yet, generate it first
npm install
rootio_patcher npm remediate
```

#### "package.json not found in current directory"

**Solution:** Navigate to your project root, use `--directory`, or create a package.json:
```bash
rootio_patcher npm remediate --directory=/path/to/project
# or
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

### Go Module Issues

#### "file not found: go.mod"

**Solution:** Make sure you're in your Go module root, or specify the path:
```bash
rootio_patcher go remediate --go-mod=./path/to/go.mod
```

#### "go mod tidy failed"

**Solution:** Ensure `go` is installed and accessible in your PATH, and that the Root.io module proxy is reachable. You may need to configure `GOPROXY` to include Root.io's module proxy.

#### Changes not taking effect after patching

**Solution:** Build your project after patching:
```bash
go build ./...
```

If you use a vendor directory, the tool runs `go mod vendor` automatically after patching.

#### No patches found but vulnerabilities are expected

**Solution:** Modules with pseudo-versions (e.g. `v0.0.0-20230101123456-abcdef012345`) are skipped. Only pinned semver releases (e.g. `v1.2.3`) are analyzed. Upgrade to a pinned release if possible.

### Composer Issues

#### "composer.lock not found"

**Solution:** Run `composer install` first to generate the lock file, then re-run the patcher:
```bash
composer install
rootio_patcher composer remediate
```

#### "file not found: composer.json"

**Solution:** Make sure you're in your project root, or specify the path:
```bash
rootio_patcher composer remediate --file=./path/to/composer.json
```

#### `composer update` fails after patching

**Solution:** Ensure `COMPOSER_AUTH` is set so Composer can authenticate with `pkg.root.io`:
```bash
export COMPOSER_AUTH='{"http-basic":{"pkg.root.io":{"username":"","password":"your-api-key"}}}'
rootio_patcher composer remediate --dry-run=false
```

#### Future `composer install` fails in CI/CD

**Solution:** The `repositories` entry pointing to `pkg.root.io` is permanent in `composer.json`. Every subsequent `composer install` needs `COMPOSER_AUTH` set in the environment to fetch patched packages. Add it to your CI/CD secrets.

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

### Go - Pre-Build Patching

1. **Discovery**: Parses `go.mod` to identify all required modules with pinned semver versions (pseudo-versions are skipped)
2. **Analysis**: Sends the module list to Root.io API to check for known vulnerabilities
3. **Reporting**: Displays `replace` directives that would be added with CVE information
4. **Patching**: Adds `replace` directives to `go.mod` pointing to Root.io patched module aliases
5. **Tidy**: Automatically runs `go mod tidy` (and `go mod vendor` if a vendor directory exists)
6. **Build**: User runs `go build ./...` to compile with the patched modules

### Composer (PHP) - Pre-Install Patching

1. **Discovery**: Reads exact resolved versions from `composer.lock` (must exist; run `composer install` first). Platform requirements (`php`, `ext-*`) are skipped.
2. **Analysis**: Sends the package list to Root.io API to check for known vulnerabilities
3. **Reporting**: Displays packages and patched versions with CVE information
4. **Patching**: Updates `composer.json` — direct dependencies have their version constraint updated in place; transitive dependencies are explicitly pinned in `require`. The Root.io Composer repository entry is added permanently.
5. **Install**: Automatically runs `composer update --with-dependencies <affected-packages>` with credentials passed via `COMPOSER_AUTH` environment variable. The `vendor/` directory is fully populated after this step — no separate install needed.
6. **Deploy**: Application is ready to pack and deploy. Future `composer install` runs in CI/CD require `COMPOSER_AUTH` to fetch patched packages from `pkg.root.io`.

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
