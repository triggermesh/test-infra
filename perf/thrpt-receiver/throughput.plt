set datafile separator comma

unset xtics
set ylabel "Events per second"
set grid ytics lc rgb '#888888' lw 0.1

plot "results.csv" skip 500 u 1:3 with line lw 1 lc rgb "blue" title "Receiver throughput"
