#!/bin/bash
youtube-dl -o "/tmp/%(id)s.%(ext)s" --extract-audio --audio-format mp3 $1
play /tmp/$1.mp3
rm /tmp/$1.mp3
