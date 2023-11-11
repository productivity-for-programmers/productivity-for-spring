package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"regexp"
	"time"

	"github.com/radovskyb/watcher"
)

func Build() (bytes.Buffer, error) {
	cmd := exec.Command("bash", "-c", buildCommand)
	cmd.Dir = springDir
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	err := cmd.Run()
	if err != nil {
		log.Printf("Error %v", b.String())
		return b, err
	}
	return b, nil
}

var defaultclient = http.Client{
	Timeout: 10 * time.Millisecond,
}

func WaitForStartup() {
	time.Sleep(1400 * time.Millisecond)
	retries := 0

	// We wrap this in a func() to make full use of defer to do cleanup when returning
	checkhealth := func() bool {
		retries++
		req, err := http.NewRequest(http.MethodGet, baseUrl+healthCheckPath, nil)
		if err != nil {
			log.Printf("client: could not create request: %s", err)
			return false
		}
		res, err := defaultclient.Do(req)
		if err != nil {
			if retries%50 == 0 {
				log.Printf("client: error making http request: %s", err)
			}
			return false
		}
		defer res.Body.Close() // Successful Do, so we need to close the body

		if res.StatusCode == 200 {
			return true
		} else {
			log.Printf("client: status code: %d", res.StatusCode)
		}
		return false
	}

	for !checkhealth() {
		time.Sleep(100 * time.Millisecond)
	}
}

func NewSingleHostBodyBufReverseProxy(target *url.URL, key string) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		httputil.NewSingleHostReverseProxy(target).Director(req)
		if req.Body != nil && req.ContentLength != 0 {
			var buf bytes.Buffer
			tee := io.TeeReader(req.Body, &buf)
			req.Body = io.NopCloser(tee)
			ctx := context.WithValue(req.Context(), key, &buf)
			r2 := req.WithContext(ctx)
			*req = *r2
		}
	}
	return &httputil.ReverseProxy{Director: director}
}

var lastBuild bytes.Buffer
var lastBuildError error
var buildRunning bool
var springDir string
var baseUrl string
var healthCheckPath string
var buildCommand string

func main() {
	flag.StringVar(&springDir, "spring-dir", ".", "Directory of the Spring Boot project")
	flag.StringVar(&baseUrl, "base-url", "http://localhost:8080", "Base URL")
	flag.StringVar(&healthCheckPath, "health-check-path", "/actuator/health", "Health Check Endpoint")
	flag.StringVar(&buildCommand, "build-command", "./gradlew build -x test", "Build command")

	flag.Parse()
	w := watcher.New()
	w.SetMaxEvents(1)
	r := regexp.MustCompile("^*.java$")
	w.AddFilterHook(watcher.RegexFilterHook(r, false))
	if err := w.AddRecursive(springDir); err != nil {
		log.Fatalln(err)
	}
	// for path, f := range w.WatchedFiles() {
	// 	log.Printf("%s: %s\n", path, f.Name())
	// }

	go func() {
		for {
			select {
			case event := <-w.Event:
				log.Println(event)
				buildRunning = true
				log.Println("Building.....")
				lastBuild, lastBuildError = Build()
				if lastBuildError == nil {
					log.Println("Build done, waiting for health check.....")
					WaitForStartup()
					log.Println("Health Check done")
				}
				buildRunning = false
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	go func() {
		if err := w.Start(time.Millisecond * 100); err != nil {
			log.Fatalln(err)
		}
	}()

	remote, err := url.Parse(baseUrl)
	if err != nil {
		panic(err)
	}

	handler := func(p *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			log.Println(r.URL)
			if buildRunning {
				log.Println("Waiting for build to finish")
				for buildRunning {
					time.Sleep(20 * time.Millisecond)
				}
				log.Println("Build done")
			}

			if lastBuildError != nil {
				w.Write(lastBuild.Bytes())
			} else {
				r.Host = remote.Host
				p.ServeHTTP(w, r)
			}
		}
	}

	// proxy := httputil.NewSingleHostReverseProxy(remote)
	proxy := NewSingleHostBodyBufReverseProxy(remote, "bodybuf")
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		time.Sleep(3 * time.Second)
		retryContext := r.Context().Value("retry")
		retry := 0
		if retryContext != nil {
			retry = retryContext.(int)
		}
		retry++
		if retry == 100 {
			w.WriteHeader(502)
			return
		} else {
			if r.Body != nil && r.ContentLength != 0 {
				if buf, ok := r.Context().Value("bodybuf").(*bytes.Buffer); ok {
					if r.ContentLength == int64(buf.Len()) {
						r.Body = io.NopCloser(buf)
					}
				}
			}
			ctx := context.WithValue(r.Context(), "retry", retry)
			r2 := r.WithContext(ctx)
			log.Printf("Retrying %d %v", retry, err)
			proxy.ServeHTTP(w, r2)
			log.Printf("Done Retrying %d", retry)
		}
	}
	http.HandleFunc("/", handler(proxy))
	log.Println("Started")
	err = http.ListenAndServe(":9000", nil)
	if err != nil {
		panic(err)
	}
}
