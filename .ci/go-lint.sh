#!/bin/bash

# Copyright (c) 2017 Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0
#

if [ ! $(command -v gometalinter) ]
then
	go get github.com/alecthomas/gometalinter
	gometalinter --install --vendor
fi

linter_args="--tests --vendor"

# When running the linters in a CI environment we need to disable them all
# by default and then explicitly enable the ones we are care about. This is
# necessary since *if* gometalinter adds a new linter, that linter may cause
# the CI build to fail when it really shouldn't. However, when this script is
# run locally, all linters should be run to allow the developer to review any
# failures (and potentially decide whether we need to explicitly enable a new
# linter in the CI).
if [ "$CI" = true ]; then
	linter_args+=" --disable-all"
fi

linter_args+=" --enable=misspell"
linter_args+=" --enable=vet"
linter_args+=" --enable=ineffassign"
linter_args+=" --enable=gofmt"
linter_args+=" --enable=gocyclo"
linter_args+=" --cyclo-over=15"
linter_args+=" --enable=golint"
linter_args+=" --deadline=600s"

eval gometalinter "${linter_args}" ./...
