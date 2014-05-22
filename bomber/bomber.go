// Copyright 2014 liujianping. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package boomer provides commands to run load tests and display results.
package bomber

import (
	"time"
	"sync"
	"errors"
	"github.com/rakyll/pb"
)

type Result struct {
	err           error
	statusStep	  int
	statusCode    int
	duration      time.Duration
	contentLength int64
}

func BuildResult(e error, step int, code int, contentLen int64) *Result {
	return &Result{err:e, statusStep:step, statusCode:code, contentLength: contentLen}
}

type IBullet interface{
	Do(ctx interface{}) *Result
}

type IProvider interface{
	Bullet() IBullet
}

type Bomber struct {
	// Total number of requests to make.
	N int
	// Concurrency level, the number of concurrent workers to run.
	C int
	// Timeout in seconds.
	Timeout int
	// Rate limit.
	Qps int
	// Output type
	Output string
	// private variables
	provider IProvider
	bar     *pb.ProgressBar
	rpt     *report
	results chan *Result
}

func (b *Bomber) exploder(ch chan IBullet) {

	for bullet := range ch {
		s := time.Now()
		r := bullet.Do(b.provider)
		if b.bar != nil {
			b.bar.Increment()
		}
		r.duration = time.Now().Sub(s)
		b.results <- r
	}
}

func (b *Bomber) Provider(provider IProvider) {
	b.provider = provider
}

func (b *Bomber) Run() error{
	if b.provider == nil {
		return errors.New("none bullet provider setting.")
	}
	
	b.results = make(chan *Result, b.N)
	if b.Output == "" {
		b.bar = newPb(b.N)
	}
	b.rpt = newReport(b.N, b.results, b.Output)
	b.run()
	return nil
}

func (b *Bomber) run() {
	var wg sync.WaitGroup
	wg.Add(b.C)

	var throttle <-chan time.Time
	if b.Qps > 0 {
		throttle = time.Tick(time.Duration(1e6/(b.Qps)) * time.Microsecond)
	}

	start := time.Now()
	jobs := make(chan IBullet, b.N)
	// Start workers.
	for i := 0; i < b.C; i++ {
		go func() {
			b.exploder(jobs)
			wg.Done()
		}()
	}

	// Start sending jobs to the workers.
	for i := 0; i < b.N; i++ {
		if b.Qps > 0 {
			<-throttle
		}
		jobs <- b.provider.Bullet()
	}
	close(jobs)

	wg.Wait()
	if b.bar != nil {
		b.bar.Finish()
	}
	b.rpt.finalize(time.Now().Sub(start))
}

func newPb(size int) (bar *pb.ProgressBar) {
	bar = pb.New(size)
	bar.Format("Bom !")
	bar.Start()
	return
}


