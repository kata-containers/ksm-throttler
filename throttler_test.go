//
// Copyright (c) 2017 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	logLevel := flag.String("log", "warn",
		"log messages above specified level; one of debug, warn, error, fatal or panic")

	flag.Parse()

	if err := SetLoggingLevel(*logLevel); err != nil {
		fmt.Fprint(os.Stderr, err)
	}

	if err := ksmTestPrepare(); err != nil {
		ksmTestCleanup()
		fmt.Fprint(os.Stderr, err)
	}

	exit := m.Run()

	ksmTestCleanup()

	os.Exit(exit)
}
