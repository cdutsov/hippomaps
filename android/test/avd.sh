#!/bin/sh
# Creates the test device (Pixel-like, 1080 x 2400, 420 dpi) and starts the emulator headless.
set -eu
AVD_HOME=/root/.android/avd
mkdir -p "$AVD_HOME/test.avd"
cat >"$AVD_HOME/test.ini" <<INI
avd.ini.encoding=UTF-8
path=$AVD_HOME/test.avd
target=android-34
INI
cat >"$AVD_HOME/test.avd/config.ini" <<INI
AvdId=test
abi.type=x86_64
hw.cpu.arch=x86_64
hw.cpu.ncore=2
hw.ramSize=3072
hw.lcd.width=1080
hw.lcd.height=2400
hw.lcd.density=420
hw.gps=yes
hw.keyboard=yes
hw.mainKeys=no
hw.gpu.enabled=yes
hw.gpu.mode=swiftshader_indirect
disk.dataPartition.size=4G
image.sysdir.1=system-images/android-34/google_apis/x86_64/
tag.id=google_apis
tag.display=Google APIs
skin.dynamic=yes
INI
exec emulator -avd test -no-window -no-audio -no-boot-anim -no-snapshot -gpu swiftshader_indirect \
	-accel on -memory 3072 -cores 2 -netdelay none -netspeed full
