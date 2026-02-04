ARG ALPINE_VERSION=3.20
FROM alpine:${ALPINE_VERSION}

# Install the Alpine public key for package signature verification
# This is the official Alpine signing key (root@alpinelinux.org-67b85bd5.rsa.pub)
RUN echo "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUlJQklqQU5CZ2txaGtpRzl3MEJBUUVGQUFPQ0FROEFNSUlCQ2dLQ0FRRUFvcG1yUjR1em95WEFiTHFiRUE3cwpBNWQrVytnUzE0TzEwMlJHUjhxa0h0cDhSUEs2b3FvZkhJZzNYSWpnSVlzalQwYTk1VXNzbGFlL2FDWU1UeFBVCmhnSmxkTVU1SnlzZmYxT1BEQld4R2ZCOFJCL081cjA1cU9JLzJ3QlgwQjl5NjhhU0xQZi9UWEE3akY5STRKL3EKdjArWVF2c3owRWdqUEFjeVN2MlgvOHE2R2RoOHRkWG4rbTVBYjJoUU5YUE1TdTM3aXMwUVR3Uk9DSFV4ZXZLeAo0OUZ5UGkvQUR5UDB6bVFYaUtCQW52alN6YVNQNmZPQm1yYlNNY05wYnVPV2lwUUJoajRnVE5KOVNzaDZSQmdmCmJVUlFhdmlwWlAyL0RiNTNiLzJ5UTJJU21QMEo1cmdjWnkrdXoyZndTWmtHb3pUa0ErZDE1dHludlJRVFQwbGkKWXdJREFRQUIKLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg==" | \
    base64 -d > /etc/apk/keys/root@alpinelinux.org-67b85bd5.rsa.pub

# Add pkg.root.io as SECONDARY repository (append, don't replace)
ARG ALPINE_VERSION=3.20
ARG ROOTIO_API_KEY
RUN echo "https://root:${ROOTIO_API_KEY}@pkg.root.io/alpine/${ALPINE_VERSION}" >> /etc/apk/repositories

# Update package index and install curl for testing
RUN apk update && apk add --no-cache curl

# Display versions to confirm installation
RUN echo "=== Installed packages ===" && \
    curl --version && \
    echo "" && \
    echo "=== Repository configuration ===" && \
    cat /etc/apk/repositories && \
    echo "" && \
    echo "=== Installed signing keys ===" && \
    ls -la /etc/apk/keys/

# Set default command to shell for interactive testing
CMD ["/bin/sh"]
