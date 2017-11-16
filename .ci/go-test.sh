#!/bin/bash

# Copyright (c) 2017 Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0
#

set -e

test_packages=$(go list ./... | grep -v vendor)
echo "Run go test and generate coverage:"
for pkg in $test_packages; do
	if [ "$pkg" = "github.com/kata-containers/ksm-throttler" ]; then
		sudo env GOPATH=$GOPATH GOROOT=$GOROOT PATH=$PATH go test -cover -coverprofile=profile.cov $pkg
	else
		sudo env GOPATH=$GOPATH GOROOT=$GOROOT PATH=$PATH go test -cover $pkg
	fi
done
