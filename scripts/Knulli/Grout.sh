#!/bin/bash
APP_DIR="$(dirname "$0")"
FLAG_FILE="./knulli_restart_request"
cd "$APP_DIR" || exit 1

export CFW=KNULLI
export LD_LIBRARY_PATH=$APP_DIR/lib

./grout

if [ -f "$FLAG_FILE" ]; then
    rm -f "$FLAG_FILE"
    # batocera-es-swissknife --update-gamelists
    batocera-es-swissknife --restart
fi

exit 0