#!/bin/sh
# Container entrypoint for the beevibe renderer.
#
# chromedp/headless-shell ships /headless-shell/run.sh, which launches the bundled
# headless-shell browser bound to loopback on 9223 and bridges it to 0.0.0.0:9222
# with socat. Passing --remote-debugging-address=0.0.0.0 to the browser directly is
# not honoured by this build, so reuse the image's own launch script and point the
# renderer at the socat listener.
set -eu

/headless-shell/run.sh &

exec /usr/local/bin/beevibe-renderer
