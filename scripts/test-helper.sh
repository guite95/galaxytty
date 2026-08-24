#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_ROOT=$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)
. "${SCRIPT_DIR}/android-env.sh"
configure_android_sdk
HELPER_ROOT=${PROJECT_ROOT}/android-helper
ADB_BIN=${ADB_BIN:-adb}
PACKAGE_NAME=com.galaxytty.helper

echo "Running hardware-independent Helper tests..."
"${HELPER_ROOT}/gradlew" --project-dir "$HELPER_ROOT" testDebugUnitTest lintDebug

if [ "${GALAXYTTY_REAL_DEVICE_TEST:-0}" != "1" ]; then
    echo "Skipping read-only device checks (set GALAXYTTY_REAL_DEVICE_TEST=1 to enable)."
    exit 0
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

if ! "$ADB_BIN" -s "$serial" shell pm path "$PACKAGE_NAME" | grep -q '^package:'; then
    echo "GalaxyTTY Helper is not installed on the selected device." >&2
    exit 1
fi

enabled_listeners=$("$ADB_BIN" -s "$serial" shell settings get secure enabled_notification_listeners | tr -d '\r')
case "$enabled_listeners" in
    *"${PACKAGE_NAME}"*) echo "Notification access: granted" ;;
    *) echo "Notification access: user approval required" ;;
esac

sms_permission=$(
    "$ADB_BIN" -s "$serial" shell dumpsys package "$PACKAGE_NAME" |
        awk '/android.permission.READ_SMS: granted=/ { print; exit }'
)
case "$sms_permission" in
    *"granted=true"*) echo "SMS history permission: granted" ;;
    *) echo "SMS history permission: user approval required" ;;
esac

if "$ADB_BIN" -s "$serial" shell dumpsys activity services "$PACKAGE_NAME" | grep -q 'BridgeForegroundService'; then
    echo "Background bridge: running"
else
    echo "Background bridge: user start required"
fi

echo "Read-only device checks passed. No notification action or message send was executed."
