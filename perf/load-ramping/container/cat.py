#!/usr/bin/python3

# A cat(1) clone that can run in the distroless:python3 image.

import sys

with open(sys.argv[1], 'r') as f:
    print(f.read())
