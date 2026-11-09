#!/bin/sh
# Builds the signed release APK and AAB of the hippomaps app in Docker.
#   android/build.sh            lint, APK, AAB into android/dist/
#   COPY_TO=/path/app.apk android/build.sh   also copies the APK there
# The release key is in $SIGNING_DIR (default /root/.config/hippomaps-android), outside the
# repository; it is created on the first run. Passwords never leave that directory.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
SIGNING_DIR=${SIGNING_DIR:-/root/.config/hippomaps-android}
IMAGE=hippomaps-android-build:2
JDK_IMAGE=eclipse-temurin:17.0.13_11-jdk-jammy
CACHE=hippomaps-android-gradle   # Docker volume: Gradle caches and downloads
CPUS=${CPUS:-4}

umask 077
mkdir -p "$SIGNING_DIR"
chmod 700 "$SIGNING_DIR"

# the release key, once: PKCS12, RSA 4096, 30 years; store and key share one random password
if [ ! -f "$SIGNING_DIR/release.p12" ]; then
	echo "creating the release key in $SIGNING_DIR"
	pass=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 40)
	printf 'storeFile=/signing/release.p12\nstorePassword=%s\nkeyAlias=hippomaps\nkeyPassword=%s\n' "$pass" "$pass" >"$SIGNING_DIR/signing.properties"
	chmod 600 "$SIGNING_DIR/signing.properties"
	# the password in the environment, not on the command line (ps shows that to every user)
	KS_PASS=$pass docker run --rm -e KS_PASS -v "$SIGNING_DIR:/signing" "$JDK_IMAGE" \
		keytool -genkeypair -keystore /signing/release.p12 -storetype PKCS12 -storepass:env KS_PASS -keypass:env KS_PASS \
		-alias hippomaps -keyalg RSA -keysize 4096 -validity 10950 \
		-dname "CN=hippomaps, O=hippomaps.ch, C=CH" >/dev/null 2>&1
	unset pass
	chmod 600 "$SIGNING_DIR/release.p12"
fi

docker build -q -t "$IMAGE" "$here" >/dev/null

docker run --rm --cpus "$CPUS" --memory 8g \
	-v "$here:/src" -v "$CACHE:/root/.gradle" -v "$SIGNING_DIR:/signing:ro" \
	-e HIPPOMAPS_SIGNING=/signing/signing.properties \
	"$IMAGE" nice -n 10 gradle --no-daemon --console=plain -q testReleaseUnitTest lint assembleRelease bundleRelease

mkdir -p "$here/dist"
cp "$here/app/build/outputs/apk/release/app-release.apk" "$here/dist/hippomaps.apk"
cp "$here/app/build/outputs/bundle/release/app-release.aab" "$here/dist/hippomaps.aab"
cp "$here/app/build/reports/lint-results-debug.html" "$here/dist/lint.html" 2>/dev/null || true

# check the signature and write assetlinks.json from the certificate fingerprint
sha=$(docker run --rm -v "$here/dist:/dist:ro" "$IMAGE" \
	sh -c 'apksigner verify --print-certs /dist/hippomaps.apk' | sed -n 's/.*certificate SHA-256 digest: //p' | head -1)
[ -n "$sha" ] || { echo "no signature on the APK" >&2; exit 1; }
fp=$(echo "$sha" | tr 'a-f' 'A-F' | sed 's/../&:/g; s/:$//')
cat >"$here/assetlinks.json" <<JSON
[
  {
    "relation": ["delegate_permission/common.handle_all_urls"],
    "target": {
      "namespace": "android_app",
      "package_name": "ch.hippomaps.app",
      "sha256_cert_fingerprints": ["$fp"]
    }
  }
]
JSON
chmod 644 "$here/assetlinks.json" "$here/dist/hippomaps.apk" "$here/dist/hippomaps.aab"

if [ -n "${COPY_TO:-}" ]; then cp "$here/dist/hippomaps.apk" "$COPY_TO"; fi
ls -l "$here/dist"
echo "certificate SHA-256: $fp"
