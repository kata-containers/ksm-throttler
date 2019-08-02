[![Build Status](https://travis-ci.org/kata-containers/ksm-throttler.svg?branch=master)](https://travis-ci.org/kata-containers/ksm-throttler)
[![Go Report Card](https://goreportcard.com/badge/github.com/kata-containers/ksm-throttler)](https://goreportcard.com/report/github.com/kata-containers/ksm-throttler)
[![Coverage Status](https://coveralls.io/repos/github/kata-containers/ksm-throttler/badge.svg?branch=master)](https://coveralls.io/github/kata-containers/ksm-throttler?branch=master)
[![GoDoc](https://godoc.org/github.com/kata-containers/ksm-throttler?status.svg)](https://godoc.org/github.com/kata-containers/ksm-throttler)

# KSM throttling daemon

* [Introduction](#introduction)
* [What is KSM?](#what-is-ksm)
* [Overall architecture](#overall-architecture)
    * [Daemon](#daemon)
        * [Throttling algorithm](#throttling-algorithm)
    * [Throttling triggers](#throttling-triggers)
        * [`virtcontainers` trigger](#virtcontainers-trigger)
    * [gRPC](#grpc)
* [Build and install](#build-and-install)
* [Run](#run)

## Introduction

This project implements a
[Kernel Same-page Merging](https://www.kernel.org/doc/Documentation/vm/ksm.txt)
throttling daemon.

Its goal is to regulate KSM by dynamically modifying the KSM `sysfs`
entries, in order to minimize memory duplication as fast as possible
while keeping the KSM daemon load low.

## What is KSM?

KSM is a host Linux* kernel feature for de-duplicating memory pages.
Although it was initially designed as a KVM specific feature, it is
now part of the generic Linux memory management subsystem and can
be leveraged by any userspace component or application looking for
memory to save.

A daemon (`ksmd`) periodically scans userspace memory, looking for
identical pages that can be replaced by a single, write-protected
page. When a process tries to modify this shared page content, it
gets a private copy into its memory space. KSM only scans and merges
pages that are both anonymous and that have been explicitly tagged as
mergeable by applications calling into the `madvise` system call
(`int madvice(addr, length, MADV_MERGEABLE)`).

KSM is customizable through a set of Linux kernel `sysfs` attributes,
the most interesting ones being:

  * `/sys/kernel/mm/ksm/run`: Turns KSM on (`1`) and off (`0`).
  * `/sys/kernel/mm/ksm/sleep_millisec`: Knob that specifies the KSM
    scanning period.
  * `/sys/kernel/mm/ksm/pages_to_scan`: Sets the number of
    pages KSM will scan per scanning cycle.

The memory density improvements that KSM can provide come at a cost.
Depending on the number of anonymous pages it will scan, it can be
relatively expensive on CPU utilization.

## Overall architecture

This project splits that task into 2 pieces:

1. The throttling algorithm, implemented as a daemon. The daemon can
be asked to throttle KSM up by *kicking* through its gRPC interface.
2. The throttling triggers, implemented as gRPC clients.

### Daemon

The throttling daemon, `ksm-throttler`, implements the throttling
algorithm on one hand and listens for throttling triggers on the
other hand.

#### Throttling algorithm

By default, `ksm-throttler` will throttle KSM up and down. Regardless
of the current KSM system settings, `ksm-throttler` will move them to
the `aggressive` settings as soon as it gets triggered.
With the `aggressive` setting, `ksmd` will run every millisecond and
will scan 10% of all available anonymous pages during each scanning
cycle.

After switching to the `aggressive` KSM settings, `ksm-throttler` will
throttle down to the `standard` setting if it does not get triggered
for the next 30 seconds.
Then `ksm-throttler` will continue throttling down to the `slow` KSM
setting if it does not get triggered for the next 2 minutes.
Finally, `ksm-throttler` will get back to the initial KSM settings after
two more minutes, unless it gets triggered.

At any point in time, `ksm-throttler` will get back to to the
`aggressive` setting when getting triggered:

```
        +----------------+
        |                |
        |    Initial     |
        |    Settings    |<<-------------------------------+
        |                |                                 |
        +-------+--------+                                 |
                |                                          |
                |                                          |
     trigger    |                                          |
                |                                          |
                v                                          |
         +--------------+                                  |
         |  Aggressive  |<<--------+                       |
         +--------------+          |                       |
                |                  |                       |
  No Trigger    |                  |                       |
     (30s)      |                  |    New                |
                |                  |  Trigger              |    No Trigger
                v                  |                       |       (2mn)
         +--------------+          |                       |
         |   Standard   |----------+                       |
         +--------------+          |                       |
                |                  |                       |
  No Trigger    |                  |                       |
     (2mn)      |                  |    New                |
                |                  |  Trigger              |
                v                  |                       |
         +--------------+          |                       |
         |     Slow     |----------+                       |
         +--------------+                                  |
                |                                          |
                |                                          |
                +------------------------------------------+

```

### Throttling triggers

Throttling triggers are gRPC clients to the `ksm-throttler` daemon.
Their role is to identify when KSM needs to be throttled up, depending
on which resources they want to monitor.

#### `virtcontainers` trigger

This project implements a throttling trigger for
[virtcontainers](https://github.com/containers/virtcontainers) based
containers, see https://github.com/kata-containers/ksm-throttler/blob/master/trigger/virtcontainers.

### gRPC

The current gRPC is very simple, and only consists of a `Kick()` method:

```
service KSMThrottler {
	rpc Kick(google.protobuf.Empty) returns (google.protobuf.Empty);
}
```

A package implements a client API in Go for that interface. For example:

```Go
import (
	"flag"
	"fmt"

	"github.com/kata-containers/ksm-throttler/pkg/client"
)

func main() {
	uri := flag.String("uri", "/var/run/kata-ksm-throttler/ksm.sock", "KSM throttler gRPC URI")
	flag.Parse()

	err := client.Kick(*uri)
	if err != nil {
		fmt.Println(err)
	}
}
```

## Build and install

```
$ make
$ sudo make install
```


## Run

To run `ksm-throttler` with virtcontainers as the throttling trigger:

```
$ systemctl start kata-vc-throttler
```

This will start both the `ksm-throttler` daemon and the `vc` throttling
trigger.
