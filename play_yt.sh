#!/bin/bash
youtube-dl -o "/tmp/youtube_play.%(ext)s" --extract-audio --audio-format mp3 $1
play /tmp/youtube_play.mp3
