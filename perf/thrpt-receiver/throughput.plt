set xlabel "Timestamp"
set ylabel "Events per second"

set grid
set xtics 10  # every 10s
set mxtics 5  # every 2s

set xdata time
set format x "%H:%M:%S"

set datafile separator comma

plot "results.csv" using ($1/1000):3 with line lc rgb "#4687f4" title "Receiver throughput"
