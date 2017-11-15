//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"errors"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

var defaultKSMRoot = "/sys/kernel/mm/ksm/"
var errKSMUnavailable = errors.New("KSM is unavailable")
var memInfo = "/proc/meminfo"

const (
	ksmRunFile       = "run"
	ksmPagesToScan   = "pages_to_scan"
	ksmSleepMillisec = "sleep_millisecs"
	ksmStart         = "1"
	ksmStop          = "0"
	defaultKSMMode   = ksmAuto
)

type ksmThrottleInterval struct {
	interval time.Duration
	nextKnob ksmMode
}

var ksmAggressiveInterval = 30 * time.Second
var ksmStandardInterval = 120 * time.Second
var ksmSlowInterval = 120 * time.Second

var ksmThrottleIntervals = map[ksmMode]ksmThrottleInterval{
	ksmAggressive: {
		// From aggressive: move to standard and wait 120s
		interval: ksmStandardInterval,
		nextKnob: ksmStandard,
	},

	ksmStandard: {
		// From standard: move to slow and wait 120s
		interval: ksmSlowInterval,
		nextKnob: ksmSlow,
	},

	ksmSlow: {
		// From slow: move to the initial settings and stop there
		interval: 0,
		nextKnob: ksmInitial,
	},

	// We should never make it here
	ksmInitial: {
		interval: 0, // We stay here unless a new container shows up
	},
}

// throttlerLog is the general logger the KSM throttler.
var throttlerLog = logrus.WithFields(logrus.Fields{
	"source": "throttler",
	"name":   "KSM throttler",
	"pid":    os.Getpid(),
})

func main() {
}
