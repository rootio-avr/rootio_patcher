# Test Case 7: Yarn Modern (Yarn 2+/Berry)

This test case demonstrates npm package remediation with **Yarn Modern** (Yarn 2+/Berry/v4+).

## What's Different from Yarn 1?

- **Lock file format**: Uses YAML format with `__metadata` field
- **Lock file version**: `version: 8` (Yarn v4+)
- **Package format**: `"package@npm:version"` instead of `package@^version`
- **Checksum format**: Uses `10c0/` prefix for checksums
- **PnP support**: Yarn 2+ supports Plug'n'Play by default

## Package Manager Comparison

| Feature | test_cases/5 (Yarn 1) | test_cases/7 (Yarn 2+) |
|---------|----------------------|------------------------|
| Lock file | `yarn.lock` (v1) | `yarn.lock` (v8) |
| Format | Custom text format | YAML |
| Parser | `YarnParser` | `Yarn2Parser` |
| Overrides field | `resolutions` | `resolutions` |
| Auto-detection | ✅ | ✅ |

## Setup

```bash
cd test_cases/7
./setup.sh
```

This will:
1. Enable corepack
2. Set Yarn version to stable (v4+)
3. Install dependencies using Yarn Modern
4. Generate Yarn 2+ lockfile

## Testing Remediation

```bash
ROOTIO_API_KEY=your_key rootio_patcher npm remediate --package-manager yarn --dry-run=true
```

**Auto-detection**: The `--package-manager yarn` option automatically detects the Yarn version from the lockfile format:
- Detects `__metadata` → Uses Yarn 2+ parser
- Detects `# yarn lockfile v1` → Uses Yarn 1 parser

## Expected packages.json

```json
{
  "name": "test-project",
  "dependencies": {
    "lodash": "^4.17.23"
  },
  "devDependencies": {
    "mocha": "8.0.0",
    "@babel/runtime": "7.25.0"
  },
  "resolutions": {
    "vulnerable-package": "npm:@rootio/vulnerable-package@patched-version"
  }
}
```

## Files in this directory

- `package.json` - NPM package manifest
- `yarn.lock` - Yarn 2+ lock file (YAML format)
- `.yarnrc.yml` - Yarn configuration (created by `yarn set version`)
- `.yarn/` - Yarn installation directory
- `setup.sh` - Setup script for this test case
- `.gitignore` - Ignores Yarn 2+ cache files

## Yarn Version Information

Check your Yarn version:
```bash
yarn --version
# Should output: 4.x.x
```

## Notes

- Yarn 2+ is also called "Yarn Berry" or "Yarn Modern"
- Yarn 2+ requires Node.js 18.12+
- Uses corepack for version management
- Supports workspaces natively
- PnP (Plug'n'Play) mode is default but can use node_modules mode
