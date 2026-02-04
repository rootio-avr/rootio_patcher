FROM ubuntu:noble

# Install required dependencies
RUN apt-get update && apt-get install -y \
    gnupg \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Add Root.io GPG key and repository
ARG ROOTIO_API_KEY
RUN echo "LS0tLS1CRUdJTiBQR1AgUFVCTElDIEtFWSBCTE9DSy0tLS0tCgptRE1FYVlIQ1dSWUpLd1lCQkFIYVJ3OEJBUWRBcDdXVHNLMTVrWTNmQ0pxOUNRVnlxODluRzFoNEw4OHZvVndqCnB0NGNXSjYwSkZKdmIzUXVhVzhnUVZCVUlGSmxjRzl6YVhSdmNua2dQR0Z3ZEVCeWIyOTBMbWx2UG9pVEJCTVcKQ2dBN0ZpRUUzSVVhWTlLRDFsTUhKYTdNZ09RM004RHd3c2tGQW1tQndsa0NHd01GQ3drSUJ3SUNJZ0lHRlFvSgpDQXNDQkJZQ0F3RUNIZ2NDRjRBQUNna1FnT1EzTThEd3dzbGY2d0QrSWxqSGRkVmFKM2xKYjBsSE0rZVFubWNvCnlmTTlpWis5cXI0SjBNYnZsNG9CQUtOL0pYZkJvR2JGYzgzM0ZmN1I5R3M5UXU2bm1EUVZlSDI4eHEwdDRwWU4KPWs3ZHMKLS0tLS1FTkQgUEdQIFBVQkxJQyBLRVkgQkxPQ0stLS0tLQo=" | \
    base64 -d | gpg --dearmor -o /etc/apt/keyrings/rootio.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/rootio.gpg] https://root:${ROOTIO_API_KEY}@pkg.root.io/ubuntu/noble noble main" > /etc/apt/sources.list.d/rootio.list

# Update package index
RUN apt-get update

# Display configuration for verification
RUN echo "=== Root.io APT Repository Configuration ===" && \
    cat /etc/apt/sources.list.d/rootio.list && \
    echo "" && \
    echo "=== GPG Key Location ===" && \
    ls -la /etc/apt/keyrings/rootio.gpg && \
    echo "" && \
    echo "=== Installed curl version ===" && \
    curl --version

# Set default command to bash
CMD ["/bin/bash"]
