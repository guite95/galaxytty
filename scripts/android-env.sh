#!/bin/sh

configure_android_sdk() {
    if [ -n "${GALAXYTTY_ANDROID_SDK_ROOT:-}" ]; then
        sdk_root=${GALAXYTTY_ANDROID_SDK_ROOT}
    elif [ -n "${ANDROID_HOME:-}" ]; then
        sdk_root=${ANDROID_HOME}
    elif [ -n "${ANDROID_SDK_ROOT:-}" ]; then
        sdk_root=${ANDROID_SDK_ROOT}
    else
        user_home=${HOME:-}
        sdk_root=
        for candidate in \
            "${user_home}/Library/Android/sdk" \
            /opt/homebrew/share/android-commandlinetools \
            /usr/local/share/android-commandlinetools
        do
            if [ -d "${candidate}/platforms/android-36" ]; then
                sdk_root=${candidate}
                break
            fi
        done
    fi

    if [ -z "${sdk_root}" ] || [ ! -d "${sdk_root}/platforms/android-36" ]; then
        echo "Android SDK Platform 36 was not found." >&2
        echo "Set GALAXYTTY_ANDROID_SDK_ROOT or ANDROID_HOME to the SDK root." >&2
        return 1
    fi

    ANDROID_HOME=${sdk_root}
    ANDROID_SDK_ROOT=${sdk_root}
    export ANDROID_HOME ANDROID_SDK_ROOT
}
