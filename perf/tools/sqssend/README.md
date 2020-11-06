# sqssend

Send batches of messages with a defined size to an Amazon SQS queue.

```
Usage of sqssend:
  -n uint
        Number of messages to send (default 100)
  -s uint
        Size of the messages in bytes (default 2048)
  -u string
        URL of the Amazon SQS queue to send messages to
```

---

## How-to

To compile the tool from source for your current platform and architecture and run it locally, you can either

* generate the `sqssend` binary in the current directory with [`go build .`][go-build], then execute it with `./sqssend
  [arguments...]`
* combine compilation and execution in a temporary directory with [`go run . [arguments...]`][go-run].

[go-build]: https://golang.org/cmd/go/#hdr-Compile_packages_and_dependencies
[go-run]: https://golang.org/cmd/go/#hdr-Compile_and_run_Go_program
