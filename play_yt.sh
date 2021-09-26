#!/bin/bash
set -u
NONCE=$RANDOM
youtube-dl -o "/tmp/%(id)s_$NONCE.%(ext)s" --extract-audio --audio-format mp3 "$1"
play "/tmp/$1_$NONCE.mp3"
rm "/tmp/$1_$NONCE.mp3"
