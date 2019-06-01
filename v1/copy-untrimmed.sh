#!/usr/bin/env bash

mkdir n

# https://stackoverflow.com/a/6282057
# this moves all of the "befores"
for file in $(ls _all | grep -B1 _b | awk -F '\n' 'ln ~ /^$/ { ln = "matched"; print $1 } $1 ~ /^--$/ { ln = "" }' | grep -v _a | grep -v _c); do
echo "mv _all/$file n/"
done

#ls _all/ | grep -A1 _a | grep -v '\-\-' | grep -v '_a' | grep -v _b
