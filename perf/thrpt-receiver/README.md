# thrpt-receiver

A simple CloudEvent receiver that can measure the throughput of the events it receives and send results to a [Mako stub
sidecar][mako-stub].

```none
Usage of thrpt-receiver:
  -consecutive-quiet-periods uint
        Consecutive recheck-period after which data is aggregated if no new event has been recorded. (default 2)
  -estimated-total-events uint
        Estimated total number of events to receive. Used to pre-allocate memory. (default 100)
  -profiling
        Periodically publish the length of the receive queue to Mako and enable a pprof server on port 8008.
  -recheck-period duration
        Frequency at which the recording of new events is being checked. (default 5s)
```

---

## Contents

1. [Running the receiver in a cluster](#running-the-receiver-in-a-cluster)
   * [Deployment](#deployment)
   * [Sending events](#sending-events)
   * [Reading results](#reading-results)
   * [Clean up](#clean-up)
1. [Plotting](#plotting)
   * [Google Sheets](#google-sheets)
   * [gnuplot](#gnuplot)
1. [Profiling](#profiling)
   * [Throughput](#throughput)
   * [Latency](#latency)

## Running the receiver in a cluster

### Deployment

Deploy the receiver and its dependencies into the `perf-thrpt-receiver` namespace using `ko`:

```console
$ ko apply -f config/
```

The receiver will be waiting for the first event to be received:

```console
$ kubectl -n perf-thrpt-receiver logs thrpt-receiver receiver
2020/10/30 16:17:03 Running event recorder
2020/10/30 16:17:03 Running CloudEvents handler
2020/10/30 16:17:03 Waiting for the first event to be received
```

### Sending events

Configure an event source to send events to the `perf-thrpt-receiver/thrpt-receiver` Service, for example using a sink
reference:

```yaml
sink:
  ref:
    apiVersion: v1
    kind: Service
    name: thrpt-receiver
    namespace: perf-thrpt-receiver
```

After the first event has been received, the receiver keeps processing events and checks every `-recheck-period` that
events are still being received. If no event is received for a number of periods corresponding to
`-consecutive-quiet-periods`, the receiver stops its event handler and publishes the benchmark's results to its Mako
sidecar:

```console
$ kubectl -n perf-thrpt-receiver logs thrpt-receiver receiver
...
2020/10/30 16:28:05 Event received, waiting until no more event is being recorded for 2 consecutive periods of 5s
2020/10/30 16:30:30 Observed 1 period(s) without event
2020/10/30 16:30:35 Observed 2 period(s) without event
2020/10/30 16:30:35 Stopping event recorder and CloudEvents handler
2020/10/30 16:30:35 Received events count: 1000
2020/10/30 16:30:35 Processing data
2020/10/30 16:30:35 Publishing results to Mako
```

### Reading results

The results, presented in a CSV format, can be exported from a HTTP endpoint served by the Mako sidecar on port `8081`.

Forward the local TCP port `8081` to the `thrpt-receiver` Pod:

```console
$ kubectl -n perf-thrpt-receiver port-forward thrpt-receiver 8081
Forwarding from 127.0.0.1:8081 -> 8081
```

Retrieve the results over HTTP at the `/results` endpoint and write the output to a file named `results.csv`:

```console
$ curl http://localhost:8081/results -o results.csv
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
100 6771k    0 6771k    0     0  5976k      0 --:--:--  0:00:01 --:--:-- 5976k
```

The contents of the `results.csv` file should be similar to the example below:

```csv
# Received input
# Input completed
# Benchmark  - Event throughput
# {"benchmarkKey":"","tags":["nodes=6","project-id=cebuk-01","zone=us-central1-a","commit=f584d79","kubernetes=v1.17.12-gke.500","goversion=go1.15.2"]}
# inputValue,errorMessage,rt
...
1.605030612296653e+12,,746
1.605030612298227e+12,,747
1.6050306122983225e+12,,748
1.6050306122986868e+12,,749
1.6050306122990955e+12,,750
1.605030612299801e+12,,751
1.605030612300778e+12,,752
1.60503061230106e+12,,753
...
# CSV end
```

See the [Plotting](#plotting) section below for suggestions about exploiting those results.

Send a final HTTP request to the `/close` endpoint to allow the Mako sidecar to terminate:

```console
$ curl -s http://localhost:8081/close
```

### Clean up

By default, the receiver Pod is requesting the resources of an entire cluster node, which makes it expensive to run. It
is therefore a good practice to clean up the receiver's resources at the end of the benchmark:

```console
$ ko delete -f config/
```

## Plotting

The results published by `thrpt-receiver` can be visualized by generating plots from CSV data. A few different ways to
create such visualizations are described in this section.

### Google Sheets

1. Open a Sheet, then select **File > Import > Upload** to upload the CSV file generated from the logs of the Mako
   sidecar.

   ![Import CSV](.assets/plot-gsheets-import-csv.png)

1. _(optional)_ Delete the unnecessary comment rows 1-3 and B column (`errorMessage`) by highlighting them, then
   selecting **Edit > Delete rows/column ...**.

1. _(optional)_ Create a column containing human-friendly timestamps.

   Highlight the A column (`inputValue`), then select **Insert > Column right**. Select the B2 cell, and use the
   following [formula][gsheets-ts-formula] in the formula bar: `=A2/1000/60/60/24 + DATE(1970,1,1)`.

   Format the cell as a Time by selecting **Format > Number > Time**, then apply that format and formula to the entire
   column, either with `Ctrl-C` (highlight all cells with `Ctrl-Shift-↓`) `Ctrl-V`, or by using the [Fill
   Handle][gsheets-fill].

   ![Format time cell](.assets/plot-gsheets-format-datetime.png)

1. Highlight the entire time:value data range, then select **Insert > Chart**.

1. Edit the chart's options and select the _Line chart_ type. You may also need to manually set the `time` column as the
   X-axis.

   ![Chart values](.assets/plot-gsheets-line-chart.png)

1. You can improve the visualization of the chart by moving it to its own sheet. This can be achieved by selecting
   **Move to own sheet** in the menu accessible in the top-right corner of the chart.

   ![Chart](.assets/plot-gsheets-full-chart.png)

### gnuplot

The provided `throughput.plt` file can be used to plot Mako's CSV results. Those results must be saved to a file called
`results.csv`.

To generate a graph exported to a PNG file, execute the following command:

```
$ gnuplot -e "set term png size 1920, 1080" throughput.plt > results.png
```

See the next section for an example of rendered graph.

## Profiling

The figures presented in this section describe the profile of a single instance of `thrpt-receiver` running under heavy
load on a dedicated node in the TriggerMesh production cluster ([GCE n1-standard-2][gce-machines]). Those metrics can be
used as a baseline to assess the performance of other CloudEvent handlers.

Using the [ramp-requests.py](../load-ramping/) script, a succession of attacks were performed at different rates in
periods of 5s, ranging from 1 event/sec to 79,432 events/sec (theoretical), with a payload of 2 KiB. The attacker was
running on a dedicated compute-optimized node ([GCE c2-standard-8][gce-machines]) to avoid exhausting the node's
resources at an early stage of the attack. The receiver's event store was initialized with a size of 900,000 using the
`-estimated-total-events` flag.

We ensured that the limits of the `thrpt-receiver` and `vegeta` processes, in particular the maximum number of open
files, were high enough to sustain a high number of concurrent connections. The number of outgoing connections
established by a single load generator can not exceed the number of ports available on its host (65536), so we consider
any value above that number as acceptable for the `nofile` resource limit:

```
/ # cat /proc/3352/limits
Limit                     Soft Limit           Hard Limit           Units
...
Max processes             unlimited            unlimited            processes
Max open files            1048576              1048576              files
...
```

### Throughput

The throughput data collected by the Mako sidecar was extracted as a CSV file and displayed in a chart generated using
gnuplot (see the [gnuplot](#gnuplot) section). We can observe that the throughput becomes extremely unstable at about
**12,000 events/sec**.

> _It's over 9000!_

![Receive throughput](.assets/profiling-receive-throughput.png)

The Vegeta report for the 12,589 events/sec attack shows a 100% success ratio, but high latencies above the 90th
percentile:

```
Requests      [total, rate, throughput]         62947, 12589.63, 12387.62
Duration      [total, attack, wait]             5.081s, 5s, 81.532ms
Latencies     [min, mean, 50, 90, 95, 99, max]  187.316µs, 133.384ms, 14.439ms, 528.109ms, 629.386ms, 1.326s, 2.386s
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     128915456, 2048.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:62947
Error Set:
```

At 15,848 events/sec, we start seeing a few errors and some unacceptably high latencies (median above 1s):

```
Requests      [total, rate, throughput]         78950, 15787.37, 6784.90
Duration      [total, attack, wait]             11.512s, 5.001s, 6.511s
Latencies     [min, mean, 50, 90, 95, 99, max]  319.739µs, 1.474s, 1.527s, 2.703s, 2.851s, 4.029s, 8.418s
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     159959040, 2026.08
Success       [ratio]                           98.93%
Status Codes  [code:count]                      0:845  200:78105
Error Set:
Post "http://thrpt-receiver.perf-thrpt-receiver": read tcp 10.16.51.3:35317->10.19.245.6:80: read: connection reset by
peer
Post "http://thrpt-receiver.perf-thrpt-receiver": read tcp 10.16.51.3:52902->10.19.245.6:80: read: connection reset by
peer
Post "http://thrpt-receiver.perf-thrpt-receiver": read tcp 10.16.51.3:45288->10.19.245.6:80: read: connection reset by
peer
...
```

Above 19,000 events/sec, our attacker is unable to produce enough requests to keep up with the target, as shown in the
following reports for 19,953 events/sec and 25,119 events/sec (see the value for `Requests . rate`):

```
Requests      [total, rate, throughput]         96416, 19283.75, 8150.28
Duration      [total, attack, wait]             11.774s, 5s, 6.775s
Latencies     [min, mean, 50, 90, 95, 99, max]  221.071µs, 3.63s, 3.89s, 6.24s, 6.49s, 6.756s, 10.075s
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     196536320, 2038.42
Success       [ratio]                           99.53%
Status Codes  [code:count]                      0:451  200:95965
Error Set:
Post "http://thrpt-receiver.perf-thrpt-receiver": dial tcp 0.0.0.0:0->10.19.245.6:80: bind: address already in use

Requests      [total, rate, throughput]         111869, 22365.76, 3890.88
Duration      [total, attack, wait]             22.053s, 5.002s, 17.052s
Latencies     [min, mean, 50, 90, 95, 99, max]  5.248ms, 11.403s, 12.524s, 16.096s, 16.786s, 16.943s, 21.056s
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     175732736, 1570.88
Success       [ratio]                           76.70%
Status Codes  [code:count]                      0:26062  200:85807
Error Set:
Post "http://thrpt-receiver.perf-thrpt-receiver": dial tcp 0.0.0.0:0->10.19.245.6:80: bind: address already in use
```

Finally, we confirmed the limit of **12,000 events/sec** with a sustained attack (3m) coordinated between 2 event
senders (2x 6,000 events/s):

_Attacker "vegeta"_

```console
$ cegen -u http://thrpt-receiver.perf-thrpt-receiver.svc.cluster.local -d @/sample-ce-data.json | vegeta attack -lazy -format json -duration 3m -rate 6000/s | vegeta report
Requests      [total, rate, throughput]         1080005, 6000.03, 6000.02
Duration      [total, attack, wait]             3m0s, 3m0s, 309.965µs
Latencies     [min, mean, 50, 90, 95, 99, max]  175.806µs, 62.058ms, 2.516ms, 116.84ms, 418.284ms, 1.113s, 4.524s
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     2211850240, 2048.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:1080005
Error Set:
```

_Attacker "goku"_

```console
$ cegen -u http://thrpt-receiver.perf-thrpt-receiver.svc.cluster.local -d @/sample-ce-data.json | vegeta attack -lazy -format json -duration 3m -rate 6000/s | vegeta report
Requests      [total, rate, throughput]         1080005, 6000.03, 5999.65
Duration      [total, attack, wait]             3m0s, 3m0s, 11.254ms
Latencies     [min, mean, 50, 90, 95, 99, max]  175.479µs, 62.069ms, 2.579ms, 119.076ms, 419.17ms, 1.104s, 2.918s
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     2211850240, 2048.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:1080005
Error Set:
```

### Latency

In the graph below, we can see the first failed requests occuring during the attack at 15,848 events/sec. However, the
p99 above the 12,000 events/s mark is already consistently above 1s (see the reports for the single and coordinated
attacks in the previous section).

![Latency profile](.assets/profiling-latencies.png)

### Heap

During an earlier, less agressive load test with the Go garbage collector disabled, `thrpt-receiver` was running with
the `-profiling` flag so we could extract an execution trace from the pprof server and analyze the heap profile of the
application:

```console
$ kubectl -n perf-thrpt-receiver port-forward thrpt-receiver 8008
Forwarding from 127.0.0.1:8008 -> 8008
```

```console
$ curl 'http://localhost:8008/debug/pprof/trace?seconds=75' > trace.out
(collects an execution trace for 75s, then returns)
```

```console
$ go tool trace trace.out
2020/11/10 21:39:10 Parsing trace...
2020/11/10 21:39:18 Splitting trace...
2020/11/10 21:39:32 Opening browser. Trace viewer is listening on http://127.0.0.1:33877
```

In a web browser, we observed an initial heap size of 4.4 MiB. After 27,5124 events had been received, 3.5 GiB were
allocated to the heap. This number went down to 125.7 MiB after the forced garbage collection triggered by
`thrpt-receiver`.

![Heap profile after GC](.assets/profiling-heap.png)

[mako-stub]: https://github.com/knative/pkg/tree/release-0.18/test/mako
[gsheets-ts-formula]: https://webapps.stackexchange.com/a/112651
[gsheets-fill]: https://support.google.com/docs/answer/75509
[gnuplot]: http://www.gnuplot.info/
[vegeta]: https://github.com/tsenart/vegeta
[gce-machines]: https://cloud.google.com/compute/docs/machine-types
