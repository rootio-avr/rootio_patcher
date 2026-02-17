# syntax=docker/dockerfile:1.6
FROM ubuntu:noble

# Install dependencies, configure Root.io repository, install packages, and verify
RUN --mount=type=secret,id=rootio_api_key \
    DEBIAN_FRONTEND=noninteractive apt-get update && \
    # Install minimal dependencies for adding repositories and installing packages
    apt-get install -y --no-install-recommends gnupg ca-certificates && \
    \
    # Initialize keyring and add Root.io GPG key
    mkdir -p /etc/apt/keyrings && \
    echo "LS0tLS1CRUdJTiBQR1AgUFVCTElDIEtFWSBCTE9DSy0tLS0tCgptRE1FYVlIQ1dSWUpLd1lCQkFIYVJ3OEJBUWRBcDdXVHNLMTVrWTNmQ0pxOUNRVnlxODluRzFoNEw4OHZvVndqCnB0NGNXSjYwSkZKdmIzUXVhVzhnUVZCVUlGSmxjRzl6YVhSdmNua2dQR0Z3ZEVCeWIyOTBMbWx2UG9pVEJCTVcKQ2dBN0ZpRUUzSVVhWTlLRDFsTUhKYTdNZ09RM004RHd3c2tGQW1tQndsa0NHd01GQ3drSUJ3SUNJZ0lHRlFvSgpDQXNDQkJZQ0F3RUNIZ2NDRjRBQUNna1FnT1EzTThEd3dzbGY2d0QrSWxqSGRkVmFKM2xKYjBsSE0rZVFubWNvCnlmTTlpWis5cXI0SjBNYnZsNG9CQUtOL0pYZkJvR2JGYzgzM0ZmN1I5R3M5UXU2bm1EUVZlSDI4eHEwdDRwWU4KPWs3ZHMKLS0tLS1FTkQgUEdQIFBVQkxJQyBLRVkgQkxPQ0stLS0tLQo=" \
    | base64 -d | gpg --dearmor -o /etc/apt/keyrings/rootio.gpg && \
    \
    # Create APT auth.conf.d entry for Root.io repository using the API key from Docker secrets
    mkdir -p /etc/apt/auth.conf.d && \
    printf "machine pkg.root.io\nlogin root\npassword %s\n" \
    "$(cat /run/secrets/rootio_api_key)" > /etc/apt/auth.conf.d/rootio.conf && \
    chmod 600 /etc/apt/auth.conf.d/rootio.conf && \
    \
    # Configure APT to use the Root.io repository with authentication and GPG key
    echo "deb [signed-by=/etc/apt/keyrings/rootio.gpg] https://pkg.root.io/ubuntu/noble noble main" \
    > /etc/apt/sources.list.d/rootio.list && \
    \
    # Update package lists and install packages, preferring Root.io versions if available
    DEBIAN_FRONTEND=noninteractive apt-get update && \
    \
    for pkg in curl git vim bash libgit2-dev tini; do \
    if apt-cache show "rootio-$pkg" >/dev/null 2>&1; then \
    apt-get install -y --no-install-recommends "rootio-$pkg"; \
    else \
    apt-get install -y --no-install-recommends "$pkg"; \
    fi; \
    done && \
    \
    rm -f /etc/apt/auth.conf.d/rootio.conf && \
    rm -rf /var/lib/apt/lists/* && \
    \
    echo "=== Root.io APT Repository Configuration ===" && \
    cat /etc/apt/sources.list.d/rootio.list && \
    echo "" && \
    echo "=== GPG Key Location ===" && \
    ls -la /etc/apt/keyrings/rootio.gpg && \
    echo "" && \
    echo "=== Installed curl version ===" && \
    curl --version

CMD ["/bin/bash"]
