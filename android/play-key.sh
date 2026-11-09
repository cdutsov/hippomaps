#!/bin/sh
# Encrypts the release key for Play App Signing (android/PLAY.md, "Signing"), in Docker:
#   android/play-key.sh <dir>     <dir> holds pepk.jar and encryption_public_key.pem from Play Console
# Writes <dir>/encrypted_private_key.zip. pepk asks for the passwords of the keystore and of the key
# (both the storePassword of signing.properties); they are typed, never on a command line.
set -eu
dir=$(cd "${1:?usage: android/play-key.sh <dir with pepk.jar and encryption_public_key.pem>}" && pwd)
SIGNING_DIR=${SIGNING_DIR:-/root/.config/hippomaps-android}
JDK_IMAGE=eclipse-temurin:17.0.13_11-jdk-jammy
for f in pepk.jar encryption_public_key.pem; do
	[ -f "$dir/$f" ] || { echo "missing $dir/$f" >&2; exit 1; }
done
docker run --rm -it -v "$SIGNING_DIR:/signing:ro" -v "$dir:/w" "$JDK_IMAGE" \
	java -jar /w/pepk.jar --keystore=/signing/release.p12 --alias=hippomaps \
	--output=/w/encrypted_private_key.zip --include-cert --rsa-aes-encryption \
	--encryption-key-path=/w/encryption_public_key.pem
chmod 600 "$dir/encrypted_private_key.zip"
echo "upload $dir/encrypted_private_key.zip in Play Console"
