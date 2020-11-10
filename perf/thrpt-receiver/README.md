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
1. [Profiling](#profiling)

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

## Profiling

The figures presented in this section describe the profile of a single instance of `thrpt-receiver` running under heavy
load on a dedicated node in the TriggerMesh production cluster. Those metrics can be used as a baseline to assess the
performance of other CloudEvent receivers.

The Go garbage collector was disabled for the entire duration of the load test to prevent GC pauses from influencing the
results. The event store was initialized with a size of 300,000 using the `-estimated-total-events` flag.

We also ensured that limits of the `thrpt-receiver` process, in particular the maximum number of open files, was high
enough to sustain a high number of concurrent connections. The number of outgoing connections established by a single
load generator can not exceed the number of ports available on its host (65536), so we consider any value above that
number as acceptable for the `nofile` resource limit of the receiver:

```
/ # cat /proc/3352/limits
Limit                     Soft Limit           Hard Limit           Units
...
Max processes             unlimited            unlimited            processes
Max open files            1048576              1048576              files
...
```

The attack was performed in 5 intervals of 12s, by increments of 1600 events/sec, with a payload of 2 KiB. Below is a
summary of the attack as reported by [`vegeta`][vegeta].

```
Requests      [total, rate, throughput]         275124, 4543.47, 4539.96
Duration      [total, attack, wait]             1m1s, 1m1s, 46.915ms
Latencies     [min, mean, 50, 90, 95, 99, max]  250.013µs, 152.998ms, 1.134ms, 555.641ms, 911.698ms, 1.36s, 2.412s
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     563453952, 2048.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:275124
Error Set:
```

The throughput data collected by the Mako sidecar was extracted as a CSV file and displayed in a chart generated using
Google Sheets. Although vegeta reported a success rate of 100%, we can observe that the thoughput becomes extremely
unstable between the 4th and 5th intervals, roughly between 6400 and 8000 events/sec.

![Receive throughput](.assets/profiling-receive-throughput.png)

During the load test, `thrpt-receiver` was running with the `-profiling` flag so we could extract an execution trace
from the pprof server and analyze the heap profile of the application:

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

In a web browser, we observed an initial heap size of 4.4 MiB. After the 275124 events had been received, 3.5 GiB were
allocated to the heap. This number went down to 125.7 MiB after the forced garbage collection triggered by
`thrpt-receiver`.

![Heap profile after GC](.assets/profiling-heap.png)

[mako-stub]: https://github.com/knative/pkg/tree/release-0.18/test/mako
[gsheets-ts-formula]: https://webapps.stackexchange.com/a/112651
[gsheets-fill]: https://support.google.com/docs/answer/75509
[vegeta]: https://github.com/tsenart/vegeta
