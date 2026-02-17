# syntax=docker/dockerfile:1.6
ARG ALPINE_VERSION=3.20
FROM alpine:${ALPINE_VERSION}

# Install signing key, configure Root.io repository, install packages, and verify
ARG ALPINE_VERSION
RUN --mount=type=secret,id=rootio_api_key \
    # Install Alpine public key for package signature verification
    echo "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUlJQklqQU5CZ2txaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUFvcG1yUjR1em95WEFiTHFiRUE3cwpBNWQrVytnUzE0TzEwMlJHUjhxa0h0cDhSUEs2b3FvZkhJZzNYSWpnSVlzalQwYTk1VXNzbGFlL2FDWU1UeFBVCmhnSmxkTVU1SnlzZmYxT1BEQld4R2ZCOFJCL081cjA1cU9JLzJ3QlgwQjl5NjhhU0xQZi9UWEE3akY5STRKL3EKdjArWVF2c3owRWdqUEFjeVN2MlgvOHE2R2RoOHRkWG4rbTVBYjJoUU5YUE1TdTM3aXMwUVR3Uk9DSFV4ZXZLeAo0OUZ5UGkvQUR5UDB6bVFYaUtCQW52alN6YVNQNmZPQm1yYlNNY05wYnVPV2lwUUJoajRnVE5KOVNzaDZSQmdmCmJVUlFhdmlwWlAyL0RiNTNiLzJ5UTJJU21QMEo1cmdjWnkrdXoyZndTWmtHb3pUa0ErZDE1dHludlJRVFQwbGkKWXdJREFRQUIKLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg==" | \
    base64 -d > /etc/apk/keys/root@alpinelinux.org-67b85bd5.rsa.pub && \
    \
    # Add pkg.root.io as repository with authentication from secret
    echo "https://root:$(cat /run/secrets/rootio_api_key)@pkg.root.io/alpine/${ALPINE_VERSION}" >> /etc/apk/repositories && \
    \
    # Update package index and install packages
    apk update && \
    apk add --no-cache curl git vim bash libgit2-dev tini && \
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
