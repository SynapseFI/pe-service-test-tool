package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	NoError int = iota
	ErrUnknown
	ErrHttp
	ErrTooManyFiles
	ErrConnectionRejected
	ErrNoSuchHost
	ErrBrokenPipe
)

var errsMap = map[int]string{
	ErrTooManyFiles:       "too many open files",
	ErrConnectionRejected: "connection reset by peer",
	ErrNoSuchHost:         "no such host",
	ErrBrokenPipe:         "broken pipe",
}

func main() {
	var concurrency int
	var overallRequests int
	var printOneLine bool

	flag.IntVar(&concurrency, "c", 1, "Concurrency")
	flag.IntVar(&overallRequests, "n", 1, "Overall number of requests to run")
	flag.BoolVar(&printOneLine, "o", true, "Whether to print results in one line as the test progresses or on separate lines")

	flag.Usage = func() {
		fmt.Println("Usage:")
		fmt.Println("./service-test [ -c=1 ] [ -n=1 ] [ -o=true ] url")
		flag.PrintDefaults()
	}

	flag.Parse()

	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	url := flag.Arg(0)

	resultsCh := make(chan int)
	httpErrCh := make(chan int)

	// Prevents requests beyond the overall number
	next := make(chan bool, overallRequests)
	for i := 0; i < overallRequests; i++ {
		next <- true
	}

	for i := 0; i < concurrency; i++ {
		go run(url, resultsCh, httpErrCh, next)
	}

	success := 0
	fail := 0

	fmt.Println()

	startedAt := time.Now()

	lineStart := ""
	lineEnd := "\n"

	if printOneLine {
		lineStart = "\r"
		lineEnd = ""
	}

	errs := map[int]int{}
	httpErrs := map[int]int{}

	for success+fail < overallRequests {
		res := <-resultsCh
		if res == NoError {
			success++
		} else {
			fail++
			if v, ok := errs[res]; ok {
				errs[res] = v + 1
			} else {
				errs[res] = 1
			}
			if res == ErrHttp {
				statusCode := <-httpErrCh
				if v, ok := httpErrs[statusCode]; ok {
					httpErrs[statusCode] = v + 1
				} else {
					httpErrs[statusCode] = 1
				}
			}
		}
		fmt.Printf("%sSucceeded: %d, failed: %d, overall: %d, time since start: %v%s", lineStart, success, fail, success+fail, time.Since(startedAt), lineEnd)
	}

	if len(errs) > 0 {
		fmt.Printf("\n\nErrors:\n")
	}

	for k, v := range errs {
		name := errsMap[k]
		if k == ErrUnknown {
			name = "Unknown"
		}
		if k == ErrHttp {
			name = "HTTP"
		}
		fmt.Printf("%s: %v\n", name, v)
	}

	if len(httpErrs) > 0 {
		fmt.Printf("\nHTTP err codes:\n")
		for k, v := range httpErrs {
			fmt.Printf("code %d: %d\n", k, v)
		}
	}

	fmt.Print("\n\n")
}

func run(url string, resultsCh, httpErrCh chan<- int, next <-chan bool) {
OUTER:
	for {
		<-next
		res, err := http.Get(url)
		if err != nil {
			for k, v := range errsMap {
				if strings.Contains(err.Error(), v) {
					resultsCh <- k
					continue OUTER
				}
			}
			log.Println(err)

			resultsCh <- ErrUnknown
			continue
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			resultsCh <- ErrHttp
			httpErrCh <- res.StatusCode
			continue
		}
		res.Body.Close()
		resultsCh <- NoError
	}
}
