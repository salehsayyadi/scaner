#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${VERSION:-$(tr -d '\r\n' < VERSION)}"
VERSION_CODE="${VERSION_CODE:-$(python -c 'a,b,c=map(int,open("VERSION").read().strip().split(".")); print(a*1000000+b*1000+c)')}"
export PATH="$(go env GOPATH)/bin:$PATH"
mkdir -p build dist
python tools/make_icon.py

# Fyne's command-line flag parser splits GOFLAGS on whitespace. Keep each linker
# argument in a separate -ldflags= token, rather than using shell-style quotes.
export GOFLAGS="-ldflags=-checklinkname=0 -ldflags=-w -ldflags=-X=main.Version=${VERSION} -ldflags=-extldflags=-Wl,-z,max-page-size=16384"

# A custom manifest avoids unnecessary shared-storage permissions and backups of
# connection credentials. Fyne supplies min/target SDK and launcher icon itself.
python - "$VERSION" "$VERSION_CODE" <<'PY'
import sys
from pathlib import Path
version, code = sys.argv[1:]
Path("cmd/scaner/AndroidManifest.xml").write_text(f'''<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.piyazche.scaner" android:versionCode="{code}" android:versionName="{version}">
  <application android:label="Scaner" android:debuggable="false" android:allowBackup="false">
    <activity android:name="org.golang.app.GoNativeActivity" android:label="Scaner"
      android:configChanges="orientation|screenSize|smallestScreenSize|screenLayout|keyboardHidden|uiMode"
      android:exported="true" android:theme="@android:style/Theme" android:windowSoftInputMode="adjustResize">
      <meta-data android:name="android.app.lib_name" android:value="scaner" />
      <intent-filter>
        <action android:name="android.intent.action.MAIN" />
        <category android:name="android.intent.category.LAUNCHER" />
      </intent-filter>
    </activity>
  </application>
  <uses-permission android:name="android.permission.INTERNET" />
</manifest>''')
PY
trap 'rm -f cmd/scaner/AndroidManifest.xml' EXIT
# fyne resolves a relative --icon against --src, so pass an absolute path.
fyne package --os android/arm64,android/arm --src cmd/scaner \
  --appID com.piyazche.scaner --name Scaner --icon "$PWD/build/icon.png" \
  --appVersion "$VERSION" --appBuild "$VERSION_CODE"
APK="$(find . -name 'Scaner.apk' -o -name 'scaner.apk' | head -1)"
test -n "$APK"
TOOLS="${ANDROID_SDK_ROOT:-$ANDROID_HOME}/build-tools/35.0.0"
"$TOOLS/zipalign" -P 16 -f 4 "$APK" build/Scaner-aligned.apk
unset GOFLAGS
OUT="dist/Scaner-${VERSION}-android.apk"
if [[ -n "${ANDROID_KEYSTORE_BASE64:-}" ]]; then
  : "${ANDROID_KEYSTORE_PASSWORD:?Set ANDROID_KEYSTORE_PASSWORD}"
  : "${ANDROID_KEY_ALIAS:?Set ANDROID_KEY_ALIAS}"
  : "${ANDROID_KEY_PASSWORD:?Set ANDROID_KEY_PASSWORD}"
  KEY="$(mktemp)"
  printf '%s' "$ANDROID_KEYSTORE_BASE64" | base64 --decode > "$KEY"
  chmod 600 "$KEY"
  "$TOOLS/apksigner" sign --ks "$KEY" --ks-key-alias "$ANDROID_KEY_ALIAS" \
    --ks-pass env:ANDROID_KEYSTORE_PASSWORD --key-pass env:ANDROID_KEY_PASSWORD \
    --out "$OUT" build/Scaner-aligned.apk
  rm -f "$KEY"
  echo "Android signing: private release key" > dist/ANDROID-SIGNING.txt
else
  # Re-sign with the deterministic, public gomobile test key after zipalign.
  # This produces an installable personal APK and permits upgrades between builds.
  python tools/extract_debug_key.py build/debug-signing.pem
  openssl pkcs8 -topk8 -nocrypt -in build/debug-signing.pem -outform DER -out build/debug-key.pk8
  # Both the key AND the certificate must stay identical across APK upgrades.
  "$TOOLS/apksigner" sign --key build/debug-key.pk8 --cert packaging/android/debug-cert.pem \
    --out "$OUT" build/Scaner-aligned.apk
  echo "Android signing: PUBLIC TEST KEY; personal/testing APK, not a production identity" > dist/ANDROID-SIGNING.txt
fi
"$TOOLS/apksigner" verify --verbose "$OUT"
"$TOOLS/zipalign" -c -P 16 4 "$OUT"
"$TOOLS/aapt" dump badging "$OUT" | grep -E 'package:|sdkVersion:|targetSdkVersion:|native-code:'
