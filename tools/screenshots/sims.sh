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
  # One device has a long script, for the script editor (screenshots, check-editor.py).
  # It also says a newer stable firmware exists (the orange ring and ⚡, check-firmware-ui.py).
  if [ "$dir" = gen3/MiniPMG3 ]; then
    cp tools/screenshots/script/*.json "/tmp/fx/$i/"
    # Its access point is on and open, as from the factory (the AP wizard, tour-apw.py).
    echo '{"ap":{"ssid":"ShellyMiniPMG3-AABBCC000001","is_open":true,"enable":true},"sta":{"ssid":"Home","enable":true,"is_open":false,"ipv4mode":"dhcp","ip":null,"netmask":null,"gw":null,"nameserver":null},"sta1":{"ssid":null,"enable":false,"is_open":true,"ipv4mode":"dhcp","ip":null,"netmask":null,"gw":null,"nameserver":null},"roam":{"rssi_thr":-80,"interval":60}}' > "/tmp/fx/$i/rpc_WiFi.GetConfig.json"
    sed -i 's/"available_updates": {}/"available_updates": {"stable": {"version": "2.0.3"}}/'       "/tmp/fx/$i/rpc_Shelly.GetStatus.json" "/tmp/fx/$i/rpc_Sys.GetStatus.json"
  fi
  # And one its Wi-Fi settings (the device Settings dialog in tour.py).
  if [ "$dir" = gen1/SHPLG-S ]; then
    echo '{"enabled":true,"ssid":"Home","ipv4_method":"dhcp","ip":null,"gw":null,"mask":null,"dns":null}' > "/tmp/fx/$i/settings_sta.json"
    echo '{"enabled":false,"ssid":null,"ipv4_method":"dhcp","ip":null,"gw":null,"mask":null,"dns":null}' > "/tmp/fx/$i/settings_sta1.json"
  fi
  # And one a live log (the Logs window).
  if [ "$dir" = gen2/Plus1 ]; then cp tools/screenshots/log/_log.jsonl "/tmp/fx/$i/"; fi
  # And one is a gateway that relays a BLU device and answers Identify (_discovery.json).
  if [ "$dir" = gen3/DimmerG3 ]; then
    cp tools/screenshots/blu/*.json "/tmp/fx/$i/"
    sed -i "s/__NOW__/$(date +%s)/g" "/tmp/fx/$i/rpc_BLE.CloudRelay.ListInfos.json"
  fi
  sed -i -e "s/AABBCC000001/AABBCC0000$n/g" -e "s/aabbcc000001/aabbcc0000$nl/g" \
    -e "s/AA:BB:CC:00:00:01/AA:BB:CC:00:00:$n/g" -e "s/\"Test\"/\"$name\"/g" "/tmp/fx/$i/"*
  /tmp/sim -vary -addr "127.0.0.$i" -port 80 "/tmp/fx/$i" &
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
