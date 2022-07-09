package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	NoError int = iota
	ErrUnknown
	ErrHttp
	ErrTooManyFiles
	ErrConnectionRejected
	ErrConnectionRefused
	ErrNoSuchHost
	ErrBrokenPipe
)

var errsMap = map[int]string{
	ErrTooManyFiles:       "too many open files",
	ErrConnectionRejected: "connection reset by peer",
	ErrConnectionRefused:  "connection refused",
	ErrNoSuchHost:         "no such host",
	ErrBrokenPipe:         "broken pipe",
}

var client *http.Client

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

	actualConcurrency := concurrency
	if overallRequests < concurrency {
		actualConcurrency = overallRequests
	}

	// Customize the Transport to have larger connection pool
	defaultRoundTripper := http.DefaultTransport
	defaultTransportPointer, ok := defaultRoundTripper.(*http.Transport)
	if !ok {
		panic(fmt.Sprintf("defaultRoundTripper not an *http.Transport"))
	}
	// Dereference it to get a copy of the struct that the pointer points to
	defaultTransport := *defaultTransportPointer
	defaultTransport.MaxIdleConns = actualConcurrency + 10
	defaultTransport.MaxIdleConnsPerHost = actualConcurrency + 10

	client = &http.Client{Transport: &defaultTransport}

	resultsCh := make(chan int)
	httpErrCh := make(chan int)
	sigs := make(chan os.Signal, 1)

	// Prevents requests beyond the overall number
	next := make(chan bool)
	go func() {
		for i := 0; i < overallRequests; i++ {
			next <- true
		}
	}()

	for i := 0; i < actualConcurrency; i++ {
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

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	keepGoing := true
	for success+fail < overallRequests && keepGoing {
		var res int
		select {
		case <-sigs:
			keepGoing = false
		case res = <-resultsCh:
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
		res, err := client.Get(url)
		if res != nil && res.Body != nil {
			io.Copy(ioutil.Discard, res.Body)
			res.Body.Close()
		}
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
			resultsCh <- ErrHttp
			httpErrCh <- res.StatusCode
			continue
		}
		resultsCh <- NoError
	}
}
