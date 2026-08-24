#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_ROOT=$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)
. "${SCRIPT_DIR}/android-env.sh"
configure_android_sdk
HELPER_ROOT=${PROJECT_ROOT}/android-helper
APK_PATH=${HELPER_ROOT}/app/build/outputs/apk/debug/app-debug.apk
ADB_BIN=${ADB_BIN:-adb}
PACKAGE_NAME=com.galaxytty.helper
ACTIVITY_NAME=${PACKAGE_NAME}/.MainActivity

resolve_device() {
    requested=${GALAXYTTY_ADB_SERIAL:-}
    if [ -n "$requested" ]; then
        if ! "$ADB_BIN" -s "$requested" get-state 2>/dev/null | grep -qx device; then
            echo "Configured Galaxy is not connected: $requested" >&2
            exit 1
        fi
        printf '%s\n' "$requested"
        return
    fi

    connected=$(
        "$ADB_BIN" devices | awk -F '\t' 'NR > 1 && $2 == "device" { print $1 }'
    )
    count=$(printf '%s\n' "$connected" | awk 'NF { count++ } END { print count+0 }')
    if [ "$count" -ne 1 ]; then
        echo "Expected exactly one authorized ADB device; found $count." >&2
        echo "Set GALAXYTTY_ADB_SERIAL to an exact target when multiple devices are connected." >&2
        "$ADB_BIN" devices -l >&2
        exit 1
    fi
    printf '%s\n' "$connected"
}

if [ ! -x "${HELPER_ROOT}/gradlew" ]; then
    echo "Missing Android Gradle wrapper: ${HELPER_ROOT}/gradlew" >&2
    exit 1
fi

serial=$(resolve_device)
echo "Building GalaxyTTY Helper..."
"${HELPER_ROOT}/gradlew" --project-dir "$HELPER_ROOT" :app:assembleDebug

if [ ! -f "$APK_PATH" ]; then
    echo "Debug APK was not produced at $APK_PATH" >&2
    exit 1
fi

echo "Updating Helper on the connected Galaxy..."
"$ADB_BIN" -s "$serial" install -r "$APK_PATH"
"$ADB_BIN" -s "$serial" shell am start -n "$ACTIVITY_NAME" >/dev/null

echo "Helper updated without uninstalling app data."
echo "If notification access is not granted, approve it from the Helper screen."
echo "Tap 'Start background bridge' and approve bridge notifications when requested."
