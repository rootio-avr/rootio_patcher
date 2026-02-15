## Poetry

Configure `.netrc` as in previous steps

### Configuration

Run


```
poetry source add --priority=primary root https://pkg.root.io/pypi/simple/
poetry source add --priority=supplemental pypi
```

Or manually edit `pyproject.toml`

```
[tool.poetry]
packages = [{include = "poetry_example", from = "src"}]

[[tool.poetry.source]]
name = "root"
url = "https://pkg.root.io/pypi/simple/"
priority = "primary"
```

### Installation

Run

```
poetry add requests==2.25.1
```

> NOTE: No need to add `+root...` suffix to package name, it will be resolved automatically

### Updating


Run 

```
poetry update
```

In order to update revision if you store `poetry.lock` file in you VCS


### Comments

Don't user `Poetry` at all, consider migration to `uv`


## UV


## UV

Configure `.netrc` as in previous steps

### Configuration

Manually edit `pyproject.toml`, set

```
[[tool.uv.index]]
name = "root"
url = "https://pkg.root.io/pypi/simple/"
```

### Installation

Run

```
uv add requests==2.25.1
```

> NOTE: No need to add `+root...` suffix to package name, it will be resolved automatically

### Updating

Run 

```
uv sync --upgrade
```

In order to update revision if you store `uv.lock` file in you VCS