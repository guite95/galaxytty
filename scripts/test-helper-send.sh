#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_ROOT=$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)
ADB_BIN=${ADB_BIN:-adb}
PACKAGE_NAME=com.galaxytty.helper
MARKER_PATH=files/galaxytty-reply-once

if [ "${GALAXYTTY_REAL_DEVICE_TEST:-0}" != "1" ] ||
    [ "${GALAXYTTY_ENABLE_SEND_TEST:-0}" != "1" ] ||
    [ -z "${GALAXYTTY_TEST_RECIPIENT:-}" ] ||
    [ -z "${GALAXYTTY_TEST_TEXT:-}" ]; then
    echo "Real reply test requires the device, send, recipient, and text gates." >&2
    exit 1
fi

serial=${GALAXYTTY_ADB_SERIAL:-}
if [ -z "$serial" ]; then
    connected=$(
        "$ADB_BIN" devices | awk -F '\t' 'NR > 1 && $2 == "device" { print $1 }'
    )
    count=$(printf '%s\n' "$connected" | awk 'NF { count++ } END { print count+0 }')
    if [ "$count" -ne 1 ]; then
        echo "Expected exactly one authorized ADB device; found $count." >&2
        exit 1
    fi
    serial=$connected
fi

cleanup_gate() {
    "$ADB_BIN" -s "$serial" shell run-as "$PACKAGE_NAME" unlink "$MARKER_PATH" >/dev/null 2>&1 || true
}
trap cleanup_gate EXIT HUP INT TERM

cleanup_gate
echo "Checking the authorized target and active reply capability without sending..."
go test -count=1 -tags=integration ./internal/integration -run '^TestRealHelperReplyTargetReadOnly$' -v

echo "Arming one debug-only RemoteInput execution for at most 60 seconds..."
send_not_before_seconds=$(date +%s)
GALAXYTTY_SEND_NOT_BEFORE_MILLIS=$((send_not_before_seconds * 1000))
export GALAXYTTY_SEND_NOT_BEFORE_MILLIS
"$ADB_BIN" -s "$serial" shell run-as "$PACKAGE_NAME" touch "$MARKER_PATH"

echo "Executing the explicitly authorized one-shot reply test..."
go test -count=1 -tags=integration ./internal/integration -run '^TestRealHelperReplySend$' -v

echo "Checking exact outgoing SMS Provider evidence without exposing message data..."
go test -count=1 -tags=integration ./internal/integration -run '^TestRealHelperOutgoingProviderEvidenceReadOnly$' -v

echo "One-shot RemoteInput request completed; delivery evidence must be verified separately."
