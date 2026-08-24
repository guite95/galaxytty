plugins {
    id("com.android.application")
}

android {
    namespace = "com.galaxytty.helper"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.galaxytty.helper"
        minSdk = 26
        targetSdk = 36
        versionCode = 15
        versionName = "0.13.0-poc"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    testImplementation("junit:junit:4.13.2")
}
