# cegen

The _CloudEvents generator_.

Outputs an infinite stream of [vegeta][vegeta]-compatible JSON targets containing valid CloudEvents.

## Usage

```
Usage of cegen:
  -d string
     Data to set in generated CloudEvents. Prefix with '@' to read from a file
  -s string
     Value to set as the CloudEvent source context attribute (default "attackr")
  -t string
     Value to set as the CloudEvent type context attribute (default "io.triggermesh.perf.drill")
  -u string
     URL of the CloudEvents receiver to use in generated vegeta targets
```

Example of usage in combination with vegeta:

```
cegen -d=@data.json -u=http://mytarget.mynamespace \
  | vegeta attack -rate=1000/s -format=json -lazy -duration=30s | \
  | vegeta report
```

## Build

To compile the tool from source for your current platform and architecture and run it locally, you can either

* generate the `cegen` binary in the current directory with [`go build .`][go-build], then execute it with `./cegen
  [arguments...]`
* combine compilation and execution in a temporary directory with [`go run . [arguments...]`][go-run]

## Benchmark

`cegen` is able to output ~110,000 targets/sec to a tmpfs on an Intel Core i5-8265U CPU.

```console
$ mount -t tmpfs -o size=512m tmpfs /mnt/ramdisk/
```

```console
$ time (cegen -u http://localhost -d '@2k_payload.txt' > /mnt/ramdisk/out &; pid=$!; sleep 1; kill $pid)
0.00s user 0.00s system 0% cpu 1.003 total
```

```console
$ cat /mnt/ramdisk/out | wc -l
112190
```

[vegeta]: https://github.com/tsenart/vegeta
[go-build]: https://golang.org/cmd/go/#hdr-Compile_packages_and_dependencies
[go-run]: https://golang.org/cmd/go/#hdr-Compile_and_run_Go_program
