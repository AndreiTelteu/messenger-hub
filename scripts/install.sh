#!/usr/bin/env bash
set -Eeuo pipefail

APP_ID="io.github.messengerhub.MessengerHub"
APP_NAME="Messenger Hub"
PROJECT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
PREFIX="/usr/local"
INSTALL_MODE="system"
RUN_TESTS=1

usage() {
  cat <<'EOF'
Build and install Messenger Hub with a GNOME application launcher.

Usage: ./scripts/install.sh [options]

Options:
  --system          Install into /usr/local (default; uses sudo when needed)
  --user            Install into ~/.local without sudo
  --prefix PATH     Install into a custom absolute prefix without sudo
  --skip-tests      Build without running the Go test suite first
  -h, --help        Show this help
EOF
}

while (($#)); do
  case "$1" in
    --system)
      PREFIX="/usr/local"
      INSTALL_MODE="system"
      ;;
    --user)
      PREFIX="${HOME}/.local"
      INSTALL_MODE="user"
      ;;
    --prefix)
      shift
      if (($# == 0)); then
        echo "Error: --prefix requires an absolute path." >&2
        exit 2
      fi
      PREFIX="$1"
      INSTALL_MODE="custom"
      ;;
    --skip-tests)
      RUN_TESTS=0
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if [[ "$PREFIX" != /* ]]; then
  echo "Error: the installation prefix must be an absolute path." >&2
  exit 2
fi
if [[ "$PREFIX" =~ [[:space:]] ]]; then
  echo "Error: paths containing whitespace are not supported in the desktop launcher." >&2
  exit 2
fi

for command in go install; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "Error: required command not found: $command" >&2
    exit 1
  fi
done

if [[ "$INSTALL_MODE" == "system" && $EUID -ne 0 ]]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "Error: sudo is required for a system installation." >&2
    exit 1
  fi
  ELEVATE=(sudo)
else
  ELEVATE=()
fi

BINDIR="${PREFIX}/bin"
DATADIR="${PREFIX}/share"
APPLICATIONSDIR="${DATADIR}/applications"
ICONDIR="${DATADIR}/icons/hicolor/scalable/apps"
BINARY="${PROJECT_DIR}/bin/messenger-hub"
DESKTOP_SOURCE="${PROJECT_DIR}/packaging/${APP_ID}.desktop"
ICON_SOURCE="${PROJECT_DIR}/packaging/${APP_ID}.svg"

desktop_file="$(mktemp --suffix=.desktop)"
cleanup() {
  rm -f -- "$desktop_file"
}
trap cleanup EXIT

echo "Building ${APP_NAME}…"
cd "$PROJECT_DIR"
if ((RUN_TESTS)); then
  go test ./...
fi
mkdir -p bin
go build -trimpath -o "$BINARY" ./cmd/messenger-hub

sed "s|^Exec=.*$|Exec=${BINDIR}/messenger-hub|" "$DESKTOP_SOURCE" >"$desktop_file"
if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "$desktop_file"
fi

echo "Installing into ${PREFIX}…"
"${ELEVATE[@]}" install -Dm755 "$BINARY" "${BINDIR}/messenger-hub"
"${ELEVATE[@]}" install -Dm644 "$desktop_file" "${APPLICATIONSDIR}/${APP_ID}.desktop"
"${ELEVATE[@]}" install -Dm644 "$ICON_SOURCE" "${ICONDIR}/${APP_ID}.svg"

if command -v update-desktop-database >/dev/null 2>&1; then
  "${ELEVATE[@]}" update-desktop-database "$APPLICATIONSDIR"
fi
if command -v gtk4-update-icon-cache >/dev/null 2>&1 && [[ -f "${DATADIR}/icons/hicolor/index.theme" ]]; then
	"${ELEVATE[@]}" gtk4-update-icon-cache -f -t "${DATADIR}/icons/hicolor"
fi

echo
echo "${APP_NAME} is installed. Open GNOME Activities and search for “${APP_NAME}”."
echo "Binary: ${BINDIR}/messenger-hub"
echo "Launcher: ${APPLICATIONSDIR}/${APP_ID}.desktop"
