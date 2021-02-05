#!/usr/bin/python3 -u

# The -u flag in the shebang forces stdout/stderr to be unbuffered to ensure Python doesn't buffer printed messages when
# stdout isn't attached to a terminal (e.g. in a container). Ref. https://stackoverflow.com/a/56934591

# NOTE(antoineco) the shebang is purposely set to the path of the python3 interpreter instead of "/usr/bin/env python3"
# because distroless images don't include the "env" executable.

# Source: https://github.com/tsenart/vegeta/tree/master/scripts/load-ramping
#
# Original script modified to
#  - accept a stream of vegeta targets (-lazy mode) instead of a single one
#  - execute subprocesses directly instead of through a shell
#  - omit automated execution of gnuplot commands

import json
import os
import subprocess
import sys
import time


if '-h' in sys.argv or '--help' in sys.argv:
    print('Usage:', file=sys.stderr)
    print('  <JSON TARGETS GENERATOR> | %s' % sys.argv[0], file=sys.stderr)
    sys.exit(1)

# Log-spaced rates (each ca. +25% (+1dB) of the previous, covering 1/sec to 100k/sec)
rates = [10.0 ** (i / 10.0) for i in range(50)]
rates = [501.18723362727246, 630.957344480193, 794.3282347242813, 1000.0, 1258.9254117941675, 1584.893192461114, 1995.2623149688789, 2511.88643150958, 3162.2776601683795, 3981.0717055349733, 5011.872336272725, 6309.57344480193, 7943.282347242814]

# Log-spaced buckets (each ca. +25% (+1dB) of the previous, covering <1us to >10s)
buckets = [0] + [1e3 * 10.0 ** (i / 10.0) for i in range(71)]


with os.fdopen(sys.stdin.fileno(), 'rb', buffering=0) as unbuffered_stdin:

    # Run vegeta attack
    for rate in rates:
        filename='results_%i.bin' % (1000*rate)
        if not os.path.exists(filename):
            cmd = ['vegeta', 'attack', '-duration=5s', '-format=json', '-lazy', '-rate=%i/1000s' % (1000*rate),
                   '-output=%s' % filename]
            print(' '.join(cmd), file=sys.stderr)
            subprocess.run(cmd)
            time.sleep(5)
            # Read potentially truncated input until the next '\n' byte to reposition
            # sys.stdin to a location that is safe for the next subprocess to consume.
            unbuffered_stdin.readline()


# Run vegeta report, and extract data for gnuplot
with open('results_latency.txt', 'w') as out_latency, \
     open('results_success.txt', 'w') as out_success:

    for rate in rates:
        cmd = ['vegeta', 'report', '-type=json',
               "-buckets=[%s]" % ','.join('%ins' % bucket for bucket in buckets), 
               'results_%i.bin' % (1000*rate)]
        print(' '.join(cmd), file=sys.stderr)
        result = json.loads(subprocess.check_output(cmd))

        # (Request rate, Response latency) -> (Fraction of responses)
        for latency, count in result['buckets'].items():
            latency_nsec = float(latency)
            fraction = count / sum(result['buckets'].values()) * result['success']
            print(rate, latency_nsec, fraction, file=out_latency)
        print(file=out_latency)

        # (Request rate) -> (Success rate)
        print(rate, result['success'], file=out_success)

print('# wrote results_latency.txt and results_success.txt', file=sys.stderr)
