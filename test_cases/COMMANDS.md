# PyPI Patcher Commands

The PyPI patcher uses **ONLY environment variables** for configuration (no command-line flags).

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ROOTIO_API_KEY` | **Yes** | - | Root.io API key for authentication |
| `ROOTIO_API_URL` | No | `https://api.root.io` | Root.io API URL |
| `ROOTIO_PKG_URL` | No | `https://pkg.root.io` | Root.io package registry URL (supports http/https) |
| `PYTHON_PATH` | No | `python` | Python executable (e.g., `python3`, `/path/to/venv/bin/python`) |
| `DRY_RUN` | No | `true` | Dry run mode (set to `false` to apply patches) |
| `USE_ALIAS` | No | `true` | Use aliased packages (e.g., `rootio_Flask`). Set to `false` for non-aliased (e.g., `Flask`) |
| `LOG_LEVEL` | No | `info` | Logging level: `debug`, `info`, `warn`, `error` |

**Notes:**
- If `PYTHON_PATH` is not set, defaults to `python` (assumes it's in PATH or venv is activated)
- `USE_ALIAS=true` installs packages with `rootio_` prefix (e.g., `rootio_Flask==2.1.2+root.io.1`)
- `USE_ALIAS=false` installs packages without prefix (e.g., `Flask==2.1.2+root.io.1`)
- `LOG_LEVEL=debug` shows detailed operational logs (useful for troubleshooting)
- `LOG_LEVEL=info` (default) shows only user-facing messages with clean output

---

## Common Usage Examples

### Production Use (Apply Patches)
```bash
# Using aliased packages (default)
ROOTIO_API_KEY=your_api_key DRY_RUN=false go run ./cmd/pypi_patcher

# Using non-aliased packages
ROOTIO_API_KEY=your_api_key DRY_RUN=false USE_ALIAS=false go run ./cmd/pypi_patcher
```

### Testing with Local/Staging Server
```bash
# Local development server
ROOTIO_API_URL=http://localhost:3000 \
ROOTIO_PKG_URL=http://localhost:8080 \
ROOTIO_API_KEY=test_key \
DRY_RUN=true \
go run ./cmd/pypi_patcher

# Staging server
ROOTIO_API_URL=https://staging-api.root.io \
ROOTIO_PKG_URL=https://staging-pkg.root.io \
ROOTIO_API_KEY=staging_key \
go run ./cmd/pypi_patcher
```

### Debugging
```bash
# Enable debug logging to see all operational details
LOG_LEVEL=debug \
ROOTIO_API_KEY=your_api_key \
go run ./cmd/pypi_patcher
```

---

## Test Case 1: Only Patchable

```bash
# Setup
cd /Users/virviil/slim/core/cmd/pypi_patcher/test_cases/01_only_patchable
bash setup.sh

# Activate venv and run patcher (dry run with aliased packages)
cd /Users/virviil/slim/core
source ./cmd/pypi_patcher/test_cases/01_only_patchable/.venv/bin/activate
ROOTIO_API_URL=http://localhost:3000 \
ROOTIO_PKG_URL=http://localhost:3000 \
ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F \
DRY_RUN=true \
go run ./cmd/pypi_patcher

# Or specify Python path explicitly (without activating venv)
PYTHON_PATH=./cmd/pypi_patcher/test_cases/01_only_patchable/.venv/bin/python \
ROOTIO_API_URL=http://localhost:3000 \
ROOTIO_PKG_URL=http://localhost:3000 \
ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F \
DRY_RUN=true \
go run ./cmd/pypi_patcher

# Run patcher (apply patches with non-aliased packages)
ROOTIO_API_URL=http://localhost:3000 \
ROOTIO_PKG_URL=http://localhost:3000 \
ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F \
DRY_RUN=false \
USE_ALIAS=false \
go run ./cmd/pypi_patcher

# Run with debug logging
LOG_LEVEL=debug \
ROOTIO_API_URL=http://localhost:3000 \
ROOTIO_PKG_URL=http://localhost:3000 \
ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F \
go run ./cmd/pypi_patcher
```

---

## Test Case 2: Only Non-Patchable

```bash
# Setup
cd /Users/virviil/slim/core/cmd/pypi_patcher/test_cases/02_only_non_patchable
bash setup.sh

# Run patcher (with venv activated)
cd /Users/virviil/slim/core
source ./cmd/pypi_patcher/test_cases/02_only_non_patchable/.venv/bin/activate
ROOTIO_API_URL=http://localhost:3000 \
ROOTIO_PKG_URL=http://localhost:3000 \
ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F \
go run ./cmd/pypi_patcher
```

---

## Test Case 3: Mixed

```bash
# Setup
cd /Users/virviil/slim/core/cmd/pypi_patcher/test_cases/03_mixed
bash setup.sh

# Run patcher (with explicit Python path)
cd /Users/virviil/slim/core
PYTHON_PATH=./cmd/pypi_patcher/test_cases/03_mixed/.venv/bin/python \
ROOTIO_API_URL=http://localhost:3000 \
ROOTIO_PKG_URL=http://localhost:3000 \
ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F \
go run ./cmd/pypi_patcher
```

---

## Test Case 4: Transient Dependencies

```bash
# Setup
cd /Users/virviil/slim/core/cmd/pypi_patcher/test_cases/04_transient_deps
bash setup.sh

# Run patcher
cd /Users/virviil/slim/core
source ./cmd/pypi_patcher/test_cases/04_transient_deps/.venv/bin/activate
ROOTIO_API_URL=http://localhost:3000 \
ROOTIO_PKG_URL=http://localhost:3000 \
ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F \
go run ./cmd/pypi_patcher
```

---

## Quick One-Liner for Each Test Case

```bash
# Test 1 (dry run with aliased packages)
cd /Users/virviil/slim/core && source ./cmd/pypi_patcher/test_cases/01_only_patchable/.venv/bin/activate && ROOTIO_API_URL=http://localhost:3000 ROOTIO_PKG_URL=http://localhost:3000 ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F go run ./cmd/pypi_patcher

# Test 1 (apply patches with non-aliased packages)
cd /Users/virviil/slim/core && source ./cmd/pypi_patcher/test_cases/01_only_patchable/.venv/bin/activate && ROOTIO_API_URL=http://localhost:3000 ROOTIO_PKG_URL=http://localhost:3000 ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F DRY_RUN=false USE_ALIAS=false go run ./cmd/pypi_patcher

# Test 2
cd /Users/virviil/slim/core && source ./cmd/pypi_patcher/test_cases/02_only_non_patchable/.venv/bin/activate && ROOTIO_API_URL=http://localhost:3000 ROOTIO_PKG_URL=http://localhost:3000 ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F go run ./cmd/pypi_patcher

# Test 3
cd /Users/virviil/slim/core && source ./cmd/pypi_patcher/test_cases/03_mixed/.venv/bin/activate && ROOTIO_API_URL=http://localhost:3000 ROOTIO_PKG_URL=http://localhost:3000 ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F go run ./cmd/pypi_patcher

# Test 4
cd /Users/virviil/slim/core && source ./cmd/pypi_patcher/test_cases/04_transient_deps/.venv/bin/activate && ROOTIO_API_URL=http://localhost:3000 ROOTIO_PKG_URL=http://localhost:3000 ROOTIO_API_KEY=sk_h7WG6CV277FA7KcqfqlzXYy3tk9LBV8F go run ./cmd/pypi_patcher
```
