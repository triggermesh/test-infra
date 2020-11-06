# attackr

Thin layer around the [vegeta][vegeta] library to drill a CloudEvents receiver with a constant request rate.

```
Usage of attackr:
  -d duration
        Duration of the attack (default 10s)
  -f uint
        Frequency of requests in events/s (default 1000)
  -s uint
        Size of the events' data in bytes (default 2048)
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
* combine compilation and execution in a temporary directory with [`go run . [arguments...]`][go-run].

### Running inside Kubernetes

Because the typical event target will run inside a Kubernetes cluster and not be exposed on a public network, the
[`config/`](./config) directory contains a sample manifest that can be used to run a single `attackr` instance inside
that same cluster using [`ko`][ko].

_:information_source: Always ensure the `KO_DOCKER_REPO` environment variable is set to the URL of a writable container
registry before running the command below._

```console
$ ko apply -f config/
```

[vegeta]: https://github.com/tsenart/vegeta
[go-build]: https://golang.org/cmd/go/#hdr-Compile_packages_and_dependencies
[go-run]: https://golang.org/cmd/go/#hdr-Compile_and_run_Go_program
[ko]: https://github.com/google/ko
