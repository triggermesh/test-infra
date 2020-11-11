# Load ramping

A customized version of [vegeta's load-ramping script][vegeta-lr] that can use a stream of JSON targets to perform
attacks at different request rates, and output results that can be used to graph the latency distribution and success
rate at each request rate using [gnuplot][gnuplot].

> This is a simple method for determining the maximum throughput a system can handle. It involves adding load in small
> increments and measuring the delivered throughput until a limit is reached. The results can be graphed, showing a
> scalability profile. [...]
>
> [...] When following this approach, measure latency as well as the throughput, especially the latency distribution.
> [...] If you push load too high, latency may become so high that it is no longer reasonable to consider the result as
> valid. Ask yourself if the delivered latency would be acceptable to a customer.
>
> -- _Brendan Gregg, [Systems Performance][sysperfbook], Chapter 12.3.7 "Ramping Load"_

## Usage

```
Usage:
  <JSON TARGETS GENERATOR> | ./ramp-requests.py
```

Example of usage in combination with [cegen][cegen]:

```
cegen -d=@sample-ce-data.json -u=http://mytarget.mynamespace | ./ramp-requests.py
```

_The included [sample-ce-data.json](./sample-ce-data.json) file was generated using <https://www.json-generator.com>.
Its size is exactly 2048 bytes._

## Plotting

It is possible to plot the results of the load test using [gnuplot][gnuplot] and the provided `ramp-requests.plt` file.

To generate a plot from the contents of the `results_latency.txt` and `results_success.txt` and export it as a PNG file,
run:

```console
$ gnuplot -e "set term png size 1280, 800" ramp-requests.plt > result.png
```

## Running inside Kubernetes

### Deployment

Because the typical event target will run inside a Kubernetes cluster and not be exposed on a public network, the
[`config/`](./config) directory contains a sample manifest that can be used to execute `ramp-requests.py` inside that
same cluster using the container image mentioned in the next section.

To deploy the `ramp-requests` Pod, execute:

```console
$ kubectl create -f config/
```

The output of both `results_latency.txt` and `results_success.txt` files is printed to stdout upon completion.

### Container image

A container image which includes `vegeta`, `cegen` and `ramp-requests.py` is available at
`gcr.io/triggermesh/perf/ramp-requests`.

It was built using the command below from the current directory:

```console
$ docker build -t gcr.io/triggermesh/perf/ramp-requests -f container/Dockerfile ../../
```

The following environment variables, which correspond to [cegen][cegen]' command-line flags, can be used to control the
execution of the attack:

| Variable     | Default                   | Description                                                               |
|--------------|---------------------------|---------------------------------------------------------------------------|
| `TARGET_URL` | http://localhost          | URL of the CloudEvents receiver to use in generated vegeta targets        |
| `CE_TYPE`    | io.triggermesh.perf.drill | Value to set as the CloudEvent type context attribute                     |
| `CE_SOURCE`  | cegen                     | Value to set as the CloudEvent source context attribute                   |
| `CE_DATA`    | @/sample-ce-data.json     | Data to set in generated CloudEvents. Prefix with '@' to read from a file |

[vegeta-lr]: https://github.com/tsenart/vegeta/tree/master/scripts/load-ramping
[gnuplot]: http://www.gnuplot.info/
[sysperfbook]: http://www.brendangregg.com/sysperfbook.html
[cegen]: ../tools/cegen/
