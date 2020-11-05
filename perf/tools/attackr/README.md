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

[vegeta]: https://github.com/tsenart/vegeta
