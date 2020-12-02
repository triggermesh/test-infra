set xlabel "Relative time in seconds"
set ylabel "Events per second"

set grid
set xtics 10  # every 10s
set mxtics 5  # every 2s

set datafile separator comma

# Relative time calculation: https://stackoverflow.com/a/65112899
plot t=0 "results.csv" u ((t==0 ? (t0=$1, t=1, $1-t0) : NaN), ($1-t0)/1000):3 with line lc rgb "#4687f4" title "Receiver throughput"
