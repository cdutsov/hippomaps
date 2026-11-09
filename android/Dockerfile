# Build image for the hippomaps Android app: JDK 17, Android command line tools, SDK platform 36,
# build tools 36.0.0. Every download is pinned by version and checksum.
FROM eclipse-temurin:17.0.13_11-jdk-jammy

ARG CMDLINE_TOOLS=11076708
ARG CMDLINE_TOOLS_SHA256=2d2d50857e4eb553af5a6dc3ad507a17adf43d115264b1afc116f95c92e5e258
# the Android Gradle plugin wants platform tools; fetched by archive name so it is not "latest"
ARG PLATFORM_TOOLS_ZIP=platform-tools_r37.0.1-linux.zip
ARG PLATFORM_TOOLS_SHA1=477254aa5f903c15cf51001717bdf347fb6b53e0
ARG GRADLE_VERSION=8.13
ARG GRADLE_SHA256=20f1b1176237254a6fc204d8434196fa11a4cfb387567519c61556e8710aed78

ENV ANDROID_HOME=/opt/android-sdk \
    ANDROID_SDK_ROOT=/opt/android-sdk \
    GRADLE_HOME=/opt/gradle
ENV PATH=$PATH:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/build-tools/36.0.0:$GRADLE_HOME/bin

RUN apt-get update \
 && apt-get install -y --no-install-recommends unzip \
 && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL -o /tmp/clt.zip "https://dl.google.com/android/repository/commandlinetools-linux-${CMDLINE_TOOLS}_latest.zip" \
 && echo "${CMDLINE_TOOLS_SHA256}  /tmp/clt.zip" | sha256sum -c - \
 && mkdir -p "$ANDROID_HOME/cmdline-tools" \
 && unzip -q /tmp/clt.zip -d "$ANDROID_HOME/cmdline-tools" \
 && mv "$ANDROID_HOME/cmdline-tools/cmdline-tools" "$ANDROID_HOME/cmdline-tools/latest" \
 && rm /tmp/clt.zip

# platforms;android-35 is revision 2, build-tools 35.0.0
RUN yes | sdkmanager --licenses >/dev/null \
 && sdkmanager --install "platforms;android-36" "build-tools;36.0.0" >/dev/null \
 && grep -h "^Pkg.Revision" "$ANDROID_HOME/platforms/android-36/source.properties" "$ANDROID_HOME/build-tools/36.0.0/source.properties"

RUN curl -fsSL -o /tmp/pt.zip "https://dl.google.com/android/repository/${PLATFORM_TOOLS_ZIP}" \
 && echo "${PLATFORM_TOOLS_SHA1}  /tmp/pt.zip" | sha1sum -c - \
 && unzip -q /tmp/pt.zip -d "$ANDROID_HOME" \
 && rm /tmp/pt.zip

RUN curl -fsSL -o /tmp/gradle.zip "https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip" \
 && echo "${GRADLE_SHA256}  /tmp/gradle.zip" | sha256sum -c - \
 && unzip -q /tmp/gradle.zip -d /opt \
 && mv "/opt/gradle-${GRADLE_VERSION}" "$GRADLE_HOME" \
 && rm /tmp/gradle.zip

WORKDIR /src
