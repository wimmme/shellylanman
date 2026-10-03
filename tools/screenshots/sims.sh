#!/bin/sh
# Simulated devices for screenshots and manual tests, run inside a golang
# container that shares the network of a ShellyLanMan container (see run.sh).
# Every fixture gets its own loopback address (127.0.0.2, .3, …) on port 80,
# its own MAC (the fixtures all carry AABBCC000001) and a readable name.
set -eu
go build -o /tmp/sim ./cmd/shellysim
i=2
while read -r dir name; do
  [ -n "$dir" ] || continue
  n=$(printf %02X "$i")
  nl=$(echo "$n" | tr A-F a-f)
  mkdir -p "/tmp/fx/$i"
  cp -r "testdata/$dir/." "/tmp/fx/$i/"
  sed -i -e "s/AABBCC000001/AABBCC0000$n/g" -e "s/aabbcc000001/aabbcc0000$nl/g" \
    -e "s/AA:BB:CC:00:00:01/AA:BB:CC:00:00:$n/g" -e "s/\"Test\"/\"$name\"/g" "/tmp/fx/$i/"*
  /tmp/sim -addr "127.0.0.$i" -port 80 "/tmp/fx/$i" &
  i=$((i + 1))
done <<'EOF'
gen1/SHPLG-S Garden pump
gen1/SHSW-1 Shed light
gen1/SHIX3-1 Hall buttons
gen1/SHRGBW2 Desk strip
gen2/Plus1 Porch light
gen2/PlusRGBWPM Kitchen strip
gen2/Pro3EM Car charger
gen2/ProRGBWWPM Garage strip
gen3/DimmerG3 Living room
gen3/I4G3 Kitchen switch
gen3/MiniPMG3 Heat pump
EOF
wait
