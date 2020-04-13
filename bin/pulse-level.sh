#!/usr/bin/env bash

parec \
  --raw \
  --device=3 \
  --channels=1 \
  --latency=2 \
  2>/dev/null \
    | od -N2 -td2 \
    | head -n1 \
    | cut -d' ' -f2- \
    | tr -d ' '
