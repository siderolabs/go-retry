// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//nolint:testpackage
package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

//nolint:scopelint
func Test_retry(t *testing.T) {
	type args struct { //nolint:govet
		c context.Context //nolint:containedctx
		f RetryableFuncWithContext
		d time.Duration
		t Ticker
		o *Options
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()

	var count int

	tests := []struct {
		name       string
		args       args
		wantString string
	}{
		{
			name: "expected error string",
			args: args{
				c: context.Background(),
				f: func(context.Context) error { return ExpectedError(errors.New("test")) },
				d: 2 * time.Second,
				t: NewConstantTicker(NewDefaultOptions()),
				o: &Options{},
			},
			wantString: "2 error(s) occurred:\n\ttest\n\ttimeout",
		},
		{
			name: "unexpected error string",
			args: args{
				c: context.Background(),
				f: func(context.Context) error { return errors.New("test") },
				d: 2 * time.Second,
				t: NewConstantTicker(NewDefaultOptions()),
				o: &Options{},
			},
			wantString: "1 error(s) occurred:\n\ttest",
		},
		{
			name: "no error string",
			args: args{
				c: context.Background(),
				f: func(context.Context) error { return nil },
				d: 2 * time.Second,
				t: NewConstantTicker(NewDefaultOptions()),
				o: &Options{},
			},
			wantString: "",
		},
		{
			name: "context canceled",
			args: args{
				c: canceledContext,
				f: func(ctx context.Context) error { return nil },
				d: 2 * time.Second,
				t: NewConstantTicker(NewDefaultOptions()),
				o: &Options{},
			},
			wantString: "1 error(s) occurred:\n\tcontext canceled",
		},
		{
			name: "limit attempt",
			args: args{
				c: context.Background(),
				f: func(ctx context.Context) error {
					count++

					if count == 2 {
						return nil
					}

					<-ctx.Done()

					return ctx.Err()
				},
				d: 2 * time.Second,
				t: NewConstantTicker(NewDefaultOptions()),
				o: &Options{
					AttemptTimeout: time.Millisecond,
				},
			},
			wantString: "",
		},
	}

	for _, tt := range tests {
		count = 0

		t.Run(tt.name, func(t *testing.T) {
			if err := retry(tt.args.c, tt.args.f, tt.args.d, tt.args.t, tt.args.o); err != nil && tt.wantString != err.Error() {
				t.Errorf("retry() error = %q\nwant:\n%q", err, tt.wantString)
			}
		})
	}
}

func Test_errors(t *testing.T) {
	e := errors.New("xyz")

	if !errors.Is(ExpectedError(e), e) {
		t.Fatal("expected error should wrap errors")
	}

	if !errors.Is(UnexpectedError(e), e) {
		t.Fatal("unexpected error should wrap errors")
	}

	errSet := ErrorSet{}
	errSet.Append(e)

	if !errors.Is(&errSet, e) {
		t.Fatal("error set should wrap errors")
	}

	errSet.Append(errors.New("foo"))

	if !errors.Is(&errSet, e) {
		t.Fatal("error set should wrap errors")
	}

	errSet = ErrorSet{}
	errSet.Append(UnexpectedError(e))

	if !errors.Is(&errSet, e) {
		t.Fatal("error set should wrap errors")
	}
}
