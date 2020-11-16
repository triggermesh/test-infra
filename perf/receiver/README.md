# receiver

The simplest possible CloudEvent HTTP receiver. Responds to every request with an ACK (HTTP 200) without processing the event.

* Compared to `event_display`, `receiver` doesn't produce blocking calls due to writing to stdout.
* Compared to `thrpt-receiver`, there is no dependency on a Mako sidecar, so `receiver` can run as a Knative Service.
