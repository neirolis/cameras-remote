package main

import (
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"

	"github.com/jinzhu/configor"
	"github.com/mcuadros/go-defaults"
	"github.com/op/go-logging"
	"github.com/sg3des/argum"
)

var version = "v0.2.2"
var log = logging.MustGetLogger("REMOTE")
var confFilename = "config-remote.yaml"

var args struct {
	Addr      string `argum:"pos,req" help:"rtsp stream address"`
	FrameRate int    `argum:"--framerate" help:"target fps" default:"5"`
	Quality   int    `argum:"--quality" help:"frames quality 1-31" default:"3"`
}

var conf struct {
	Servers []Server `yaml:"servers"`
}

func init() {
	// logFormat := `%{color}[%{module} %{shortfile}] %{message}%{color:reset}`
	logFormat := `%{message}`
	logging.SetFormatter(logging.MustStringFormatter(logFormat))
	logging.SetBackend(logging.NewLogBackend(os.Stderr, "", 0))

	argum.MustParse(&args)
}

func main() {
	if _, err := os.Stat(confFilename); err != nil {
		log.Fatalf("configuration file '%s' not found", err)
	}
	if err := configor.Load(&conf, confFilename); err != nil {
		log.Fatal(err)
	}

	var servers []*Server
	var wg sync.WaitGroup

	// search for a less loaded server
	for _, s := range conf.Servers {
		defaults.SetDefaults(&s)

		wg.Add(1)
		go func(s *Server) {
			defer wg.Done()

			if err := s.Dial(); err != nil {
				log.Error(s.Addr, err)
				return
			}

			if err := s.CheckLoad(); err != nil {
				log.Error(s.Addr, err)
				return
			}

			servers = append(servers, s)
		}(&s)
	}

	wg.Wait()

	// no servers no work
	if len(servers) == 0 {
		log.Fatal("a suitable server for starting the decoding process has not been found")
	}

	// sort servers by load level
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].load < servers[j].load
	})

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		servers[0].Stop()
		os.Exit(1)
	}()

	// pick a less loaded server and run ffmpeg
	if err := servers[0].StartFFmpeg(); err != nil {
		log.Fatal(err)
	}
}
