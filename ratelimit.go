package main

import "go.uber.org/ratelimit"

var (
	requestRateLimit = ratelimit.New(5)
)
