#!/bin/sh
# Turns the frames of tour.py (screenshots-out/tour/) into tour.mp4 and tour.gif
# with ffmpeg on the Docker host; NAME=tour-apw for tour-apw.py:
#
#   SCRIPT=tour.py sh tools/screenshots/run.sh && sh tools/screenshots/video.sh
#   SCRIPT=tour-apw.py sh tools/screenshots/run.sh && NAME=tour-apw sh tools/screenshots/video.sh
#
# The GIF gets its own palette (palettegen) and only redraws what changed.
set -eu
cd "$(dirname "$0")/../.."
NAME=${NAME:-tour}
IN=screenshots-out/$NAME
OUT=screenshots-out
ffmpeg -loglevel error -y -f concat -safe 0 -i "$IN/concat.txt" \
  -vf "fps=25,format=yuv420p" -c:v libx264 -crf 20 -preset slow -movflags +faststart "$OUT/$NAME.mp4"
ffmpeg -loglevel error -y -f concat -safe 0 -i "$IN/concat.txt" \
  -vf "fps=10,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" \
  -loop 0 "$OUT/$NAME.gif"
ls -l "$OUT/$NAME.mp4" "$OUT/$NAME.gif"
