# Headless Android emulator for testing the app: API 34 Google APIs x86_64, KVM.
# Emulator and system image are fetched by exact archive name and SHA-1 (the
# checksums of Google's repository manifest).
FROM hippomaps-android-build:1

ARG EMULATOR_ZIP=emulator-linux_x64-16740729.zip
ARG EMULATOR_SHA1=f41f9e31eb5ca64926603c03265c50bbc0ff4172
ARG SYSIMG_ZIP=x86_64-34_r14.zip
ARG SYSIMG_SHA1=e0f6c9a0691aa27bd597d0deb1bcfdc943ac8ca7

RUN apt-get update && apt-get install -y --no-install-recommends \
      libpulse0 libgl1 libnss3 libxcomposite1 libxcursor1 libxi6 libxtst6 libxdamage1 libxrandr2 \
      libasound2 libdbus-1-3 libfontconfig1 libxkbfile1 libxkbcommon0 libx11-xcb1 libbsd0 libtinfo5 \
 && rm -rf /var/lib/apt/lists/*

RUN set -e; cd /tmp; \
    fetch() { curl -fsSL -o "$1" "$2/$1" && echo "$3  $1" | sha1sum -c -; }; \
    fetch "$EMULATOR_ZIP" https://dl.google.com/android/repository "$EMULATOR_SHA1"; \
    fetch "$SYSIMG_ZIP" https://dl.google.com/android/repository/sys-img/google_apis "$SYSIMG_SHA1"; \
    unzip -q "$EMULATOR_ZIP" -d "$ANDROID_HOME"; \
    mkdir -p "$ANDROID_HOME/system-images/android-34/google_apis"; \
    unzip -q "$SYSIMG_ZIP" -d "$ANDROID_HOME/system-images/android-34/google_apis"; \
    rm -f /tmp/*.zip

ENV PATH=$PATH:$ANDROID_HOME/emulator:$ANDROID_HOME/platform-tools
# python3 for test/cdp.py (reads the page of a debug build over the DevTools protocol)
RUN apt-get update && apt-get install -y --no-install-recommends python3 && rm -rf /var/lib/apt/lists/*
COPY cdp.py /usr/local/bin/cdp.py
COPY avd.sh /usr/local/bin/avd.sh
CMD ["/usr/local/bin/avd.sh"]
