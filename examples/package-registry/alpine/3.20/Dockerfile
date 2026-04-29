# syntax=docker/dockerfile:1.6
ARG ALPINE_VERSION=3.20
FROM alpine:${ALPINE_VERSION}

# Install signing key, configure Root.io repository, install packages, and verify
ARG ALPINE_VERSION
RUN --mount=type=secret,id=rootio_api_key \
    # Install Alpine public key for package signature verification
    echo "LS0tLS1CRUdJTiBSU0EgUFVCTElDIEtFWS0tLS0tCk1JSUNDZ0tDQWdFQXNvNzBvakxQRG1jdXVEV3ptcGRnRmZ1Q3Y1TVBsdXBVUUN3Z2QzUEVoSVhiYUFPU1F5a1AKOVFkQVdaeVF1UEduOGlST3dDa2JDMUl4bmxQZU9ldVZIdVhhU0dRYnRUN09McU9yaWJHb0ttRVI1dHRHZlVVMgpybXV0ekphTVczcjY4ZVNqVUJlVVdkYXRtVDFNNnhWWi8wUjRyRitiRkdleExzMTZ0ZHl6YldTWjJEbmJUNTd4ClZsQmFKUThjZ3ZyMmhWN2txZmk2VUllNlZUcnJQMkwvMmlmTlB3L2MyUGdJcnV1cXRjVDdaaUxERmdidnllWUMKWTkxWjlvNnVGb3RSTlF6eVphQTQwQkVEeisyQ3oyUkorOFl3NkJvYUYrTHRkMWVzbTh6UGE4b1c2TTNQaTY5cwpJRlYvR3Z5UzZ1NzcvdXhwN1VwTSsvaVMvZ0pONFFobWJuVTlDamsyeGs1M2ZSamVBQzY2bldodWhSTi9Wd0hRCmUwNVJ1TmliZGFHVzR5eVlmbGtSMVpVZFhsVVhDUkJFazF3UU9Ed0Fka1NuRmwrWlFWS2lVdy9iMDJneXY2YzMKWHp5emZlbXdMcm5zMDZaUEwzbTE2VFlrT3BkQ2NCb0tUQ1NNaTZ6SDdCb211dlpUaTUzSGVkV2NDV2JTcDFIVAp0VFpHMmxmdXJVK1AyQndEZmVJWGxXMWJ0TlMwUDVjSTczOVYzTS9CSW1IYzRCQzRhVWorSWw2dHdqTzZUNlYxCnAwU3BIakJESjBaVVJ2WDZmMDFGWENnMXJqdzZ1T2xrdExrZTNGQUNzUjl5U0VkZkpKTE1IMDlFU1dYbE9RMWoKNnVqZlRWVzdKekFwcDkrV0R2Yzd1U3lzc2dlS3BxY3JuTkd2SlhYbGQxTlcyNWRwSXp0ZXlMc0NBd0VBQVE9PQotLS0tLUVORCBSU0EgUFVCTElDIEtFWS0tLS0tCg==" | \
    base64 -d > /etc/apk/keys/root@alpinelinux.org.rsa.pub && \
    \
    # Add pkg.root.io as repository with authentication from secret
    echo "https://root:$(cat /run/secrets/rootio_api_key)@pkg.root.io/alpine/${ALPINE_VERSION}" >> /etc/apk/repositories && \
    \
    # Update package index and install packages, preferring Root.io versions if available
    apk update && \
    \
    for pkg in curl git vim bash libgit2-dev tini; do \
    if apk search -e "rootio-$pkg" | grep -q "rootio-$pkg"; then \
    apk add --no-cache "rootio-$pkg"; \
    else \
    apk add --no-cache "$pkg"; \
    fi; \
    done && \
    \
    # Remove Root.io repository line with credentials from repositories file
    sed -i '/pkg\.root\.io/d' /etc/apk/repositories && \
    \
    echo "=== Installed packages ===" && \
    curl --version && \
    echo "" && \
    echo "=== Repository configuration ===" && \
    cat /etc/apk/repositories && \
    echo "" && \
    echo "=== Installed signing keys ===" && \
    ls -la /etc/apk/keys/

CMD ["/bin/sh"]
