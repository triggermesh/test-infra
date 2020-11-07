# attackr

Thin layer around the [vegeta][vegeta] library to drill a CloudEvents receiver and analyze the results.

```
Usage of attackr:
  -d duration
        Duration of the attack (default 10s)
  -f uint
        Frequency of requests in events/s (default 1000)
  -m string
        Mode of operation (default "constant")
  -s uint
        Size of the events' data in bytes (default 2048)
  -t duration
        Maximum time to wait for each request to be responded to (default 3s)
  -u string
        URL of the CloudEvents receiver to send events to
  -w uint
        Number of initial vegeta workers (default 10)
```

---

## How-to

### Running locally

To compile the tool from source for your current platform and architecture and run it locally, you can either

* generate the `attackr` binary in the current directory with [`go build .`][go-build], then execute it with `./attackr
  [arguments...]`
* combine compilation and execution in a temporary directory with [`go run . [arguments...]`][go-run]

### Running inside Kubernetes

Because the typical event target will run inside a Kubernetes cluster and not be exposed on a public network, the
[`config/`](./config) directory contains a sample manifest that can be used to run a single `attackr` instance inside
that same cluster using [`ko`][ko].

*:information_source: Always ensure the `KO_DOCKER_REPO` environment variable is set to the URL of a writable container
registry before running the command below.*

```console
$ ko apply -f config/
```

## About supported modes

`attackr` can perform attacks following different profiles depending on the value of the mode flag (`-m`). The supported
modes are described below.

### `constant`

Send events at a constant rate (`-f`) for the whole duration (`-d`) of the attack.

Useful to test whether a service can sustain a constant flow of requests without failing or exceeding a certain latency.

```
rate

│
│
│
│ ------------------------
│
│
└─────────────────────────── time
```

### `ramp`

Send events in 5 intervals of same duration at increasing rates, starting at 1/5th of the maximum rate (`-f`). The sum
of all intervals equals the configured attack duration (`-d`).

Useful to find the threshold at which a service starts failing or exceeding a certain latency.

```
rate

│
│                     ----
│                    /
│                ----
│               /
│           ----
│          /
│      ----
│     /
│ ----
└─────────────────────────── time
```

> This is a simple method for determining the maximum throughput a system can
> handle. It involves adding load in small increments and measuring the delivered
> throughput until a limit is reached. The results can be graphed, showing a
> scalability profile. [...]
>
> [...] When following this approach, measure latency as well as the throughput,
> especially the latency distribution. [...] If you push load too high, latency
> may become so high that it is no longer reasonable to consider the result as
> valid. Ask yourself if the delivered latency would be acceptable to a
> customer.
>
> -- _Brendan Gregg, [Systems Performance][sysperfbook], Chapter 12.3.7 "Ramping Load"_

[vegeta]: https://github.com/tsenart/vegeta
[go-build]: https://golang.org/cmd/go/#hdr-Compile_packages_and_dependencies
[go-run]: https://golang.org/cmd/go/#hdr-Compile_and_run_Go_program
[ko]: https://github.com/google/ko
[sysperfbook]: http://www.brendangregg.com/sysperfbook.html
