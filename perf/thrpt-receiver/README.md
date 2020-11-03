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
        Periodically publish the length of the receive queue to Mako.
  -recheck-period duration
        Frequency at which the recording of new events is being checked. (default 5s)
```

## Running the receiver in a cluster

Deploy the receiver and its dependencies into the `perf-thrpt-receiver` namespace using `ko`:

```console
$ ko apply -f config/
```

The receiver will be waiting for the first event to be received:

```console
$ kubectl -n perf-thrpt-receiver logs perf-thrpt-receiver receiver
2020/10/30 16:17:03 Running event recorder
2020/10/30 16:17:03 Running CloudEvents handler
2020/10/30 16:17:03 Waiting for the first event to be received
```

Configure an event source to send events to the `perf-thrpt-receiver` Service, for example using a sink reference:

```yaml
sink:
  ref:
    apiVersion: v1
    kind: Service
    name: perf-thrpt-receiver
```

After the first event has been received, the receiver keeps processing events and checks every `recheck-period` that
events are still being received. If no event is received for a number of periods corresponding to
`consecutive-quiet-periods`, the receiver stops its event handler and publishes the benchmark's results to its Mako
sidecar:

```console
$ kubectl -n perf-thrpt-receiver logs perf-thrpt-receiver receiver
...
2020/10/30 16:28:05 Event received, waiting until no more event is being recorded for 2 consecutive periods of 5s
2020/10/30 16:30:30 Observed 1 period(s) without event
2020/10/30 16:30:35 Observed 2 period(s) without event
2020/10/30 16:30:35 Stopping event recorder and CloudEvents handler
2020/10/30 16:30:35 Received events count: 1000
2020/10/30 16:30:35 Processing data
2020/10/30 16:30:35 Publishing results to Mako
```

The results, presented in a CSV format, can be exported from the logs of the Mako sidecar:

```console
$ kubectl -n perf-thrpt-receiver logs perf-thrpt-receiver mako-stub
# Received input
# Input completed
# Benchmark  - Event throughput
# {"benchmarkKey":"","tags":["nodes=6","project-id=cebuk-01","zone=us-central1-a","commit=f584d79","kubernetes=v1.17.12-gke.500","goversion=go1.15.2"]}
# inputValue,errorMessage,rt
...
1.6040212866868181e+12,,4
1.6040212866869102e+12,,5
1.6040212866874644e+12,,6
1.6040212866875515e+12,,7
1.6040212866882632e+12,,8
1.6040212866883477e+12,,9
1.6040212880283127e+12,,1
1.6040212880283296e+12,,1
1.6040212880290117e+12,,2
1.6040212880291021e+12,,3
1.6040212880294727e+12,,4
1.6040212880295918e+12,,5
1.6040212880302295e+12,,6
1.6040212880302585e+12,,7
1.6040212880309763e+12,,8
...
# CSV end

```

Clean up the receiver's resources at the end of the benchmark:

```console
$ ko delete -f config/
```

[mako-stub]: https://github.com/knative/pkg/tree/release-0.18/test/mako
