package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"text/template"
)

// go:generate go get -u github.com/go-bindata/go-bindata/...
// go:generate go-bindata -o viz.go -nomemcopy ./viz.js

func caputureReader(in chan<- string, r io.Reader) {
	defer close(in)

	s := bufio.NewScanner(r)
	s.Split(bufio.ScanLines)

	for s.Scan() {
		in <- s.Text()
	}
}

func signalf(sig chan os.Signal, err error) {
	fmt.Fprintf(os.Stderr, "%+v", err)
	sig <- os.Interrupt
}

func main() {
	flag.Parse()

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := net.Listen("tcp", *address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v", err)
		return
	}
	defer l.Close()

	chunks := ""
	chunksWg := &sync.WaitGroup{}
	chunksWg.Add(1)

	stdin := make(chan string)
	go caputureReader(stdin, os.Stdin)
	go func(doPipeOut bool) {
		defer chunksWg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-stdin:
				if !ok {
					return
				}
				chunks = fmt.Sprintf("%s%s", chunks, chunk)

				if doPipeOut {
					print(chunk)
				}
			}
		}
	}(*pipeOut)
	go func() {
		chunksWg.Wait()

		select {
		case <-ctx.Done():
			return
		default:
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				htmlPageTemplate.Execute(w, chunks)
			})
			err := http.Serve(l, nil)
			if err != nil {
				signalf(sig, err)
				return
			}
			sig <- os.Interrupt
		}
	}()

	<-sig
}

var (
	address          = flag.String("addr", "localhost:8080", "set the listen address ip:port")
	pipeOut          = flag.Bool("p", false, "enable piping, writes the stdin to stdout")
	htmlPageTemplate = MustParsef(`<html>
		%s
	</html>`, string(MustAsset("viz.js")))
)

func MustParsef(format string, args ...interface{}) *template.Template {
	t := template.New("")
	t, err := t.Parse(fmt.Sprintf(format, args...))
	if err != nil {
		panic(err)
	}
	return t
}
