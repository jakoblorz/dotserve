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

func captureReader(out chan<- string, in io.Reader) {
	defer close(out)

	r := bufio.NewReader(in)

	for {
		chunk, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "%+v", err)
			}
			return
		}

		out <- chunk
	}
}

func captureWriter(in <-chan string, out io.Writer) {
	for {
		chunk, ok := <-in
		if !ok {
			return
		}

		fmt.Fprint(os.Stdout, chunk)
	}
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

	file := ""
	boot := &sync.WaitGroup{}
	boot.Add(1)

	stdin := make(chan string)
	stdout := make(chan string)
	go captureReader(stdin, os.Stdin)
	go captureWriter(stdout, os.Stdout)
	go func(doPipeOut bool) {
		defer boot.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-stdin:
				if !ok {
					return
				}
				file = fmt.Sprintf("%s%s", file, chunk)

				if doPipeOut {
					stdout <- chunk
				}
			}
		}
	}(*pipeOut)
	go func() {
		boot.Wait()

		select {
		case <-ctx.Done():
			return
		default:
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				htmlPageTemplate.Execute(w, fmt.Sprintf("`%s`", file))
			})

			err := http.Serve(l, nil)
			sig <- os.Interrupt

			if err != nil {
				fmt.Fprintf(os.Stderr, "%+v", err)
				return
			}
		}
	}()

	<-sig
}

var (
	address          = flag.String("addr", "localhost:8080", "set the listen address ip:port")
	pipeOut          = flag.Bool("p", false, "enable piping, writes the stdin to stdout")
	htmlPageTemplate = MustParsef(`<html>
		<head>
			<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
			<meta charset="utf-8">
			<title>Viztree</title>
		</head>

		<body>
			<div id="graph"></div>
		</body>
		
		<script language="javascript" type="text/javascript">%s</script>
		<script language="javascript">
			(function() {
				document.getElementById("graph).innerHTML = Viz({{.}}, "svg");
			})();
		</script>
	</html>`, MustAsset("viz.js"))
)

func MustParsef(format string, args ...interface{}) *template.Template {
	t := template.New("")
	t, err := t.Parse(fmt.Sprintf(format, args...))
	if err != nil {
		panic(err)
	}
	return t
}
