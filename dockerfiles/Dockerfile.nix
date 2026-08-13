FROM registry.suse.com/bci/bci-base:15.7@sha256:bb6858a8fb2127c7373ff01975f7c6ba1be893c1f70a0d81be72aacec4871742

LABEL org.opencontainers.image.source="https://github.com/rancher/ci-image" \
      org.opencontainers.image.title="Rancher nix CI image" \
      org.opencontainers.image.description="Nix environment"

ARG TARGETARCH
ENV ARCH=$TARGETARCH
ENV GH_TELEMETRY=false
ENV DO_NOT_TRACK=true
ENV PATH="/var/ci-tools/active:${PATH}"

RUN zypper -n refresh && \
    zypper -n install \
        gettext-runtime \
        ca-certificates \
        docker \
        gawk \
        git-core \
        gzip \
        jq \
        make \
        tar \
        unzip \
        zstd \
        wget \
        sudo \
        vim \
    && \
    zypper -n clean -a && \
    rm -rf /var/log/{lastlog,tallylog,zypper.log,zypp/history,YaST2}

# Create runner group (GID 121) and user (UID 1001) early for use in tool installations.
# /var/ci-tools/ is set up with setgid (2755) so subdirectories inherit the runner group.
# This allows any user added to the runner group to access tools extracted to /var/ci-tools/.
RUN groupadd -g 121 runner && \
    useradd -u 1001 -g 121 -m runner && \
    mkdir -p /var/ci-tools && \
    chown root:runner /var/ci-tools && \
    chmod 2755 /var/ci-tools

# cosign v3.1.2
RUN case "${ARCH}" in \
        amd64) CHECKSUM="f7622ed3cf22e55e1ae6377c080979ff77a22da9981c11df222a2e444991e7cf" ;; \
        arm64) CHECKSUM="90e7ae0b5dfd60f20816b52c012addf7fc055ebcc7bea4ce81c428ca8518c302" ;; \
        *) echo "Unsupported: ${ARCH}"; exit 1 ;; \
    esac && \
    export TMP_DIR=$(mktemp -d) && \
    case "${ARCH}" in \
        amd64) DOWNLOAD_URL="https://github.com/sigstore/cosign/releases/download/v3.1.2/cosign-linux-amd64" ;; \
        arm64) DOWNLOAD_URL="https://github.com/sigstore/cosign/releases/download/v3.1.2/cosign-linux-arm64" ;; \
    esac && \
    curl -fsSL --retry 3 --retry-delay 5 --retry-all-errors "${DOWNLOAD_URL}" > "${TMP_DIR}/cosign" && \
    printf "%s  %s\n" "${CHECKSUM}" "${TMP_DIR}/cosign" > "${TMP_DIR}/checksum.sha256" && \
    sha256sum -c "${TMP_DIR}/checksum.sha256" && \
    install "${TMP_DIR}/cosign" "/usr/local/bin/cosign" && \
    rm -rf "${TMP_DIR}"

# gh v2.96.0
RUN case "${ARCH}" in \
        amd64) CHECKSUM="83d5c2ccad5498f58bf6368acb1ab32588cf43ab3a4b1c301bf36328b1c8bd60" ;; \
        arm64) CHECKSUM="06f86ec7103d41993b76cd78072f43595c34aaa56506d971d9860e67140bf909" ;; \
        *) echo "Unsupported: ${ARCH}"; exit 1 ;; \
    esac && \
    export TMP_DIR=$(mktemp -d) && \
    export TMP_FILE="${TMP_DIR}/gh.tar.gz" && \
    case "${ARCH}" in \
        amd64) DOWNLOAD_URL="https://github.com/cli/cli/releases/download/v2.96.0/gh_2.96.0_linux_amd64.tar.gz"; EXTRACT="gh_2.96.0_linux_amd64/bin/gh" ;; \
        arm64) DOWNLOAD_URL="https://github.com/cli/cli/releases/download/v2.96.0/gh_2.96.0_linux_arm64.tar.gz"; EXTRACT="gh_2.96.0_linux_arm64/bin/gh" ;; \
    esac && \
    curl -fsSL --retry 3 --retry-delay 5 --retry-all-errors "${DOWNLOAD_URL}" > "${TMP_FILE}" && \
    printf "%s  %s\n" "${CHECKSUM}" "${TMP_FILE}" > "${TMP_DIR}/checksum.sha256" && \
    sha256sum -c "${TMP_DIR}/checksum.sha256" && \
    tar xzf "${TMP_FILE}" -C "${TMP_DIR}" && \
    install "${TMP_DIR}/${EXTRACT}" "/usr/local/bin/gh" && \
    rm -rf "${TMP_DIR}"

# helmv3 v3.21.3
RUN case "${ARCH}" in \
        amd64) CHECKSUM="15e041a93a590dce8100f39385cd98c84a765c9e36aeeb9e2dc6ff9e4769e2e0" ;; \
        arm64) CHECKSUM="67f58155079ff9ffab98ba5c88daff0ed9b542f3a4732f5dd426dde7dd0f5244" ;; \
        *) echo "Unsupported: ${ARCH}"; exit 1 ;; \
    esac && \
    export TMP_DIR=$(mktemp -d) && \
    export TMP_FILE="${TMP_DIR}/helmv3.tar.gz" && \
    case "${ARCH}" in \
        amd64) DOWNLOAD_URL="https://get.helm.sh/helm-v3.21.3-linux-amd64.tar.gz"; EXTRACT="linux-amd64/helm" ;; \
        arm64) DOWNLOAD_URL="https://get.helm.sh/helm-v3.21.3-linux-arm64.tar.gz"; EXTRACT="linux-arm64/helm" ;; \
    esac && \
    curl -fsSL --retry 3 --retry-delay 5 --retry-all-errors "${DOWNLOAD_URL}" > "${TMP_FILE}" && \
    printf "%s  %s\n" "${CHECKSUM}" "${TMP_FILE}" > "${TMP_DIR}/checksum.sha256" && \
    sha256sum -c "${TMP_DIR}/checksum.sha256" && \
    tar xzf "${TMP_FILE}" -C "${TMP_DIR}" && \
    install "${TMP_DIR}/${EXTRACT}" "/usr/local/bin/helmv3" && \
    rm -rf "${TMP_DIR}"

# helmv4 v4.2.3
RUN case "${ARCH}" in \
        amd64) CHECKSUM="e9b88b4ee95b18c706839c28d3a0220e5bc470e9cd9262410c90793c45ff8b7c" ;; \
        arm64) CHECKSUM="21abd9354d39b2cd79a8d76be6912cd137a983cbf997193503fb8a6a6e2f2785" ;; \
        *) echo "Unsupported: ${ARCH}"; exit 1 ;; \
    esac && \
    export TMP_DIR=$(mktemp -d) && \
    export TMP_FILE="${TMP_DIR}/helmv4.tar.gz" && \
    case "${ARCH}" in \
        amd64) DOWNLOAD_URL="https://get.helm.sh/helm-v4.2.3-linux-amd64.tar.gz"; EXTRACT="linux-amd64/helm" ;; \
        arm64) DOWNLOAD_URL="https://get.helm.sh/helm-v4.2.3-linux-arm64.tar.gz"; EXTRACT="linux-arm64/helm" ;; \
    esac && \
    curl -fsSL --retry 3 --retry-delay 5 --retry-all-errors "${DOWNLOAD_URL}" > "${TMP_FILE}" && \
    printf "%s  %s\n" "${CHECKSUM}" "${TMP_FILE}" > "${TMP_DIR}/checksum.sha256" && \
    sha256sum -c "${TMP_DIR}/checksum.sha256" && \
    tar xzf "${TMP_FILE}" -C "${TMP_DIR}" && \
    install "${TMP_DIR}/${EXTRACT}" "/usr/local/bin/helmv4" && \
    rm -rf "${TMP_DIR}"

# slsactl v0.1.35
RUN case "${ARCH}" in \
        amd64) CHECKSUM="dd8f75429cc629a4b36bf92297420d3147a140f545d3a92f2b8ffb8414f0d10b" ;; \
        arm64) CHECKSUM="f68817d75ebe0ed0a14d55b9c637968244c38ff087a8fae21c89092f2d44db54" ;; \
        *) echo "Unsupported: ${ARCH}"; exit 1 ;; \
    esac && \
    export TMP_DIR=$(mktemp -d) && \
    export TMP_FILE="${TMP_DIR}/slsactl.tar.gz" && \
    case "${ARCH}" in \
        amd64) DOWNLOAD_URL="https://github.com/rancherlabs/slsactl/releases/download/v0.1.35/slsactl_0.1.35_linux_amd64.tar.gz"; EXTRACT="slsactl" ;; \
        arm64) DOWNLOAD_URL="https://github.com/rancherlabs/slsactl/releases/download/v0.1.35/slsactl_0.1.35_linux_arm64.tar.gz"; EXTRACT="slsactl" ;; \
    esac && \
    curl -fsSL --retry 3 --retry-delay 5 --retry-all-errors "${DOWNLOAD_URL}" > "${TMP_FILE}" && \
    printf "%s  %s\n" "${CHECKSUM}" "${TMP_FILE}" > "${TMP_DIR}/checksum.sha256" && \
    sha256sum -c "${TMP_DIR}/checksum.sha256" && \
    tar xzf "${TMP_FILE}" -C "${TMP_DIR}" && \
    install "${TMP_DIR}/${EXTRACT}" "/usr/local/bin/slsactl" && \
    rm -rf "${TMP_DIR}"

# yq v4.53.3
RUN case "${ARCH}" in \
        amd64) CHECKSUM="fa52a4e758c63d38299163fbdd1edfb4c4963247918bf9c1c5d31d84789eded4" ;; \
        arm64) CHECKSUM="578648e463a11c1b6db6010cbf41eafed6bee79466fcffa1bb446672cf7945ea" ;; \
        *) echo "Unsupported: ${ARCH}"; exit 1 ;; \
    esac && \
    export TMP_DIR=$(mktemp -d) && \
    case "${ARCH}" in \
        amd64) DOWNLOAD_URL="https://github.com/mikefarah/yq/releases/download/v4.53.3/yq_linux_amd64" ;; \
        arm64) DOWNLOAD_URL="https://github.com/mikefarah/yq/releases/download/v4.53.3/yq_linux_arm64" ;; \
    esac && \
    curl -fsSL --retry 3 --retry-delay 5 --retry-all-errors "${DOWNLOAD_URL}" > "${TMP_DIR}/yq" && \
    printf "%s  %s\n" "${CHECKSUM}" "${TMP_DIR}/yq" > "${TMP_DIR}/checksum.sha256" && \
    sha256sum -c "${TMP_DIR}/checksum.sha256" && \
    install "${TMP_DIR}/yq" "/usr/local/bin/yq" && \
    rm -rf "${TMP_DIR}"

# nix 2.35.1

# Pre-install setup for nix
# Create unprivileged user for Nix installation
RUN useradd -m suse && \
    if [ ! -f /etc/sudoers ]; then touch /etc/sudoers; fi && \
    echo "suse ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Add suse user to runner group and create /etc/nix directory and configuration
RUN usermod -a -G runner suse && \
    sudo mkdir -p /etc/nix && \
    printf "build-users-group =\nsandbox = false\nfilter-syscalls = false\n" > /etc/nix/nix.conf && \
    sudo chown -R suse:runner /etc/nix && \
    sudo mkdir -p /nix && \
    sudo chown -R suse:runner /nix && \
    echo 'source /home/suse/.nix-profile/etc/profile.d/nix.sh' > /etc/profile.d/nix.sh && \
    echo 'source /home/suse/.nix-profile/etc/profile.d/nix.sh' > /etc/bash.bashrc.local

RUN case "${ARCH}" in \
        amd64) CHECKSUM="c3fe29778acaa93b5095ee66e36f11ec7c6a284c40970a24cc83ac4f04809db3" ;; \
        arm64) CHECKSUM="79b739996f1751573b4d2b56e4ae607855184c711f2cc1274fa0952a13d4bfc9" ;; \
        *) echo "Unsupported: ${ARCH}"; exit 1 ;; \
    esac && \
    export INSTALL_DIR="/var/ci-tools/nix" && \
    mkdir -p "${INSTALL_DIR}" && \
    export TMP_DIR=$(mktemp -d) && \
    export TMP_FILE="${TMP_DIR}/nix.tar.xz" && \
    case "${ARCH}" in \
        amd64) DOWNLOAD_URL="https://releases.nixos.org/nix/nix-2.35.1/nix-2.35.1-x86_64-linux.tar.xz"; EXTRACT="nix-2.35.1-x86_64-linux/" ;; \
        arm64) DOWNLOAD_URL="https://releases.nixos.org/nix/nix-2.35.1/nix-2.35.1-aarch64-linux.tar.xz"; EXTRACT="nix-2.35.1-aarch64-linux/" ;; \
    esac && \
    curl -fsSL --retry 3 --retry-delay 5 --retry-all-errors "${DOWNLOAD_URL}" > "${TMP_FILE}" && \
    printf "%s  %s\n" "${CHECKSUM}" "${TMP_FILE}" > "${TMP_DIR}/checksum.sha256" && \
    sha256sum -c "${TMP_DIR}/checksum.sha256" && \
    tar xJf "${TMP_FILE}" -C "${TMP_DIR}" && \
    FULL_EXTRACT_PATH="${TMP_DIR}/${EXTRACT}" && \
    EXTRACT_DIR=$(dirname "${FULL_EXTRACT_PATH}") && \
    if [ "${EXTRACT_DIR}" != "${TMP_DIR}" ]; then \
        cp -a "${FULL_EXTRACT_PATH}" "${INSTALL_DIR}/"; \
    else \
        (cd "${FULL_EXTRACT_PATH}" && cp -a . "${INSTALL_DIR}/"); \
    fi && \
    rm -rf "${TMP_DIR}"

# Post-install setup for nix
# Fix ownership and run Nix installer from the extracted archive
RUN set -e; \
    sudo chown -R suse:runner /var/ci-tools/nix

# Switch to unprivileged user for installation
USER suse
WORKDIR /home/suse
ENV USER=suse

RUN set -e; \
    cd /var/ci-tools/nix && \
    ./install --no-daemon

RUN set -e; \
    install -d .config/nix/profiles

# Restore root user for remaining Dockerfile operations
USER root
ENV USER=root

# Family selectors — copy scripts and set up manifest + active symlinks.
# /var/ci-tools/active is on PATH ahead of /usr/local/bin; runner can update
# the active symlink with: ci-select <family> <tool>  or  select-<family> <tool>
COPY dockerfiles/scripts/select-helm.sh /usr/local/bin/select-helm
COPY dockerfiles/scripts/ci-select.sh /usr/local/bin/ci-select
COPY dockerfiles/scripts/ci-env-init.sh /usr/local/bin/ci-env-init
RUN chmod +x /usr/local/bin/select-helm && chmod +x /usr/local/bin/ci-select && \
    chmod +x /usr/local/bin/ci-env-init


# Set up CI tool family infrastructure (runner user and group created earlier).
RUN mkdir -p /var/ci-tools/active \
    && mkdir -p /usr/local/share/ci-tools/families/helm \
    && touch /usr/local/share/ci-tools/families/helm/helmv3 \
    && touch /usr/local/share/ci-tools/families/helm/helmv4 \
    && ln -sf helmv4 /usr/local/share/ci-tools/families/helm/default \
    && ln -sf /usr/local/bin/helmv4 /var/ci-tools/active/helm \
    && chown -R root:runner /var/ci-tools \
    && chmod 2775 /var/ci-tools/active

# We trust our base image and the repos that are pulled in workflows. Otherwise
# each workflow that uses our base images would have to add the step below.
RUN git config --system --add safe.directory '*'

# Auto-configure tool families from {FAMILY}_VERSION env vars, then exec user command.
# Users can set HELM_VERSION=helmv4 to auto-select helmv4 as the active helm tool.
ENTRYPOINT ["ci-env-init"]
CMD ["/bin/bash"]
