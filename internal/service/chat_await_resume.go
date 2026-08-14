package service

import "errors"

var errResumeInFlight = errors.New("resume already in flight for this session")
