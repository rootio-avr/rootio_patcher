# PyPI Patcher Test Cases

This directory contains test cases for validating PyPI package vulnerability scanning and remediation functionality.

## Overview

Each test case creates an isolated Python virtual environment with specific packages that have known vulnerabilities. These test cases help validate:

- Vulnerability detection accuracy
- Patch availability detection
- Remediation recommendations
- Transient dependency handling

## Test Cases

### 01_only_patchable

**Purpose:** Test packages with vulnerabilities that have available patches.

**Packages:**
- `Flask==2.1.2` - Web framework with known CVE
- `cryptography==36.0.2` - Cryptographic library with security fixes available
- `certifi==2020.12.5` - Certificate verification with outdated certs
- `Authlib==1.6.4` - OAuth library with 8 known vulnerabilities
- `eventlet==0.33.1` - Async networking with security issues
- `gevent==21.12.0` - Coroutine-based networking
- `aiomysql==0.2.0` - MySQL async driver
- `Brotli==1.1.0` - Compression library
- `asteval==1.0.5` - Expression evaluator
- `fastmcp==2.3.4` - MCP framework with 7 vulnerabilities
- `gunicorn==21.2.0` - WSGI server with 2 vulnerabilities
- `flower==1.1.0` - Celery monitoring tool

**Expected Result:** All vulnerabilities should have patches available. Scanner should recommend upgrading to patched versions.

---

### 02_only_non_patchable

**Purpose:** Test packages with vulnerabilities that don't have straightforward patches (major version upgrades required or packages no longer maintained).

**Packages:**
- `Django==1.11.29` - End-of-life version with many CVEs
- `Pillow==8.1.0` - Image processing with breaking changes in newer versions
- `requests==2.6.0` - Very old HTTP library
- `PyYAML==3.12` - YAML parser with security issues
- `urllib3==1.24.1` - HTTP client with old vulnerabilities
- `Jinja2==2.10.1` - Template engine with XSS issues
- `paramiko==2.4.2` - SSH library with old crypto
- `lxml==4.6.2` - XML parser with vulnerabilities
- `bleach==3.1.0` - HTML sanitizer with bypass issues
- `notebook==5.7.8` - Jupyter notebook with security issues

**Expected Result:** Vulnerabilities present but patches require major version upgrades or breaking changes. Scanner should flag these as requiring manual intervention.

---

### 03_mixed

**Purpose:** Test realistic scenario with both patchable and non-patchable vulnerabilities.

**Packages:**

**Patchable:**
- `Flask==2.1.2`
- `gunicorn==21.2.0`
- `cryptography==36.0.2`
- `certifi==2020.12.5`
- `eventlet==0.33.1`
- `aiomysql==0.2.0`

**Non-Patchable:**
- `Django==1.11.29`
- `Pillow==8.1.0`
- `PyYAML==3.12`
- `urllib3==1.24.1`
- `paramiko==2.4.2`

**Safe (no vulnerabilities):**
- `click==8.1.7`
- `colorama==0.4.6`
- `python-dateutil==2.8.2`
- `six==1.16.0`
- `setuptools==69.0.0`

**Expected Result:** Scanner should distinguish between easily patchable vulnerabilities and those requiring manual intervention. Should provide clear categorization.

---

### 04_transient_deps

**Purpose:** Test vulnerability detection in transient (indirect) dependencies.

**Direct Dependencies:**
- `apache-airflow==2.3.3` - Workflow orchestration (pulls in many dependencies)
- `apache-airflow-providers-amazon==4.0.0`
- `apache-airflow-providers-mysql==3.0.0`
- `apache-airflow-providers-google==8.1.0`
- `celery==5.2.3` - Task queue (pulls in eventlet/gevent)
- `SQLAlchemy==1.4.25` - Database ORM
- `pandas==1.5.3` - Data analysis
- `numpy==1.24.0` - Numerical computing

**Expected Vulnerable Transient Dependencies:**
- `Flask` (pulled by apache-airflow)
- `Jinja2` (pulled by Flask/airflow)
- `cryptography` (pulled by various packages)
- `certifi` (pulled by requests)
- `eventlet` (pulled by celery)
- `urllib3` (pulled by requests)

**Expected Result:** Scanner should detect vulnerabilities in transient dependencies and provide clear dependency chain information (e.g., "apache-airflow → Flask → Jinja2").

---

## Usage

### Run All Test Cases

```bash
cd cmd/pypi_patcher/test_cases
bash run_all.sh
```

### Run Single Test Case

```bash
# Run specific test case
bash run_all.sh 01_only_patchable

# Or run directly
cd 01_only_patchable
bash setup.sh
```

### Activate Test Environment

After running a test case, activate its virtual environment:

```bash
source 01_only_patchable/.venv/bin/activate
```

### Clean Up

Remove all virtual environments:

```bash
find . -type d -name ".venv" -exec rm -rf {} +
```

---

## Test Validation Checklist

When running the PyPI patcher against these test cases, verify:

- [ ] **01_only_patchable**: All vulnerabilities detected, patches available
- [ ] **02_only_non_patchable**: Vulnerabilities detected, no easy patches flagged
- [ ] **03_mixed**: Correct categorization of patchable vs non-patchable
- [ ] **04_transient_deps**: Transient dependencies scanned, clear dependency chains shown
- [ ] **Performance**: Reasonable scan time for ~15-30 packages
- [ ] **Accuracy**: No false positives or false negatives
- [ ] **Reporting**: Clear, actionable remediation advice

---

## Adding New Test Cases

To add a new test case:

1. Create directory: `mkdir 05_your_test_case`
2. Create `requirements.txt` with package versions
3. Create `setup.sh` (copy and modify from existing cases)
4. Make executable: `chmod +x 05_your_test_case/setup.sh`
5. Update `run_all.sh` to include new test case
6. Document in this README

---

## Notes

- All packages use pinned versions to ensure reproducibility
- Test cases use real CVEs from public vulnerability databases
- Virtual environments are isolated - safe to run concurrently
- Some installations may take several minutes (especially apache-airflow)
- Python 3.8+ recommended for compatibility

---

## Vulnerability Reference

For reference, here are the expected vulnerability counts per package (approximate):

| Package | Version | Expected CVEs | Patches Available |
|---------|---------|---------------|-------------------|
| Flask | 2.1.2 | 1 | Yes |
| cryptography | 36.0.2 | 1 | Yes |
| Authlib | 1.6.4 | 8 | Yes |
| fastmcp | 2.3.4 | 7 | Yes |
| gunicorn | 21.2.0 | 2 | Yes |
| Django | 1.11.29 | 20+ | Major upgrade |
| Pillow | 8.1.0 | 5+ | Yes (breaking) |
| Jinja2 | 2.10.1 | 3+ | Yes |

Note: Actual CVE counts may vary based on vulnerability database version and time of scan.
