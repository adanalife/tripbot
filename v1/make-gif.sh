#!/usr/bin/env bash

NUM=$1

cd /Volumes/usbshare1/maps
convert -delay 0 -loop 1 *.png ../animation${NUM}-big.gif

gifsicle -i ../animation${NUM}-big.gif -O3 --colors 256 -o ../animation${NUM}.gif
rm ../animation${NUM}-big.gif
