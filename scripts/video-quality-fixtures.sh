#!/bin/sh
# Linux reference fixtures only; never reads personal media or application data.
set -eu

fixture_dir=${1:?usage: scripts/video-quality-fixtures.sh /tmp/new-fixture-directory}
case "$fixture_dir" in /tmp/*) ;; *) echo 'Use a new directory under /tmp.' >&2; exit 1 ;; esac
mkdir "$fixture_dir"

# NPS Video: Michael Quinn. The linked NPS page explicitly identifies this
# unmarked NPS work as public domain. This benchmark implies no NPS endorsement.
# https://www.nps.gov/media/video/view.htm?id=39E50741-155D-451F-67CACF2694A2F3E7
curl --fail --location --retry 2 --max-time 120 --max-filesize 134217728 \
  https://www.nps.gov/nps-audiovideo/audiovideo/e27f505a-638a-4adf-b9fd-53735f8a34a81080p.mp4 \
  --output "$fixture_dir/nps-native24.mp4"
printf '%s  %s\n' \
  0caaa644b7662938d0ed0cdfc32621962fa824c955ae01f3aa098c880c1ecac1 \
  "$fixture_dir/nps-native24.mp4" | sha256sum --check

set -- -an -sn -dn -map_metadata -1 -map_chapters -1 \
  -c:v libx264 -threads 1 -preset veryfast -crf 18 -maxrate 20M -bufsize 40M \
  -g 120 -pix_fmt yuv420p -color_primaries bt709 -color_trc bt709 \
  -colorspace bt709 -color_range tv -movflags +faststart
ffmpeg -v error -f lavfi -i testsrc2=size=1920x1080:rate=60:duration=30 \
  "$@" "$fixture_dir/motion60.mp4"
# This controlled 60 FPS fixture repeats native 24000/1001 FPS frames. It does
# not interpolate motion. Production preparation never raises source FPS.
ffmpeg -v error -i "$fixture_dir/nps-native24.mp4" -t 30 -vf fps=60 \
  "$@" "$fixture_dir/ambient60.mp4"
for fixture in nps-native24 motion60 ambient60; do
  ffprobe -v error -select_streams v:0 \
    -show_entries stream=codec_name,width,height,avg_frame_rate,nb_frames,duration:format=size \
    -of json "$fixture_dir/$fixture.mp4" > "$fixture_dir/$fixture.json"
done
sha256sum "$fixture_dir/"*.mp4 > "$fixture_dir/SHA256SUMS"
