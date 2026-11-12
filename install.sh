#!/usr/bin/env bash
set -euo pipefail

REPO="${AGYS_REPO:-Nox1KCL/Agys}"
VERSION="${AGYS_VERSION:-latest}"
INSTALL_DIR="${AGYS_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="agys"
FORCE_LOCAL=0

usage() {
	cat <<'EOF'
Usage: install.sh [OPTIONS]

Installs agys on Linux:
  1. If curl is available, tries downloading the release binary from GitHub (Nox1KCL/Agys).
  2. Otherwise (or if unavailable), looks for a local binary (./bin/agys, ./agys)
     or builds from source via `go build` when source files are present.
  3. Installs to ~/.local/bin/agys and adds the directory to PATH.

Options:
  --local        Skip GitHub download and use a local binary / build from source
  -h, --help     Show this help message

Environment variables:
  AGYS_REPO          GitHub repository (default: Nox1KCL/Agys)
  AGYS_VERSION       Release tag or "latest" (default: latest)
  AGYS_INSTALL_DIR   Installation directory (default: ~/.local/bin)
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--local)
		FORCE_LOCAL=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Error: unknown argument '$1'" >&2
		usage >&2
		exit 1
		;;
	esac
done

log() {
	printf '[agys] %s\n' "$*"
}

fail() {
	printf '[agys] Error: %s\n' "$*" >&2
	exit 1
}

# 1. Linux only.
OS_NAME="$(uname -s)"
if [[ "$OS_NAME" != "Linux" ]]; then
	fail "only Linux is supported (detected: ${OS_NAME})."
fi

# 2. Detect CPU architecture.
RAW_ARCH="$(uname -m)"
case "$RAW_ARCH" in
x86_64 | amd64)
	ARCH="amd64"
	;;
aarch64 | arm64)
	ARCH="arm64"
	;;
*)
	fail "unsupported CPU architecture: ${RAW_ARCH} (supported: amd64, arm64)."
	;;
esac

# Resolve script directory even when invoked via relative path.
SCRIPT_DIR=""
if [[ -n "${BASH_SOURCE[0]:-}" && -e "${BASH_SOURCE[0]}" ]]; then
	SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
	rm -rf "$TMP_DIR"
}
trap cleanup EXIT

SOURCE_BIN=""

try_download_release() {
	command -v curl >/dev/null 2>&1 || return 1

	local base_url
	if [[ "$VERSION" == "latest" ]]; then
		base_url="https://github.com/${REPO}/releases/latest/download"
	else
		base_url="https://github.com/${REPO}/releases/download/${VERSION}"
	fi

	# Support both .tar.gz archives and raw binaries in GitHub Releases.
	local archive_url="${base_url}/${BIN_NAME}_linux_${ARCH}.tar.gz"
	local archive_path="${TMP_DIR}/${BIN_NAME}.tar.gz"

	log "Trying GitHub Releases (${REPO}, ${VERSION}, linux/${ARCH})..."
	if command -v tar >/dev/null 2>&1 && curl -fsSL --connect-timeout 5 "$archive_url" -o "$archive_path" 2>/dev/null; then
		if tar -xzf "$archive_path" -C "$TMP_DIR" 2>/dev/null; then
			local extracted
			extracted="$(find "$TMP_DIR" -maxdepth 2 -type f -name "$BIN_NAME" | head -n1 || true)"
			if [[ -n "$extracted" && -s "$extracted" ]]; then
				SOURCE_BIN="$extracted"
				log "Downloaded release archive from GitHub."
				return 0
			fi
		fi
	fi

	local raw_urls=(
		"${base_url}/${BIN_NAME}-linux-${ARCH}"
		"${base_url}/${BIN_NAME}_linux_${ARCH}"
	)
	local raw_path="${TMP_DIR}/${BIN_NAME}"
	for url in "${raw_urls[@]}"; do
		if curl -fsSL --connect-timeout 5 "$url" -o "$raw_path" 2>/dev/null && [[ -s "$raw_path" ]]; then
			SOURCE_BIN="$raw_path"
			log "Downloaded release binary from GitHub."
			return 0
		fi
	done

	return 1
}

find_local_binary() {
	local candidates=(
		"./bin/${BIN_NAME}"
		"./${BIN_NAME}"
	)
	if [[ -n "$SCRIPT_DIR" ]]; then
		candidates+=(
			"${SCRIPT_DIR}/bin/${BIN_NAME}"
			"${SCRIPT_DIR}/${BIN_NAME}"
		)
	fi

	local dest_real
	dest_real="$(readlink -m "${INSTALL_DIR}/${BIN_NAME}" 2>/dev/null || echo "${INSTALL_DIR}/${BIN_NAME}")"

	for cand in "${candidates[@]}"; do
		if [[ -f "$cand" && -s "$cand" ]]; then
			local cand_real
			cand_real="$(readlink -m "$cand" 2>/dev/null || echo "$cand")"
			if [[ "$cand_real" == "$dest_real" ]]; then
				continue
			fi
			SOURCE_BIN="$cand"
			log "Found local binary: ${cand}"
			return 0
		fi
	done

	return 1
}

try_local_go_build() {
	command -v go >/dev/null 2>&1 || return 1

	local build_root=""
	if [[ -f "./go.mod" ]]; then
		build_root="$(pwd)"
	elif [[ -n "$SCRIPT_DIR" && -f "${SCRIPT_DIR}/go.mod" ]]; then
		build_root="$SCRIPT_DIR"
	else
		return 1
	fi

	local built="${TMP_DIR}/${BIN_NAME}"
	log "No prebuilt local binary found — building from source (${build_root})..."
	if (cd "$build_root" && go build -o "$built" ./cmd/agys); then
		SOURCE_BIN="$built"
		return 0
	fi
	return 1
}

if [[ "$FORCE_LOCAL" -eq 0 ]]; then
	try_download_release || true
fi

if [[ -z "$SOURCE_BIN" ]]; then
	if [[ "$FORCE_LOCAL" -eq 0 ]] && command -v curl >/dev/null 2>&1; then
		log "GitHub release unavailable — checking local paths..."
	fi
	find_local_binary || try_local_go_build || fail "could not find an agys binary (neither in GitHub Releases nor locally at ./bin/agys or ./agys)."
fi

# 3. Install binary.
mkdir -p "$INSTALL_DIR"
TARGET_BIN="${INSTALL_DIR}/${BIN_NAME}"
install -m 755 "$SOURCE_BIN" "$TARGET_BIN"
log "Installed: ${TARGET_BIN}"

# 4. Add directory to PATH idempotently for fish, bash, zsh, or .profile.
path_entry_exists() {
	local rc_file="$1"
	[[ -f "$rc_file" ]] || return 1

	if grep -Fq "$INSTALL_DIR" "$rc_file"; then
		return 0
	fi
	if [[ "$INSTALL_DIR" == "$HOME/.local/bin" ]] && grep -Fq '.local/bin' "$rc_file"; then
		return 0
	fi
	return 1
}

append_posix_path() {
	local rc_file="$1"
	local path_expr="$INSTALL_DIR"
	if [[ "$INSTALL_DIR" == "$HOME/.local/bin" ]]; then
		path_expr='$HOME/.local/bin'
	fi

	if path_entry_exists "$rc_file"; then
		return 1
	fi

	mkdir -p "$(dirname "$rc_file")"
	{
		printf '\n# Added by agys installer\n'
		printf 'export PATH="%s:$PATH"\n' "$path_expr"
	} >>"$rc_file"
	log "Added ${INSTALL_DIR} to PATH in ${rc_file}"
	return 0
}

append_fish_path() {
	local fish_cfg="$HOME/.config/fish/config.fish"
	local path_expr="$INSTALL_DIR"
	if [[ "$INSTALL_DIR" == "$HOME/.local/bin" ]]; then
		path_expr='~/.local/bin'
	fi

	if path_entry_exists "$fish_cfg"; then
		return 1
	fi

	mkdir -p "$(dirname "$fish_cfg")"
	{
		printf '\n# Added by agys installer\n'
		printf 'fish_add_path -m %s\n' "$path_expr"
	} >>"$fish_cfg"
	log "Added ${INSTALL_DIR} to PATH in ${fish_cfg}"
	return 0
}

configure_shell_paths() {
	local updated=0
	local matched_any=0
	local current_shell
	current_shell="$(basename "${SHELL:-}")"

	if [[ "$current_shell" == "fish" || -f "$HOME/.config/fish/config.fish" || -d "$HOME/.config/fish" ]]; then
		matched_any=1
		if append_fish_path; then
			updated=1
		fi
	fi

	if [[ "$current_shell" == "zsh" || -f "$HOME/.zshrc" ]]; then
		matched_any=1
		if append_posix_path "$HOME/.zshrc"; then
			updated=1
		fi
	fi

	if [[ "$current_shell" == "bash" || -f "$HOME/.bashrc" ]]; then
		matched_any=1
		if append_posix_path "$HOME/.bashrc"; then
			updated=1
		fi
	fi

	if [[ "$matched_any" -eq 0 ]]; then
		if append_posix_path "$HOME/.profile"; then
			updated=1
		fi
	fi

	case ":${PATH}:" in
	*":${INSTALL_DIR}:"*)
		log "${INSTALL_DIR} is already in your current PATH."
		;;
	*)
		if [[ "$updated" -eq 1 ]]; then
			log "Restart your shell or source your config file to update PATH."
		else
			log "${INSTALL_DIR} is already configured in your shell profile."
		fi
		;;
	esac
}

configure_shell_paths
log "Done! Run: agys"
