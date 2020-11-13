// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package retry provides generic action retry.
package retry

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// RetryableFunc represents a function that can be retried.
type RetryableFunc func() error

// Retryer defines the requirements for retrying a function.
type Retryer interface {
	Retry(RetryableFunc) error
}

// Ticker defines the requirements for providing a clock to the retry logic.
type Ticker interface {
	Tick() time.Duration
	StopChan() <-chan struct{}
	Stop()
}

// ErrorSet represents a set of unique errors.
type ErrorSet struct {
	errs []error

	mu sync.Mutex
}

func (e *ErrorSet) Error() string {
	if len(e.errs) == 0 {
		return ""
	}

	errString := fmt.Sprintf("%d error(s) occurred:", len(e.errs))
	for _, err := range e.errs {
		errString = fmt.Sprintf("%s\n\t%s", errString, err)
	}

	return errString
}

// Append adds the error to the set if the error is not already present.
func (e *ErrorSet) Append(err error) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.errs == nil {
		e.errs = []error{}
	}

	ok := false

	for _, existingErr := range e.errs {
		if err.Error() == existingErr.Error() {
			ok = true

			break
		}
	}

	if !ok {
		e.errs = append(e.errs, err)
	}

	return ok
}

// Is implements errors.Is.
func (e *ErrorSet) Is(err error) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	return len(e.errs) == 1 && errors.Is(e.errs[0], err)
}

// TimeoutError represents a timeout error.
type TimeoutError struct{}

func (TimeoutError) Error() string {
	return "timeout"
}

// IsTimeout reutrns if the provided error is a timeout error.
func IsTimeout(err error) bool {
	_, ok := err.(TimeoutError)

	return ok
}

type expectedError struct{ error }

func (e expectedError) Unwrap() error {
	return e.error
}

type unexpectedError struct{ error }

func (e unexpectedError) Unwrap() error {
	return e.error
}

type retryer struct {
	duration time.Duration
	options  *Options
}

type ticker struct {
	C       chan time.Time
	options *Options
	rand    *rand.Rand
	s       chan struct{}
}

func (t ticker) Jitter() time.Duration {
	if int(t.options.Jitter) == 0 {
		return time.Duration(0)
	}

	if t.rand == nil {
		t.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return time.Duration(t.rand.Int63n(int64(t.options.Jitter)))
}

func (t ticker) StopChan() <-chan struct{} {
	return t.s
}

func (t ticker) Stop() {
	t.s <- struct{}{}
}

// ExpectedError error represents an error that is expected by the retrying
// function. This error is ignored.
func ExpectedError(err error) error {
	if err == nil {
		return nil
	}

	return expectedError{err}
}

// UnexpectedError error represents an error that is unexpected by the retrying
// function. This error is fatal.
func UnexpectedError(err error) error {
	if err == nil {
		return nil
	}

	return unexpectedError{err}
}

func retry(f RetryableFunc, d time.Duration, t Ticker, o *Options) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	errs := &ErrorSet{}

	for {
		if err := f(); err != nil {
			exists := errs.Append(err)

			switch err.(type) {
			case expectedError:
				// retry expected errors
				if !exists && o.LogErrors {
					log.Printf("retrying error: %s", err)
				}
			default:
				return errs
			}
		} else {
			return nil
		}

		select {
		case <-timer.C:
			errs.Append(TimeoutError{})

			return errs
		case <-t.StopChan():
			return nil
		case <-time.After(t.Tick()):
		}
	}
}
