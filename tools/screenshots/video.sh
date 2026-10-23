#!/bin/sh
# Turns the frames of tour.py (screenshots-out/tour/) into tour.mp4 and tour.gif
# with ffmpeg on the Docker host:
#
#   SCRIPT=tour.py sh tools/screenshots/run.sh && sh tools/screenshots/video.sh
#
# The GIF gets its own palette (palettegen) and only redraws what changed.
set -eu
cd "$(dirname "$0")/../.."
IN=screenshots-out/tour
OUT=screenshots-out
ffmpeg -loglevel error -y -f concat -safe 0 -i "$IN/concat.txt" \
  -vf "fps=25,format=yuv420p" -c:v libx264 -crf 20 -preset slow -movflags +faststart "$OUT/tour.mp4"
ffmpeg -loglevel error -y -f concat -safe 0 -i "$IN/concat.txt" \
  -vf "fps=10,scale=960:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" \
  -loop 0 "$OUT/tour.gif"
ls -l "$OUT/tour.mp4" "$OUT/tour.gif"
